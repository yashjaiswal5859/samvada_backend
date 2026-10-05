package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"voice-agent-go/internal/config"
	"voice-agent-go/internal/db"
	"voice-agent-go/internal/services"
	"voice-agent-go/internal/services/telephony"

	"github.com/google/uuid"
)

type OutboundCallAPIRequest struct {
	ToPhone           string   `json:"to_phone"`
	FromPhone         string   `json:"from_phone,omitempty"`
	CompanyName       string   `json:"company_name,omitempty"`
	TelephonyProvider string   `json:"telephony_provider,omitempty"` // "exotel", "twilio", "plivo", "telnyx"
	LLMModel          string   `json:"llm_model,omitempty"`
	RAGProvider       string   `json:"rag_provider,omitempty"`
	CustomerName      string   `json:"customer_name,omitempty"`
	InitialLanguage   string   `json:"initial_language,omitempty"`  // "hindi" / "hi" or "english" / "en" (default: "english")
	TTSSpeed          float64  `json:"tts_speed,omitempty"`       // e.g. 0.85 (0.75 to 1.25)
	AmbienceVolume    *float64 `json:"ambience_volume,omitempty"` // e.g. 0.35 (0.0 to 1.0)
	CallbackURL       string   `json:"callback_url,omitempty"`
	StreamURL         string   `json:"stream_url,omitempty"`
}

type CallAPIHandler struct {
	ttsService *services.TTSService
}

func NewCallAPIHandler(ttsService *services.TTSService) *CallAPIHandler {
	return &CallAPIHandler{
		ttsService: ttsService,
	}
}

// HandleInitiateCall processes POST /v1/calls/outbound requests dynamically resolving the telephony provider from payload
func (h *CallAPIHandler) HandleInitiateCall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed. Use POST.", http.StatusMethodNotAllowed)
		return
	}

	var req OutboundCallAPIRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON payload: %v", err), http.StatusBadRequest)
		return
	}

	if req.ToPhone == "" {
		http.Error(w, "Field 'to_phone' is required.", http.StatusBadRequest)
		return
	}

	if req.CompanyName == "" {
		req.CompanyName = config.Settings.DefaultCompanyName
	}
	if req.CustomerName == "" {
		req.CustomerName = config.Settings.DefaultCustomerName
	}
	if req.TTSSpeed <= 0 {
		req.TTSSpeed = config.Settings.TtsSpeed
	}

	initialLang := strings.ToLower(strings.TrimSpace(req.InitialLanguage))
	if initialLang == "hi" || initialLang == "hindi" {
		initialLang = "hindi"
	} else {
		initialLang = "english"
	}

	// Resolve the requested telephony provider from payload dynamically (e.g., "exotel", "twilio", "plivo", "telnyx")
	providerName := strings.TrimSpace(req.TelephonyProvider)
	provider, err := telephony.GetFactory().GetProvider(providerName)
	if err != nil {
		log.Printf("[Call API] Failed to resolve telephony provider %q: %v", providerName, err)
		http.Error(w, fmt.Sprintf("Unsupported telephony provider: %s", providerName), http.StatusBadRequest)
		return
	}

	ambVolStr := "Default (0.80)"
	if req.AmbienceVolume != nil {
		ambVolStr = fmt.Sprintf("%.2f", *req.AmbienceVolume)
	}

	log.Printf("\n"+
		"================================================================================\n"+
		"[LEVEL 1A: OUTBOUND API REQUEST RECEIVED]\n"+
		"  ├─ Target Phone:       %s\n"+
		"  ├─ Telephony Provider: %s\n"+
		"  ├─ Company Name:       %s\n"+
		"  ├─ Customer Name:      %s\n"+
		"  ├─ Initial Language:   %s\n"+
		"  ├─ TTS Speed:          %.2f\n"+
		"  └─ Ambience Volume:    %s\n"+
		"================================================================================",
		req.ToPhone, providerName, req.CompanyName, req.CustomerName, initialLang, req.TTSSpeed, ambVolStr)

	// Generate a stable preliminary call ID used as the canonical Redis key for pre-synthesized audio
	preliminaryCallID := uuid.New().String()
	log.Printf("[LEVEL 1B: PRELIMINARY ID GENERATED] PreliminaryCallID: %s", preliminaryCallID)

	greetingText1 := ""
	greetingText2 := ""

	docsBasePath := config.Settings.DocsBasePath
	if docsBasePath == "" {
		docsBasePath = "D:/Voice AI/docs"
	}
	compDir := filepath.Join(docsBasePath, req.CompanyName)

	if initialLang == "hindi" {
		if req.CustomerName != "" && req.CustomerName != "there" && req.CustomerName != "Valued Customer" {
			greetingText1 = fmt.Sprintf("[warm] नमस्ते, क्या मेरी बात %s जी से हो रही है?", req.CustomerName)
		} else {
			greetingText1 = "[warm] नमस्ते, क्या मेरी बात आपसे हो रही है?"
		}

		greetingPathHi := filepath.Join(compDir, "greeting_hi.txt")
		greetingBytesHi, errHi := os.ReadFile(greetingPathHi)
		if errHi == nil && len(greetingBytesHi) > 0 {
			greetingText2 = strings.TrimSpace(string(greetingBytesHi))
			greetingText2 = strings.ReplaceAll(greetingText2, "{name}", req.CustomerName)
			greetingText2 = strings.ReplaceAll(greetingText2, "{CustomerName}", req.CustomerName)
		} else {
			greetingText2 = fmt.Sprintf("[excited] %s जी! मैं %s से Riya बोल रही हूँ। आपके लिए एक special offer है, बस two minutes लगेंगे!", req.CustomerName, req.CompanyName)
		}
	} else {
		if req.CustomerName != "" && req.CustomerName != "there" && req.CustomerName != "Valued Customer" {
			greetingText1 = fmt.Sprintf("[warm] Hello, am I speaking with %s?", req.CustomerName)
		} else {
			greetingText1 = "[warm] Hello, who am I speaking with?"
		}

		greetingPath := filepath.Join(compDir, "greeting.txt")
		greetingBytes, err := os.ReadFile(greetingPath)
		if err == nil && len(greetingBytes) > 0 {
			greetingText2 = strings.TrimSpace(string(greetingBytes))
			greetingText2 = strings.ReplaceAll(greetingText2, "{name}", req.CustomerName)
			greetingText2 = strings.ReplaceAll(greetingText2, "{CustomerName}", req.CustomerName)
		} else {
			greetingText2 = fmt.Sprintf("[excited] Hi %s! Riya here from %s. Got a quick offer for you, just two minutes!", req.CustomerName, req.CompanyName)
		}
	}

	log.Printf("[LEVEL 1C: GREETINGS PREPARED]\n"+
		"  ├─ Turn 1 Greeting Text: %q\n"+
		"  └─ Turn 2 Intro Pitch Text: %q", greetingText1, greetingText2)

	// Pre-synthesis — synthesize fresh Turn 1 greeting & Turn 2 intro on-the-fly (NO fillers)
	var greeting1Audio, greeting2Audio []byte

	if h.ttsService != nil {
		log.Printf("[LEVEL 1D: PRE-SYNTHESIS START] Synthesizing fresh Turn 1 & Turn 2 greeting audio clips...")
		ttsCtx, ttsCancel := context.WithTimeout(context.Background(), 60*time.Second)
		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			greeting1Audio = getGreetingCachedAudio(ttsCtx, h.ttsService, greetingText1, req.CompanyName, initialLang, req.CustomerName, req.TTSSpeed, "turn1")
		}()

		go func() {
			defer wg.Done()
			greeting2Audio = getGreetingCachedAudio(ttsCtx, h.ttsService, greetingText2, req.CompanyName, initialLang, req.CustomerName, req.TTSSpeed, "turn2")
		}()

		wg.Wait()
		ttsCancel()

		log.Printf("[LEVEL 1D: PRE-SYNTHESIS COMPLETE]\n"+
			"  ├─ Turn 1 Greeting Audio: %d bytes\n"+
			"  └─ Turn 2 Intro Audio:    %d bytes",
			len(greeting1Audio), len(greeting2Audio))
	}

	// Store pre-synthesized audio under preliminaryCallID BEFORE calling MakeCall
	if db.RedisClient != nil {
		configKey := fmt.Sprintf("call_config:%s", preliminaryCallID)
		redisMap := map[string]interface{}{
			"telephony_provider": provider.GetProviderName(),
			"company_name":       req.CompanyName,
			"customer_name":      req.CustomerName,
			"initial_language":   initialLang,
			"to_phone":           req.ToPhone,
			"tts_speed":          fmt.Sprintf("%.2f", req.TTSSpeed),
			"greeting_text_1":    greetingText1,
			"greeting_text_2":    greetingText2,
		}
		if len(greeting1Audio) > 0 {
			redisMap["greeting_audio_1"] = base64.StdEncoding.EncodeToString(greeting1Audio)
		}
		if len(greeting2Audio) > 0 {
			redisMap["greeting_audio_2"] = base64.StdEncoding.EncodeToString(greeting2Audio)
		}
		if req.AmbienceVolume != nil {
			redisMap["ambience_volume"] = fmt.Sprintf("%.2f", *req.AmbienceVolume)
		}
		_ = db.RedisClient.HSet(r.Context(), configKey, redisMap).Err()
		_ = db.RedisClient.Expire(context.Background(), configKey, 24*time.Hour).Err()
		log.Printf("✅ [LEVEL 1E: REDIS PERSISTED] Stored fresh greetings under key %q (TTL: 24h)", configKey)
	}

	outboundReq := telephony.OutboundCallRequest{
		ToPhone:     req.ToPhone,
		FromPhone:   req.FromPhone,
		CallbackURL: req.CallbackURL,
		StreamURL:   req.StreamURL,
		Metadata: map[string]string{
			"company_name":        req.CompanyName,
			"customer_name":       req.CustomerName,
			"telephony_provider":  provider.GetProviderName(),
			"preliminary_call_id": preliminaryCallID, // Passed through to answer URL for Redis lookup
		},
	}

	log.Printf("[LEVEL 1F: TELEPHONY DISPATCH] Sending outbound call via %s to %s...", provider.GetProviderName(), req.ToPhone)

	callCtx, callCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer callCancel()

	// Trigger phone call via telephony provider with an independent 60s timeout
	resp, err := provider.MakeCall(callCtx, outboundReq)
	if err != nil {
		log.Printf("❌ [LEVEL 1F: TELEPHONY ERROR] Error triggering call via %s: %v", provider.GetProviderName(), err)
		http.Error(w, fmt.Sprintf("Failed to initiate call: %v", err), http.StatusInternalServerError)
		return
	}

	resolvedCallID := resp.CallID
	if resolvedCallID == "" {
		resolvedCallID = preliminaryCallID
	}

	log.Printf("✅ [LEVEL 1F: TELEPHONY DISPATCH SUCCESS]\n"+
		"  ├─ Provider:     %s\n"+
		"  ├─ Request UUID: %s\n"+
		"  ├─ Call Status:  %s\n"+
		"  └─ Raw Response: %s",
		resp.Provider, resolvedCallID, resp.Status, resp.RawResponse)

	// Also store a cross-reference under the telephony provider's call ID pointing to preliminaryCallID
	if db.RedisClient != nil && resolvedCallID != preliminaryCallID {
		_ = db.RedisClient.HSet(r.Context(), fmt.Sprintf("call_config:%s", resolvedCallID),
			"preliminary_call_id", preliminaryCallID,
			"telephony_provider", provider.GetProviderName(),
			"company_name", req.CompanyName,
			"customer_name", req.CustomerName,
			"to_phone", req.ToPhone,
			"tts_speed", fmt.Sprintf("%.2f", req.TTSSpeed),
		).Err()
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":       "success",
		"message":      fmt.Sprintf("Outbound call triggered successfully via %s (Greetings Pre-Synthesized)", provider.GetProviderName()),
		"call_id":      resolvedCallID,
		"provider":     resp.Provider,
		"call_status":  resp.Status,
		"raw_response": resp.RawResponse,
	})
}

// HandlePlivoAnswerXML returns the Plivo XML containing WebSocket media stream URL when call is answered
func (h *CallAPIHandler) HandlePlivoAnswerXML(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	callUUID := r.FormValue("CallUUID")
	if callUUID == "" {
		callUUID = r.URL.Query().Get("CallUUID")
	}
	preliminaryCallID := r.FormValue("preliminary_call_id")
	if preliminaryCallID == "" {
		preliminaryCallID = r.URL.Query().Get("preliminary_call_id")
	}
	if callUUID == "" {
		callUUID = preliminaryCallID
	}
	if callUUID == "" {
		callUUID = uuid.New().String()
	}

	log.Printf("\n"+
		"================================================================================\n"+
		"[LEVEL 2: TELEPHONY PROVIDER ANSWER WEBHOOK HIT]\n"+
		"  ├─ Provider:             Plivo (/v1/telephony/plivo/answer)\n"+
		"  ├─ Plivo CallUUID:       %s\n"+
		"  ├─ Preliminary Call ID: %s\n"+
		"  └─ Client Remote Addr:  %s\n"+
		"================================================================================",
		callUUID, preliminaryCallID, r.RemoteAddr)

	if db.RedisClient != nil && callUUID != "" && preliminaryCallID != "" {
		configKey := fmt.Sprintf("call_config:%s", callUUID)
		_ = db.RedisClient.HSet(r.Context(), configKey, "preliminary_call_id", preliminaryCallID).Err()
		_ = db.RedisClient.Expire(r.Context(), configKey, 24*time.Hour).Err()
		log.Printf("🔗 [LEVEL 2B: REDIS BINDING SUCCESS] Linked CallUUID %s -> preliminaryCallID %s in Redis (TTL: 24h)", callUUID, preliminaryCallID)
	}

	wsHost := strings.TrimPrefix(strings.TrimPrefix(config.Settings.ServerPublicHost, "http://"), "https://")
	if wsHost == "" {
		wsHost = "localhost:8000"
	}
	wsScheme := "ws"
	if strings.HasPrefix(config.Settings.ServerPublicHost, "https://") {
		wsScheme = "wss"
	}
	wsURL := fmt.Sprintf("%s://%s/v1/stream/plivo?call_id=%s&preliminary_call_id=%s", wsScheme, wsHost, callUUID, preliminaryCallID)
	xmlWSURL := strings.ReplaceAll(wsURL, "&", "&amp;")

	xml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Response>
    <Stream bidirectional="true" streamTimeout="3600" keepCallAlive="true" contentType="audio/x-mulaw;rate=8000">%s</Stream>
</Response>`, xmlWSURL)

	log.Printf("[LEVEL 2C: ANSWER XML GENERATED] Returning Plivo Stream XML for CallUUID %s:\n%s", callUUID, xml)

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(xml))
}

// HandleTwilioAnswerXML returns the TwiML XML containing WebSocket media stream URL when Twilio call connects
func (h *CallAPIHandler) HandleTwilioAnswerXML(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	callSid := r.FormValue("CallSid")
	if callSid == "" {
		callSid = r.URL.Query().Get("CallSid")
	}
	preliminaryCallID := r.FormValue("preliminary_call_id")
	if preliminaryCallID == "" {
		preliminaryCallID = r.URL.Query().Get("preliminary_call_id")
	}
	if callSid == "" {
		callSid = preliminaryCallID
	}
	if callSid == "" {
		callSid = uuid.New().String()
	}

	wsHost := strings.TrimPrefix(strings.TrimPrefix(config.Settings.ServerPublicHost, "http://"), "https://")
	if wsHost == "" {
		wsHost = "localhost:8000"
	}
	wsScheme := "ws"
	if strings.HasPrefix(config.Settings.ServerPublicHost, "https://") {
		wsScheme = "wss"
	}
	wsURL := fmt.Sprintf("%s://%s/v1/stream/twilio?call_id=%s&preliminary_call_id=%s", wsScheme, wsHost, callSid, preliminaryCallID)
	xmlWSURL := strings.ReplaceAll(wsURL, "&", "&amp;")

	xml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Response>
    <Connect>
        <Stream url="%s" />
    </Connect>
</Response>`, xmlWSURL)

	log.Printf("[Twilio XML] Returning Answer TwiML for CallSid %s:\n%s", callSid, xml)

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(xml))
}

// HandleTelnyxAnswerXML returns TeXML XML containing the WebSocket stream URL when Telnyx call connects
func (h *CallAPIHandler) HandleTelnyxAnswerXML(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	callID := r.FormValue("preliminary_call_id")
	if callID == "" {
		callID = r.URL.Query().Get("preliminary_call_id")
	}
	preliminaryCallID := callID
	if callID == "" {
		callID = r.FormValue("CallSid")
	}
	if callID == "" {
		callID = r.FormValue("CallControlID")
	}
	if callID == "" {
		callID = uuid.New().String()
	}

	wsHost := strings.TrimPrefix(strings.TrimPrefix(config.Settings.ServerPublicHost, "http://"), "https://")
	if wsHost == "" {
		wsHost = "localhost:8000"
	}
	wsScheme := "ws"
	if strings.HasPrefix(config.Settings.ServerPublicHost, "https://") {
		wsScheme = "wss"
	}
	wsURL := fmt.Sprintf("%s://%s/v1/stream/telnyx?call_id=%s&preliminary_call_id=%s", wsScheme, wsHost, callID, preliminaryCallID)
	xmlWSURL := strings.ReplaceAll(wsURL, "&", "&amp;")

	xml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Response>
    <Connect>
        <Stream url="%s" />
    </Connect>
</Response>`, xmlWSURL)

	log.Printf("[Telnyx TeXML] Returning Answer TeXML XML for CallID %s:\n%s", callID, xml)

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(xml))
}

// getPreGeneratedAudio checks for disk-cached pre-rendered audio matching provider-voice_id-tts_speed.
// On Cache HIT, returns pre-synthesized audio in 0ms. On Cache MISS, synthesizes once & stores to disk.
func getPreGeneratedAudio(ctx context.Context, tts *services.TTSService, text string, speed float64) []byte {
	if tts == nil || text == "" {
		return nil
	}
	provider := strings.ToLower(config.Settings.TtsProvider)
	if provider == "" {
		provider = "fishaudio"
	}
	voiceID := config.Settings.FishAudioVoice
	if provider == "cartesia" {
		voiceID = config.Settings.CartesiaVoice
	} else if provider == "openai" {
		voiceID = config.Settings.OpenaiVoice
	} else if provider == "murf" || provider == "murf_cloud" || provider == "murf_falcon2" || provider == "falcon2" || provider == "falcon-2" {
		voiceID = config.Settings.MurfVoice
	}
	if voiceID == "" {
		voiceID = "default"
	}

	cleanText := strings.TrimSpace(text)
	textSlug := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return '_'
	}, cleanText)
	if len(textSlug) > 25 {
		textSlug = textSlug[:25]
	}

	fileName := fmt.Sprintf("%s-%s-%.2f-%s.mulaw", provider, voiceID, speed, textSlug)
	cacheDir := filepath.Join("audio", "pregenerated")
	_ = os.MkdirAll(cacheDir, 0755)
	cachePath := filepath.Join(cacheDir, fileName)

	// 1. Check Disk Cache (0ms Latency)
	if data, err := os.ReadFile(cachePath); err == nil && len(data) > 0 {
		log.Printf("⚡⚡ [DISK PRE-GEN CACHE HIT] %s (%d bytes)", fileName, len(data))
		return data
	}

	// 2. Cache Miss: Synthesize & save to disk
	audio, err := tts.SynthesizeSpeechMulawWithSpeed(ctx, text, speed)
	if err == nil && len(audio) > 0 {
		_ = os.WriteFile(cachePath, audio, 0644)
		log.Printf("💾 [DISK PRE-GEN CACHE SAVED] Synthesized & cached: %s (%d bytes)", fileName, len(audio))
		return audio
	}
	return nil
}

func getGreetingCachedAudio(ctx context.Context, tts *services.TTSService, text string, companyName string, initialLang string, customerName string, speed float64, turnName string) []byte {
	if tts == nil || strings.TrimSpace(text) == "" {
		return nil
	}
	provider := strings.ToLower(config.Settings.TtsProvider)
	if provider == "" {
		provider = "fishaudio"
	}
	voiceID := config.Settings.FishAudioVoice
	if provider == "cartesia" {
		voiceID = config.Settings.CartesiaVoice
	} else if provider == "openai" {
		voiceID = config.Settings.OpenaiVoice
	} else if provider == "murf" || provider == "murf_cloud" || provider == "murf_falcon2" || provider == "falcon2" || provider == "falcon-2" {
		voiceID = config.Settings.MurfVoice
	}
	if voiceID == "" {
		voiceID = "default"
	}

	cleanCustomer := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return '_'
	}, strings.TrimSpace(customerName))
	if cleanCustomer == "" {
		cleanCustomer = "valued_customer"
	}

	cacheDir := filepath.Join("audio", "greetings", provider, voiceID, companyName, initialLang)
	_ = os.MkdirAll(cacheDir, 0755)

	fileName := fmt.Sprintf("%s_speed_%.2f_%s.mulaw", cleanCustomer, speed, turnName)
	filePath := filepath.Join(cacheDir, fileName)

	// 1. Check Disk Cache (0ms Latency HIT)
	if data, err := os.ReadFile(filePath); err == nil && len(data) > 0 {
		log.Printf("⚡⚡ [GREETING DISK CACHE HIT] %s (%d bytes)", filePath, len(data))
		return data
	}

	// 2. Cache Miss: Synthesize & save to disk for future calls
	audio, err := tts.SynthesizeSpeechMulawWithSpeed(ctx, text, speed)
	if err == nil && len(audio) > 0 {
		_ = os.WriteFile(filePath, audio, 0644)
		log.Printf("💾 [GREETING DISK CACHE SAVED] Synthesized & cached to disk: %s (%d bytes)", filePath, len(audio))
		return audio
	}
	return nil
}
