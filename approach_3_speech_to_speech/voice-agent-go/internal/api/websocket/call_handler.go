package websocket

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"voice-agent-go/internal/config"
	"voice-agent-go/internal/db"
	"voice-agent-go/internal/services"
	"voice-agent-go/internal/services/rag"
	"voice-agent-go/internal/services/stt"
	"voice-agent-go/internal/services/telephony"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow cross-origin WebSocket connections for dev
	},
}

// Twilio WebSocket incoming message format
type TwilioMessage struct {
	Event string `json:"event"`
	Media struct {
		Payload string `json:"payload"`
	} `json:"media"`
	Start struct {
		CallSid string `json:"callSid"`
	} `json:"start"`
}

type CallHandler struct {
	sessionService     *services.SessionService
	emotionService     *services.EmotionService
	ragProvider        rag.RAGProvider
	recorderService    *services.AudioRecorderService
	ttsService         *services.TTSService
	backchannelService *services.BackchannelService
	fillerService      *services.FillerService
	sttProvider        stt.STTProvider
	httpClient         *http.Client
}

func NewCallHandler(sessionService *services.SessionService, emotionService *services.EmotionService, ragProvider rag.RAGProvider, sttProvider stt.STTProvider) *CallHandler {
	ttsSvc := services.NewTTSService()
	return &CallHandler{
		sessionService:     sessionService,
		emotionService:     emotionService,
		ragProvider:        ragProvider,
		recorderService:    services.NewAudioRecorderService("D:/Voice AI/recordings"),
		ttsService:         ttsSvc,
		backchannelService: services.NewBackchannelService(ttsSvc),
		fillerService:      services.NewFillerService(ttsSvc),
		sttProvider:        sttProvider,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (h *CallHandler) HandleCallStream(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("❌ [LEVEL 3 ERROR] WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	startTime := time.Now()
	urlPrelimID := r.URL.Query().Get("preliminary_call_id")
	urlCallID := r.URL.Query().Get("call_id")

	callDirection := "inbound"
	if urlPrelimID != "" {
		callDirection = "outbound"
	}

	log.Printf("\n"+
		"================================================================================\n"+
		"[LEVEL 3: WEBSOCKET MEDIA STREAM CONNECTED]\n"+
		"  ├─ Endpoint:            %s\n"+
		"  ├─ WS Query CallID:     %s\n"+
		"  ├─ WS Query PrelimID:   %s\n"+
		"  ├─ Call Direction:      %s\n"+
		"  └─ Client Remote Addr:  %s\n"+
		"================================================================================",
		r.URL.Path, urlCallID, urlPrelimID, callDirection, r.RemoteAddr)

	var (
		sessionID           string
		phone               = ""
		companyName         = config.Settings.DefaultCompanyName
		tenantIDStr         = "00000000-0000-0000-0000-000000000001"
		customerName        = config.Settings.DefaultCustomerName
		initialLang         = config.Settings.InitialLanguage
		ttsSpeed            = config.Settings.TtsSpeed
		mu                  sync.Mutex
		audioBuffer         []byte
		dynamicMemories     []string
		callHistory         []map[string]string
		greetingSent        bool
		stopChan            = make(chan struct{})
		wg                  sync.WaitGroup
		recorder            *services.CallSessionRecorder
		pacer               *services.MediaPacer
		sttBuffer           []byte
		preSpeechRingBuffer []byte
		lastSpeechTime      time.Time
		lastTranscriptTime  time.Time = time.Now()
		lastBackchannelTime time.Time = time.Now().Add(-10 * time.Second)
		nextBackchannelGap  time.Duration = time.Duration(8+rand.Intn(5)) * time.Second
		isSpeaking          bool
		speechFrameCount    int
		nudgeSent           bool
		sessionNudgeClips   [][]byte
		userTurnCount       int
		firstMediaReceived  bool
	)

	defer func() {
		sessDuration := time.Since(startTime)
		log.Printf("\n"+
			"================================================================================\n"+
			"[LEVEL 4B: WEBSOCKET SESSION ENDED / DISCONNECTED]\n"+
			"  ├─ Session ID (CallUUID): %s\n"+
			"  ├─ Total Call Duration:   %v\n"+
			"  ├─ Customer Name:         %s\n"+
			"  ├─ Company Name:          %s\n"+
			"  └─ Close Reason:          WebSocket stream closed by remote peer / completed\n"+
			"================================================================================",
			sessionID, sessDuration, customerName, companyName)
	}()

	// Main processing channels
	audioChan := make(chan []byte, 100)
	transcriptChan := make(chan string, 10)

	// Inactivity Nudge Goroutine: If caller stays quiet for >=5s after agent finishes speaking (based on real Deepgram text, ignoring background noise)
	wg.Add(1)
	go func() {
		defer wg.Done()
		nudgeTicker := time.NewTicker(1 * time.Second)
		defer nudgeTicker.Stop()

		for {
			select {
			case <-stopChan:
				return
			case <-nudgeTicker.C:
				mu.Lock()
				lastInteraction := lastTranscriptTime
				if pacer != nil {
					agentLastSent := pacer.LastAudioSentTime()
					if agentLastSent.After(lastInteraction) {
						lastInteraction = agentLastSent
					}
				}
				quietDuration := time.Since(lastInteraction)
				agentPlaying := pacer != nil && pacer.IsPlaying()
				userSpoke := isSpeaking
				alreadyNudged := nudgeSent
				hasGreeting := greetingSent
				mu.Unlock()

				if hasGreeting && !userSpoke && !agentPlaying && !alreadyNudged && quietDuration >= 10*time.Second {
					mu.Lock()
					nudgeSent = true
					clipsCount := len(sessionNudgeClips)
					var chosenClip []byte
					if clipsCount > 0 {
						chosenClip = sessionNudgeClips[rand.Intn(clipsCount)]
					}
					mu.Unlock()

					if len(chosenClip) > 0 && pacer != nil {
						pacer.PushAudio(chosenClip)
						log.Printf("[Inactivity Nudge] User silent for 10s. Pushed pre-rendered 0ms low-volume nudge clip!")
					}
				}
			}
		}
	}()

	// Goroutine 1: Continuous Audio Emotion sliding window classification via C++
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				mu.Lock()
				if len(audioBuffer) == 0 {
					mu.Unlock()
					continue
				}
				// Copy sliding window buffer for CIE without clearing main accumulator
				chunkSize := len(audioBuffer)
				if chunkSize > 3200 {
					chunkSize = 3200
				}
				chunk := make([]byte, chunkSize)
				copy(chunk, audioBuffer[len(audioBuffer)-chunkSize:])
				mu.Unlock()

				// Send to C++ ML server or cloud provider
				if h.emotionService != nil {
					scores, err := h.emotionService.ClassifyAudio(chunk, sessionID)
					if err != nil {
						log.Printf("[WebSocket CIE] Audio classification error: %v", err)
					} else {
						log.Printf("[WebSocket CIE] Audio classification -> Anger: %.2f, Joy: %.2f, Neutral: %.2f",
							scores.Anger, scores.Joy, scores.Neutral)
					}
				}
			case <-stopChan:
				return
			}
		}
	}()

	// Goroutine 2: Text Emotion classification and Local RAG context indexing via C++
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case text, ok := <-transcriptChan:
				if !ok {
					return
				}
				// 1. Classify sentiment
				if h.emotionService != nil {
					scores, err := h.emotionService.ClassifyText(text, sessionID)
					if err != nil {
						log.Printf("[WebSocket CIE] Text classification error: %v", err)
					} else {
						log.Printf("[WebSocket CIE] Text sentiment -> Anger: %.2f, Joy: %.2f, Neutral: %.2f",
							scores.Anger, scores.Joy, scores.Neutral)
					}
				}

				// 2. Perform RAG search on C++
				results, err := h.ragProvider.Search(sessionID, text, tenantIDStr)
				if err != nil {
					log.Printf("[WebSocket RAG] C++ RAG Search error: %v", err)
				} else {
					log.Printf("[WebSocket RAG] Search query: \"%s\" -> %d document snippets retrieved.", text, len(results))
					for _, res := range results {
						log.Printf("  -> [Match (Score: %.2f)] %s", res.Score, res.Text)
					}
				}

				// 3. Accumulate memory if user states preference
				if len(text) > 10 { // Mock trigger logic for memory ingestion
					memoryText := "Customer mentioned: " + text
					mu.Lock()
					dynamicMemories = append(dynamicMemories, memoryText)
					mu.Unlock()

					// Dynamically index on C++ server RAM
					_ = h.ragProvider.IndexDynamic(sessionID, memoryText, "live_transcript", tenantIDStr)
				}
			case <-stopChan:
				return
			}
		}
	}()

	var activeStreamSid string
	var sttSession stt.STTSession
	var sttListenerStarted bool
	var introHandled bool
	var introPhaseComplete bool = (callDirection == "inbound")
	var lastUserQueryText string
	var lastUserQueryTime time.Time
	var activeTurnCancel context.CancelFunc
	var activeTurnMu sync.Mutex
	var triggerTurn func(transcriptText string, confidence float64, rawSpeech []byte, rawWav []byte)

	// Cleanup on loop exit
	defer func() {
		if pacer != nil {
			pacer.Stop()
		}
		close(stopChan)
		close(audioChan)
		close(transcriptChan)
		wg.Wait()
		if recorder != nil {
			recorder.Close()
		}
		if sttSession != nil {
			sttSession.Close()
		}

		if sessionID != "" {
			h.sessionService.EndSession(sessionID, phone, tenantIDStr, dynamicMemories)
		}
		log.Println("[WebSocket] Telephony media stream socket closed.")
	}()

	// Query telephony provider & call_id from URL for strict session isolation
	providerName := r.URL.Query().Get("provider")
	if providerName == "" {
		pathSid := r.PathValue("call_sid")
		if pathSid == "plivo" || pathSid == "twilio" || pathSid == "exotel" || pathSid == "telnyx" {
			providerName = pathSid
		}
	}
	urlCallID = r.URL.Query().Get("call_id")
	if urlCallID != "" {
		sessionID = urlCallID
	}

	provider, err := telephony.GetFactory().GetProvider(providerName)
	if err != nil {
		log.Printf("[WebSocket] Warning: Could not resolve requested provider %q: %v. Defaulting to Plivo.", providerName, err)
		provider, _ = telephony.GetFactory().GetProvider("plivo")
	}

	log.Printf("[WebSocket] Active stream parser: %s (Call Session ID: %s)", provider.GetProviderName(), sessionID)

	// Main loop: Parse incoming vendor WebSocket message payloads dynamically
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("[WebSocket] Connection read error: %v", err)
			break
		}

		if len(message) < 300 {
			log.Printf("[Plivo Inbound WS] Raw frame: %s", string(message))
			if services.GlobalKafkaAudioService != nil {
				services.GlobalKafkaAudioService.PublishLog(sessionID, "PLIVO_INBOUND", "INFO", string(message))
			}
		} else {
			log.Printf("[Plivo Inbound WS] Media frame received (%d bytes)", len(message))
		}

		frame, err := provider.ParseStreamMessage(message)
		if err != nil {
			log.Printf("[WebSocket] Message parsing warning: %v", err)
			continue
		}

		if frame.StreamSid != "" {
			activeStreamSid = frame.StreamSid
			if pacer != nil {
				pacer.SetStreamSid(frame.StreamSid)
			}
		}

		// Ensure session, media pacer, and Turn 1 sales greeting are initialized on first frame (EventStart or EventMedia)
		if !greetingSent && (frame.Event == telephony.EventStart || frame.Event == telephony.EventMedia) {
			greetingSent = true
			if frame.SessionID != "" {
				sessionID = frame.SessionID
			} else if sessionID == "" {
				sessionID = provider.GetProviderName() + "_call_" + time.Now().Format("20060102_150405")
			}
			log.Printf("[LEVEL 1: SESSION START] Call stream started via %s. Session ID: %s", provider.GetProviderName(), sessionID)

			if services.GlobalKafkaAudioService != nil {
				services.GlobalKafkaAudioService.PublishAudioFrame(sessionID, "system", nil, "start")
				services.GlobalKafkaAudioService.PublishLog(sessionID, "SESSION", "INFO", "Call session started via "+provider.GetProviderName())
			}

			// Initialize Session Recorder for dual-channel audio storage
			if h.recorderService != nil && recorder == nil {
				rec, err := h.recorderService.StartSessionRecorder(sessionID)
				if err != nil {
					log.Printf("[Audio Recorder] Error starting recorder: %v", err)
				} else {
					recorder = rec
				}
			}

			// Initialize Media Pacer for 20ms RTP packetization
			if pacer == nil {
				pacer = services.NewMediaPacer(sessionID, activeStreamSid, provider, conn, &mu)
				pacer.Start()
			}

			// Initialize DB call record and RAG index
			_, err := h.sessionService.CreateSession(sessionID, companyName, tenantIDStr, phone, callDirection)
			if err != nil {
				log.Printf("[LEVEL 1: WARNING] Session setup note: %v", err)
			}

			// Dynamically load call metadata (customer_name, company_name, tts_speed, ambience_volume) from Redis session config
			if db.RedisClient != nil && sessionID != "" {
				urlPrelimID := r.URL.Query().Get("preliminary_call_id")
				if urlPrelimID != "" {
					sessCfgKey := fmt.Sprintf("call_config:%s", sessionID)
					_ = db.RedisClient.HSet(context.Background(), sessCfgKey, "preliminary_call_id", urlPrelimID).Err()
					_ = db.RedisClient.Expire(context.Background(), sessCfgKey, 24*time.Hour).Err()
					log.Printf("🔗 [WS CONNECT] Bound sessionID %s -> preliminaryCallID %s from WS URL", sessionID, urlPrelimID)
				}

				// Resolve the canonical Redis key (preliminaryCallID set during POST /v1/calls/outbound)
				cfgKey := sessionID
				if refID, err := db.RedisClient.HGet(context.Background(), fmt.Sprintf("call_config:%s", sessionID), "preliminary_call_id").Result(); err == nil && refID != "" {
					cfgKey = refID
				}
				configKey := fmt.Sprintf("call_config:%s", cfgKey)

				if val, err := db.RedisClient.HGet(context.Background(), configKey, "customer_name").Result(); err == nil && val != "" {
					customerName = val
				}
				if val, err := db.RedisClient.HGet(context.Background(), configKey, "company_name").Result(); err == nil && val != "" {
					companyName = val
				}
				if val, err := db.RedisClient.HGet(context.Background(), configKey, "initial_language").Result(); err == nil && val != "" {
					initialLang = strings.ToLower(val)
				}
				if val, err := db.RedisClient.HGet(context.Background(), configKey, "tts_speed").Result(); err == nil && val != "" {
					if s, err := strconv.ParseFloat(val, 64); err == nil && s > 0 {
						ttsSpeed = s
						log.Printf("[Call Handler] Loaded dynamic TTS speed from API request: %.2f", ttsSpeed)
					}
				}
				if val, err := db.RedisClient.HGet(context.Background(), configKey, "ambience_volume").Result(); err == nil && val != "" {
					if vol, err := strconv.ParseFloat(val, 64); err == nil && vol >= 0 {
						if pacer != nil {
							pacer.SetAmbienceVolume(vol)
						}
						log.Printf("🔒 [Call Handler] Loaded CONSTANT Ambience Volume from API payload: %.2f (Session %s)", vol, sessionID)
					}
				}
			}
				var greetingText string
				var preSynthesized1 []byte

				// [FIX Issue 1] Resolve the canonical Redis key for pre-synthesized audio.
				// The audio is stored under preliminaryCallID (set before MakeCall).
				// sessionID here may be the Plivo CallUUID, which differs from preliminaryCallID.
				// Check for a cross-reference: call_config:<sessionID> → preliminary_call_id → call_config:<preliminaryCallID>
				preSynthKey := sessionID
				if db.RedisClient != nil && sessionID != "" {
					configKey := fmt.Sprintf("call_config:%s", sessionID)
					if refID, err := db.RedisClient.HGet(context.Background(), configKey, "preliminary_call_id").Result(); err == nil && refID != "" {
						preSynthKey = refID
						log.Printf("🔗 [PRE-SYNTHESIS KEY RESOLVED] sessionID %q → preliminaryCallID %q", sessionID, refID)
					}
				}

				// Check Redis for pre-synthesized Turn 1 greeting audio from outbound API
				if db.RedisClient != nil && preSynthKey != "" {
					configKey := fmt.Sprintf("call_config:%s", preSynthKey)
					// [FIX Issue 5] Use base64 decoding (matching base64 encoding in call_api_handler)
					if b64Str, err := db.RedisClient.HGet(context.Background(), configKey, "greeting_audio_1").Result(); err == nil && b64Str != "" {
						if b, err := base64.StdEncoding.DecodeString(b64Str); err == nil && len(b) > 0 {
							preSynthesized1 = b
							log.Printf("⚡⚡ [0ms PRE-SYNTHESIS HIT] Found pre-rendered Turn 1 greeting audio in Redis (%d bytes)", len(preSynthesized1))
						}
					}
					if txt, err := db.RedisClient.HGet(context.Background(), configKey, "greeting_text_1").Result(); err == nil && txt != "" {
						greetingText = txt
					}
				}

				if greetingText == "" {
					greetingText = services.GetPromptService().GetGreeting("inbound", companyName, customerName, initialLang)
				}

				log.Printf("[LEVEL 1: INBOUND SUPPORT GREETING] Customer connected. Playing support greeting: %q", greetingText)

				mu.Lock()
				callHistory = append(callHistory, map[string]string{
					"role":    "assistant",
					"content": greetingText,
				})
				mu.Unlock()

				go func(audio1 []byte, text1 string) {
					time.Sleep(1000 * time.Millisecond) // 1s delay: allow caller to put phone to ear
					if len(audio1) > 0 {
						log.Printf("🚀 [0ms INSTANT PLAYOUT] Playing pre-rendered Turn 1 greeting (%d bytes) with 0ms latency!", len(audio1))
						if pacer != nil {
							pacer.PushAudio(audio1)
						}
						if recorder != nil {
							recorder.WriteAgentAudio(audio1)
							recorder.WriteTranscript("agent", text1)
						}
						if services.GlobalKafkaAudioService != nil {
							services.GlobalKafkaAudioService.PublishAudioFrame(sessionID, "agent", audio1, "media")
							services.GlobalKafkaAudioService.PublishTranscript(sessionID, "agent", text1)
						}
						return
					}

					// Dynamic TTS fallback if pre-synthesis missing
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					mulawBytes, err := h.ttsService.SynthesizeSpeechStreamWithSpeed(ctx, text1, ttsSpeed, func(chunk []byte) {
						if pacer != nil && len(chunk) > 0 {
							pacer.PushAudio(chunk)
						}
					})
					if err == nil && len(mulawBytes) > 0 {
						log.Printf("[TTS RAW OUTPUT] Text: %q -> Generated %d mu-law audio bytes.", text1, len(mulawBytes))
						if recorder != nil {
							recorder.WriteAgentAudio(mulawBytes)
							recorder.WriteTranscript("agent", text1)
						}
						if services.GlobalKafkaAudioService != nil {
							services.GlobalKafkaAudioService.PublishAudioFrame(sessionID, "agent", mulawBytes, "media")
							services.GlobalKafkaAudioService.PublishTranscript(sessionID, "agent", text1)
						}
					} else if err != nil {
						log.Printf("[LEVEL 1: TTS ERROR] Failed to synthesize greeting: %v", err)
					}
				}(preSynthesized1, greetingText)

				// Pre-synthesize personalized low-volume English nudge clips ONCE at call start for 0ms latency
				go func(name string) {
					phrases := []string{
						fmt.Sprintf("Hey %s, are you there?", name),
						fmt.Sprintf("Hello %s, can you hear me?", name),
						fmt.Sprintf("Am I still audible to you, %s?", name),
						fmt.Sprintf("Are you still on the line, %s?", name),
					}
					if name == "" || name == "there" || name == "Valued Customer" {
						phrases = []string{
							"Hey, are you there?",
							"Hello, can you hear me?",
							"Am I still audible to you?",
							"Are you still on the line?",
						}
					}

					var compiledClips [][]byte
					ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
					defer cancel()

					for _, phrase := range phrases {
						audio, err := h.ttsService.SynthesizeSpeechMulaw(ctx, phrase)
						if err == nil && len(audio) > 0 {
							compiledClips = append(compiledClips, audio)
						}
					}

					if len(compiledClips) > 0 {
						mu.Lock()
						sessionNudgeClips = compiledClips
						mu.Unlock()
						log.Printf("[Session Pre-render] Pre-synthesized %d personalized 100%% volume nudge clips for session!", len(compiledClips))
					}
				}(customerName)
		}

		switch frame.Event {
		case telephony.EventStart:
			// Session start handled in first frame initialization block above

		case telephony.EventMedia:
			if frame.PayloadBase64 == "" {
				continue
			}
			payloadBytes, err := base64.StdEncoding.DecodeString(frame.PayloadBase64)
			if err != nil {
				log.Printf("[LEVEL 2: AUDIO ERROR] Base64 decode error: %v", err)
				continue
			}

			if !firstMediaReceived {
				firstMediaReceived = true
				log.Printf("⚡ [LEVEL 4A: FIRST MEDIA FRAME RECEIVED] Telephony provider sent first audio payload (%d bytes) for session: %s", len(payloadBytes), sessionID)
			}

			// Apply Dynamic Range Compression (DRC) to amplify whispered/soft speech by up to 3.5x (+11dB)
			payloadBytes = services.ApplyDynamicGain(payloadBytes)

			// Publish Customer audio frame to Kafka Worker
			if services.GlobalKafkaAudioService != nil {
				services.GlobalKafkaAudioService.PublishAudioFrame(sessionID, "customer", payloadBytes, "media")
			}

			// Ensure recorder is lazily created if EventStart frame was missing sessionID
			if recorder == nil && h.recorderService != nil {
				if sessionID == "" {
					if frame.SessionID != "" {
						sessionID = frame.SessionID
					} else {
						sessionID = "plivo_call_" + time.Now().Format("20060102_150405")
					}
				}
				rec, err := h.recorderService.StartSessionRecorder(sessionID)
				if err == nil {
					recorder = rec
				}
			}

			// Store customer audio chunk directly into session recording
			if recorder != nil {
				recorder.WriteCustomerAudio(payloadBytes)
			}

			// Lazily initialize Streaming STT WebSocket session if STT_MODE == "streaming"
			if config.Settings.SttMode == "streaming" && sttSession == nil && sessionID != "" && h.sttProvider != nil && h.sttProvider.RequiresServerProcessing() {
				sess, err := h.sttProvider.NewSession(sessionID)
				if err != nil {
					log.Printf("[STT Streaming ERROR] Failed to connect to %s WS: %v. Falling back to batch mode.", h.sttProvider.Type(), err)
				} else {
					sttSession = sess
					log.Printf("🎙️ [STT Streaming ENABLED] Real-time %s WebSocket active for session: %s", h.sttProvider.Type(), sessionID)

					// Start background listener goroutine to process incoming transcripts from STT provider
					if !sttListenerStarted {
						sttListenerStarted = true
						wg.Add(1)
						go func(s stt.STTSession, sid string) {
							defer wg.Done()
							log.Printf("⚡⚡ [%s WS REALTIME LISTENER ACTIVE] Full-duplex barge-in listener active for session: %s", strings.ToUpper(h.sttProvider.Type()), sid)

							var (
								stitchedText  string
								highestConf   float64
								debounceTimer *time.Timer
								debounceMu    sync.Mutex
							)

							for res := range s.Transcripts() {
								cleanText := strings.TrimSpace(res.Text)
								if cleanText == "" {
									continue
								}
								log.Printf("🎙️ [%s WS REALTIME STREAM] Transcript (IsFinal=%v, Conf=%.2f): %q", strings.ToUpper(h.sttProvider.Type()), res.IsFinal, res.Confidence, cleanText)

								if triggerTurn == nil {
									continue
								}

								// Turn 1 confirmation: Trigger EXACTLY ONCE on the first speech response to Greeting 1
								if !introHandled {
									introHandled = true
									log.Printf("⚡ [INTRO TURN 1 TRIGGERED] First speech response received: %q. Launching Turn 2 intro pitch.", cleanText)
									triggerTurn(cleanText, res.Confidence, nil, nil)
									continue
								}

								// While intro phase is active (!introPhaseComplete), strictly DROP all incoming user speech!
								// Riya's intro pitch (Turn 2) plays 100% uninterrupted from start to finish. ZERO barge-in during intro!
								mu.Lock()
								isIntroActive := !introPhaseComplete
								mu.Unlock()

								if isIntroActive {
									log.Printf("🛑 [INTRO PHASE ACTIVE] Transcript %q ignored while Turn 2 intro pitch is playing. Waiting for intro pitch to complete!", cleanText)
									continue
								}

								// POST-INTRO TURNS (Turn 3+): True Full-Duplex Barge-In Interruption!
								// If customer speaks while agent is speaking/playing audio, STOP AGENT IMMEDIATELY!
								if pacer != nil && (pacer.IsSpeaking() || pacer.IsPlaying()) {
									log.Printf("🛑 [BARGE-IN ACTIVATED] Customer interrupted agent speech (%q). Halting active playback immediately!", cleanText)
									pacer.Flush()
									pacer.ClearBuffer()

									activeTurnMu.Lock()
									if activeTurnCancel != nil {
										activeTurnCancel()
										activeTurnCancel = nil
									}
									activeTurnMu.Unlock()
								}

								// Accumulate & Stitch Utterances with 1-Second Debounce (1s Gap)
								if res.IsFinal {
									debounceMu.Lock()
									if stitchedText != "" {
										stitchedText += " " + cleanText
									} else {
										stitchedText = cleanText
									}
									if res.Confidence > highestConf {
										highestConf = res.Confidence
									}

									if debounceTimer != nil {
										debounceTimer.Stop()
									}

									// Wait 1000ms (1 second) of silence gap after speech to process merged query
									debounceTimer = time.AfterFunc(1000*time.Millisecond, func() {
										debounceMu.Lock()
										textToTrigger := stitchedText
										confToTrigger := highestConf
										stitchedText = ""
										highestConf = 0.0
										debounceMu.Unlock()

										if textToTrigger != "" {
											log.Printf("💬 [1s GAP UTTERANCE READY] Stitched 1s debounced query for session %s: %q (Conf: %.0f%%)", sessionID, textToTrigger, confToTrigger*100)
											triggerTurn(textToTrigger, confToTrigger, nil, nil)
										}
									})
									debounceMu.Unlock()
								}
							}
						}(sttSession, sessionID)
					}
				}
			}

			// Forward audio chunk continuously to Deepgram Streaming WebSocket (Full-Duplex — 0ms WAV conversion)
			if sttSession != nil {
				_ = sttSession.SendAudio(payloadBytes)
			}

			// VAD: Detect voice activity & trigger natural active listening backchannels during long continuous speech
			if services.IsSpeechFrame(payloadBytes) {
				speechFrameCount++
				lastSpeechTime = time.Now()
				if speechFrameCount >= 5 {
					isSpeaking = true
					nudgeSent = false
				}
				// Natural Backchannel: Trigger when user speaks continuously for >= 150 frames (~3s) with a 4s cooldown
				if speechFrameCount >= 150 && speechFrameCount%150 == 0 {
					if time.Since(lastBackchannelTime) >= 4*time.Second && h.backchannelService != nil && pacer != nil {
						clip := h.backchannelService.GetRandomClip()
						if len(clip) > 0 {
							pacer.PushAudio(clip)
							lastBackchannelTime = time.Now()
							log.Printf("🎙️ [NATURAL BACKCHANNEL] User speaking continuously (~%.1fs) → Pushed soft active listening cue", float64(speechFrameCount)*0.02)
						}
					}
				}
			} else {
				speechFrameCount = 0
			}

			// Maintain rolling 300ms pre-speech ring buffer (2400 bytes of 8kHz mulaw) to prevent initial syllable clipping
			mu.Lock()
			if !isSpeaking {
				preSpeechRingBuffer = append(preSpeechRingBuffer, payloadBytes...)
				if len(preSpeechRingBuffer) > 2400 {
					preSpeechRingBuffer = preSpeechRingBuffer[len(preSpeechRingBuffer)-2400:]
				}
			} else if len(sttBuffer) == 0 && len(preSpeechRingBuffer) > 0 {
				sttBuffer = append(sttBuffer, preSpeechRingBuffer...)
				preSpeechRingBuffer = preSpeechRingBuffer[:0]
			}

			sttBuffer = append(sttBuffer, payloadBytes...)
			audioBuffer = append(audioBuffer, payloadBytes...)
			if len(audioBuffer) > 16000 {
				audioBuffer = audioBuffer[len(audioBuffer)-16000:] // Keep max 2s sliding window for CIE
			}

			// Trigger STT when:
			// 1. User spoke AND paused (250ms on Turn 1 for fast confirmation, 650ms for mid-call turns) AND accumulated >= 1600 bytes
			// 2. OR buffer reached 48000 bytes (6s max cap)
			pauseWindow := 650 * time.Millisecond
			if userTurnCount == 0 {
				pauseWindow = 250 * time.Millisecond // Fast 250ms endpointing on Turn 1!
			}
			shouldProcess := isSpeaking && time.Since(lastSpeechTime) >= pauseWindow && len(sttBuffer) >= 1600 || len(sttBuffer) >= 48000
			var speechBytes []byte
			if shouldProcess {
				speechBytes = make([]byte, len(sttBuffer))
				copy(speechBytes, sttBuffer)
				sttBuffer = sttBuffer[:0]
				isSpeaking = false
			}
			mu.Unlock()

			// Unified function to handle user turn (works for both Streaming WS & Batch REST)
			triggerTurn = func(transcriptText string, confidence float64, rawSpeech []byte, rawWav []byte) {
				cleanText := strings.TrimSpace(transcriptText)
				if cleanText == "" || len(cleanText) < 2 {
					log.Printf("[Call STT] Empty or noise transcript returned (%q)", transcriptText)
					return
				}

				// Preempt & cancel any previous active LLM/TTS turn context in flight
				activeTurnMu.Lock()
				if activeTurnCancel != nil {
					log.Printf("🚨 [TURN PREEMPTION] Cancelling active in-flight LLM/TTS turn context for session %s", sessionID)
					activeTurnCancel()
				}
				turnCtx, turnCancel := context.WithCancel(context.Background())
				activeTurnCancel = turnCancel
				activeTurnMu.Unlock()

				// Instant Mid-Speech Barge-In: Flush any audio currently playing in user's earpiece
				if pacer != nil {
					pacer.Flush()
				}

				// TIER 3: Low Confidence (< 0.50) -> Fast Local Repeat Cue Without Calling LLM / RAG!
				if confidence > 0 && confidence < 0.50 {
					log.Printf("⚠️ [ASR CONFIDENCE TIER 3] Low speech certainty (%.0f%% < 50%%) for %q. Fast-triggering repeat prompt (Skipping LLM).", confidence*100, cleanText)
					if services.GlobalKafkaAudioService != nil {
						services.GlobalKafkaAudioService.PublishLog(sessionID, "STT_CONFIDENCE", "WARN", fmt.Sprintf("⚠️ Low ASR Probability (%.0f%%) for %q -> Fast local repeat (Skipped LLM)", confidence*100, cleanText))
					}
					repeatPrompt := "Ma'am, I couldn't hear that clearly. Could you please repeat?"
					if customerName != "" && customerName != "there" && customerName != "Valued Customer" {
						repeatPrompt = fmt.Sprintf("Sorry %s, I couldn't hear that clearly. Could you please repeat?", customerName)
					}

					mu.Lock()
					lastTranscriptTime = time.Now()
					nudgeSent = false
					mu.Unlock()

					if recorder != nil {
						recorder.WriteTranscript("user", fmt.Sprintf("[Muffled Audio: %s (conf: %.0f%%)]", cleanText, confidence*100))
						recorder.WriteTranscript("agent", repeatPrompt)
					}

					go func() {
						ttsCtx, ttsCancel := context.WithTimeout(turnCtx, 5*time.Second)
						defer ttsCancel()
						repeatAudio, ttsErr := h.ttsService.SynthesizeSpeechMulaw(ttsCtx, repeatPrompt)
						if ttsErr == nil && len(repeatAudio) > 0 && pacer != nil {
							pacer.PushAudio(repeatAudio)
						}
					}()
					return
				}

				// Real speech confirmed! (Tier 1 >= 0.85 or Tier 2 0.50–0.85)
				now := time.Now()
				isQueryAggregation := false
				mu.Lock()
				lastTranscriptTime = now
				nudgeSent = false

				// DYNAMIC QUERY AGGREGATION: Active ONLY AFTER intro pitch completes (introPhaseComplete == true)!
				if introPhaseComplete && lastUserQueryText != "" && now.Sub(lastUserQueryTime) <= 2500*time.Millisecond {
					isQueryAggregation = true
					mergedText := strings.TrimSpace(lastUserQueryText + " " + cleanText)
					log.Printf("🧠 [QUERY AGGREGATION] Merged rapid user speech into single combined query: %q", mergedText)
					cleanText = mergedText

					if len(callHistory) > 0 && callHistory[len(callHistory)-1]["role"] == "user" {
						callHistory[len(callHistory)-1]["content"] = cleanText
					} else {
						callHistory = append(callHistory, map[string]string{"role": "user", "content": cleanText})
					}
				} else {
					callHistory = append(callHistory, map[string]string{"role": "user", "content": cleanText})
				}
				lastUserQueryText = cleanText
				lastUserQueryTime = now
				mu.Unlock()

				// Dynamically Index Customer Speech into Session Knowledge Base (C++ / Cloud Vector RAG)
				if h.ragProvider != nil {
					go func(session string, userText string, tenant string) {
						memoryEntry := fmt.Sprintf("Customer Statement: %q", userText)
						err := h.ragProvider.IndexDynamic(session, memoryEntry, "live_customer_speech", tenant)
						if err == nil {
							log.Printf("🧠 [LIVE KB INDEXED] Stored customer detail into Knowledge Base: %q", memoryEntry)
						} else {
							log.Printf("⚠️ [LIVE KB INDEXING NOTE] Dynamic index: %v", err)
						}
					}(sessionID, cleanText, tenantIDStr)
				}

				// Play soft low-voice active listening backchannel
				mu.Lock()
				agentLastSent := time.Now()
				if pacer != nil {
					agentLastSent = pacer.LastAudioSentTime()
				}
				refTime := agentLastSent
				if lastBackchannelTime.After(refTime) {
					refTime = lastBackchannelTime
				}

				if nextBackchannelGap <= 0 {
					nextBackchannelGap = time.Duration(8+rand.Intn(5)) * time.Second
				}

				canPlayBackchannel := time.Since(refTime) >= nextBackchannelGap
				if canPlayBackchannel {
					lastBackchannelTime = time.Now()
					nextBackchannelGap = time.Duration(10+rand.Intn(6)) * time.Second
				}
				mu.Unlock()

				if canPlayBackchannel && h.backchannelService != nil && pacer != nil && !pacer.IsPlaying() {
					clip := h.backchannelService.GetRandomClip()
					if len(clip) > 0 {
						pacer.PushAudio(clip)
						log.Printf("[Backchannel] Deepgram text (%q) -> Played soft low-voice cue", transcriptText)
					}
				}

				// Increment session turn counter for mid-call turn control
				userTurnCount++

				// [STRICT RULE] Turn 1 Response -> ALWAYS play pre-rendered Turn 2 intro pitch (greeting_audio_2). ZERO LLM CALLS ALLOWED!
				if userTurnCount == 1 && callDirection == "outbound" {
					compDir := resolveCompanyDir(config.Settings.DocsBasePath, companyName)
					greetingFile := "greeting.txt"
					if initialLang == "hindi" || initialLang == "hi" {
						greetingFile = "greeting_hi.txt"
					}
					greetingPath := filepath.Join(compDir, greetingFile)
					greetingBytes, err := os.ReadFile(greetingPath)

					introText := ""
					if err == nil && len(greetingBytes) > 0 {
						introText = strings.TrimSpace(string(greetingBytes))
						introText = strings.ReplaceAll(introText, "{name}", customerName)
						introText = strings.ReplaceAll(introText, "{CustomerName}", customerName)
					} else {
						introText = fmt.Sprintf("[excited] Hi %s! Riya here from %s. Got a quick offer for you, just two minutes!", customerName, companyName)
					}
					var introAudio []byte

					// Resolve canonical Redis key for Turn 2 intro audio
					turn2Key := sessionID
					if db.RedisClient != nil {
						refConfigKey := fmt.Sprintf("call_config:%s", sessionID)
						if refID, err := db.RedisClient.HGet(context.Background(), refConfigKey, "preliminary_call_id").Result(); err == nil && refID != "" {
							turn2Key = refID
						}
					}

					if db.RedisClient != nil && turn2Key != "" {
						configKey := fmt.Sprintf("call_config:%s", turn2Key)
						if b64Str, err := db.RedisClient.HGet(context.Background(), configKey, "greeting_audio_2").Result(); err == nil && b64Str != "" {
							if b, err := base64.StdEncoding.DecodeString(b64Str); err == nil && len(b) > 0 {
								introAudio = b
								if txt, err := db.RedisClient.HGet(context.Background(), configKey, "greeting_text_2").Result(); err == nil && txt != "" {
									introText = txt
								}
							}
						}
					}

					// Fallback: If Redis audio is missing, synthesize Turn 2 intro directly
					if len(introAudio) == 0 && h.ttsService != nil {
						log.Printf("⚠️ [PRE-SYNTH FALLBACK] Synthesizing Turn 2 intro pitch on-the-fly...")
						ttsCtx, ttsCancel := context.WithTimeout(context.Background(), 5*time.Second)
						if aud, err := h.ttsService.SynthesizeSpeechMulaw(ttsCtx, introText); err == nil && len(aud) > 0 {
							introAudio = aud
						}
						ttsCancel()
					}

					log.Printf("🚀 [0ms INSTANT PLAYOUT] Turn 1 response received ('%s'). Playing pre-rendered Turn 2 intro pitch (%d bytes). NO LLM CALLED!", transcriptText, len(introAudio))
					if len(introAudio) > 0 && pacer != nil {
						pacer.PushAudio(introAudio)
					}
					if recorder != nil {
						recorder.WriteAgentAudio(introAudio)
						recorder.WriteTranscript("agent", introText)
					}
					if services.GlobalKafkaAudioService != nil {
						services.GlobalKafkaAudioService.PublishAudioFrame(sessionID, "agent", introAudio, "media")
						services.GlobalKafkaAudioService.PublishTranscript(sessionID, "agent", introText)
					}
					mu.Lock()
					callHistory = append(callHistory, map[string]string{"role": "assistant", "content": introText})
					mu.Unlock()

					// Monitor completion of greeting_audio_2 in background to unlock introPhaseComplete
					go func() {
						time.Sleep(500 * time.Millisecond)
						for pacer != nil && pacer.IsPlaying() {
							time.Sleep(100 * time.Millisecond)
						}
						mu.Lock()
						introPhaseComplete = true
						userTurnCount = 2
						mu.Unlock()
						log.Printf("✅ [INTRO PITCH COMPLETE] Anjali finished Turn 2 intro pitch. Now listening for prospect's answer for Turn 3 LLM!")
					}()

					// STRICT MANDATE: Return immediately! Zero LLM calls allowed before greeting_audio_2 completes!
					return
				}

				// Check if user transcript is a question (contains '?')
				isQuestion := strings.Contains(cleanText, "?") || strings.HasSuffix(strings.TrimSpace(cleanText), "?")
				var spokenFillerText string

				// Mid-Call Audio Sequence (Silence Gap on Query Merge + Filler + LLM Response)
				if pacer != nil {
					if introPhaseComplete && pacer.IsPlaying() {
						pacer.ClearBuffer()
						log.Printf("🛑 [BARGE-IN ACTIVATED] Customer interrupted agent speech! Cleared ongoing audio playback immediately.")
					}
					if introPhaseComplete && isQueryAggregation {
						// 1.5-second smooth conversational gap (12,000 bytes of 8kHz mu-law silence)
						silenceGap := bytes.Repeat([]byte{0xFF}, 12000)
						pacer.PushAudio(silenceGap)
						log.Printf("⏸️ [QUERY MERGE GAP] Inserted 1.5s (12000 bytes) smooth silence gap BEFORE filler and merged response!")
					}

					if isQuestion {
						questionFillers := []string{
							"Okay, got it!",
							"Sure, got it!",
							"Ah, I see!",
						}
						if initialLang == "hindi" || initialLang == "hi" {
							questionFillers = []string{
								"जी, बिल्कुल!",
								"जी, समझ गई!",
								"जी, ठीक है!",
							}
						}
						selectedQuestionFiller := questionFillers[rand.Intn(len(questionFillers))]
						spokenFillerText = selectedQuestionFiller
						log.Printf("❓ [QUESTION FILLER HIT] Customer asked a question (%q) → Playing random question filler %q", cleanText, selectedQuestionFiller)
						var ackAudio []byte
						if h.ttsService != nil {
							ttsCtx, ttsCancel := context.WithTimeout(context.Background(), 3*time.Second)
							ackAudio = h.ttsService.GetPreGeneratedAudio(ttsCtx, selectedQuestionFiller, ttsSpeed)
							ttsCancel()
						}
						if len(ackAudio) > 0 {
							pacer.PushAudio(ackAudio)
							log.Printf("✅ [QUESTION FILLER PLAYED] Pushed %d bytes of %q filler audio!", len(ackAudio), selectedQuestionFiller)
						}
					} else {
						var customFiller []byte
						turnKey := sessionID
						if db.RedisClient != nil {
							refConfigKey := fmt.Sprintf("call_config:%s", sessionID)
							if refID, err := db.RedisClient.HGet(context.Background(), refConfigKey, "preliminary_call_id").Result(); err == nil && refID != "" {
								turnKey = refID
							}
							configKey := fmt.Sprintf("call_config:%s", turnKey)
							
							// Retrieve last used filler index to prevent repeating the same filler consecutively
							lastIdxVal, _ := db.RedisClient.HGet(context.Background(), configKey, "last_filler_idx").Result()
							lastIdx := 0
							if lastIdxVal != "" {
								_, _ = fmt.Sscanf(lastIdxVal, "%d", &lastIdx)
							}

							fillerIdx := rand.Intn(7) + 1
							if fillerIdx == lastIdx {
								fillerIdx = (fillerIdx % 7) + 1
							}
							_ = db.RedisClient.HSet(context.Background(), configKey, "last_filler_idx", fmt.Sprintf("%d", fillerIdx))

							if initialLang == "hindi" || initialLang == "hi" {
								hiTexts := []string{
									"हाँ, जी!",
									"जी, बिल्कुल!",
									"जी, समझ गई!",
									"सही बात है!",
									"जी, ठीक है!",
									"बिल्कुल, सही है!",
								}
								spokenFillerText = hiTexts[(fillerIdx-1)%len(hiTexts)]
								enTexts := []string{
									"Yeah, got it!",
									"Ah, I see!",
									"Right, got it!",
									"Makes total sense!",
									"Alright, got it!",
									"Got it, perfect!",
									"Sure, got it!",
								}
								spokenFillerText = enTexts[(fillerIdx-1)%len(enTexts)]
							}

							fillerField := fmt.Sprintf("filler_audio_%d", fillerIdx)
							if b64Str, err := db.RedisClient.HGet(context.Background(), configKey, fillerField).Result(); err == nil && b64Str != "" {
								if b, err := base64.StdEncoding.DecodeString(b64Str); err == nil && len(b) > 0 {
									customFiller = b
								}
							}
						}

						if len(customFiller) > 0 {
							pacer.PushAudio(customFiller)
							log.Printf("🚀 [0ms FILLER HIT] Played call-specific pre-rendered filler %q at requested tts_speed (Turn %d)", spokenFillerText, userTurnCount)
						} else if h.fillerService != nil {
							fillLang := "en"
							if initialLang == "hindi" || initialLang == "hi" {
								fillLang = "hi"
							}
							var clip []byte
							if userTurnCount <= 3 {
								clip = h.fillerService.GetInitialHmmFiller(fillLang)
							} else {
								clip = h.fillerService.GetRandomFiller(fillLang)
							}
							if len(clip) > 0 {
								pacer.PushAudio(clip)
								log.Printf("[FILLER] Turn %d → Played instant natural universal filler ('Hmm, ...') before LLM", userTurnCount)
							}
						}
					}
				}

				log.Printf("[LEVEL 3: STT TRANSCRIPT] User Spoke: %q (Confidence: %.0f%%)", transcriptText, confidence*100)

				if recorder != nil {
					recorder.WriteTranscript("user", transcriptText)
				}
				if services.GlobalKafkaAudioService != nil {
					services.GlobalKafkaAudioService.PublishTranscript(sessionID, "user", transcriptText)
					services.GlobalKafkaAudioService.PublishLog(sessionID, "STT", "INFO", fmt.Sprintf("User Spoke: %q (ASR Probability: %.0f%%)", transcriptText, confidence*100))
				}

				// STRICT MANDATE: LLM is 100% FORBIDDEN on Turn 1 & Turn 2!
				// LLM can ONLY be called AFTER Turn 2 greeting sales pitch completes (introPhaseComplete == true)!
				if !introPhaseComplete {
					log.Printf("🛑 [LLM GUARDRAIL ACTIVATED] Blocked LLM execution because Turn 2 intro pitch has not finished yet!")
					return
				}

				// Process RAG + OpenAI GPT-4o + Fish/Cartesia TTS in background goroutine
				go func(ctx context.Context, query string, conf float64, rawSpeech []byte, rawWav []byte) {
					var emotionBlock string
					// C++ Emotion classification bypassed: Anjali's tone is statically fixed to high-energy, warm, and joyful!
					emotionBlock = "Adapt your conversational tone appropriately based on the user's emotion."

					// TIER 2: Moderate Confidence (0.50 <= confidence < 0.85) -> Smart System Context Note (+0ms extra delay)
					if conf >= 0.50 && conf < 0.85 {
						log.Printf("💡 [ASR CONFIDENCE TIER 2] Moderate speech certainty (%.0f%%). Injecting system context note for LLM.", conf*100)
						emotionBlock += fmt.Sprintf("\n[ASR SPEECH CONFIDENCE: %.0f%%]\nThe audio had slight background noise or accent variation. If the customer's intent is ambiguous, politely ask a brief clarifying question.", conf*100)
					}

					startTurnTime := time.Now()

					// Document-Constrained Query Normalization (Devanagari Transliteration + Soundex / Levenshtein against RAG Doc Vocab)
					vocab := services.BuildDocVocabulary(companyName)
					normalizedQuery := services.NormalizeQuery(query, vocab)
					if normalizedQuery != query {
						log.Printf("🔍 [QUERY NORMALIZED] Raw ASR: %q ➔ RAG Doc Normalized: %q", query, normalizedQuery)
						query = normalizedQuery
					}

					// Vector RAG Search
					ragStartTime := time.Now()
					ragResults, searchErr := h.ragProvider.Search(sessionID, query, tenantIDStr)
					ragDuration := time.Since(ragStartTime)
					if searchErr != nil {
						log.Printf("[Call RAG ERROR] Search failed: %v", searchErr)
					}

					// Sub-query RAG search fallback for aggregated multi-clause user queries
					if isQueryAggregation && len(query) > 25 {
						words := strings.Fields(query)
						if len(words) > 4 {
							subQuery := strings.Join(words[len(words)-4:], " ")
							if subResults, err := h.ragProvider.Search(sessionID, subQuery, tenantIDStr); err == nil && len(subResults) > 0 {
								for _, subRes := range subResults {
									isDup := false
									for _, mainRes := range ragResults {
										if mainRes.Text == subRes.Text {
											isDup = true
											break
										}
									}
									if !isDup {
										ragResults = append(ragResults, subRes)
									}
								}
							}
						}
					}

					contextBlock := ""
					if len(ragResults) > 0 {
						contextBlock = "\n\nRetrieved context from company knowledge base:\n"
						for i, res := range ragResults {
							contextBlock += fmt.Sprintf("[%d] (Source: %s, Similarity: %.2f) %s\n", i+1, res.Source, res.Score, res.Text)
						}
					}
					if emotionBlock != "" {
						contextBlock += emotionBlock
					}

					// Cascaded LLM Token Streaming -> TTS Clause Synthesis
					mu.Lock()
					historyCopy := make([]map[string]string, len(callHistory))
					copy(historyCopy, callHistory)
					mu.Unlock()

					llmStartTime := time.Now()
					var mulawBytes []byte
					var ttfaDuration time.Duration
					var llmTTFT time.Duration
					var ttsTTFA time.Duration
					firstClauseReceived := false
					firstAudioPushed := false

					replyText, llmErr := h.callOpenAIGPTStreamForCall(ctx, sessionID, companyName, customerName, phone, query, contextBlock, spokenFillerText, historyCopy, func(clauseText string) {
						clauseText = cleanTextForTTS(clauseText)
						if !firstClauseReceived {
							clauseText = trimRedundantOpeningFluff(clauseText)
							llmTTFT = time.Since(llmStartTime)
							firstClauseReceived = true
						}
						if ctx.Err() != nil || strings.TrimSpace(clauseText) == "" {
							return
						}
						if !firstClauseReceived {
							llmTTFT = time.Since(llmStartTime)
							firstClauseReceived = true
						}
						ttsStartClauseTime := time.Now()
						log.Printf("[LLM Stream Clause] Synthesizing: %q (LLM First Clause TTFT: %v)", clauseText, llmTTFT)
						ttsCtx, ttsCancel := context.WithTimeout(ctx, 5*time.Second)
						defer ttsCancel()

						clauseAudio, ttsErr := h.ttsService.SynthesizeSpeechStreamWithSpeed(ttsCtx, clauseText, ttsSpeed, func(chunk []byte) {
							if ctx.Err() != nil {
								return
							}
							if !firstAudioPushed {
								ttsTTFA = time.Since(ttsStartClauseTime)
								ttfaDuration = time.Since(startTurnTime)
								firstAudioPushed = true
								log.Printf("\n"+
									"⏱️⏱️ [LATENCY BENCHMARK BREAKDOWN — TURN %d] User Query: %q\n"+
									"  ├─ LEVEL 1 [STT Speech Finalized]:        0.00 ms (t0)\n"+
									"  ├─ LEVEL 2 [Vector RAG Search]:           %.2f ms (%v)\n"+
									"  ├─ LEVEL 3 [LLM TTFT First Token/Clause]: %.2f ms (%v) -> %q\n"+
									"  ├─ LEVEL 4 [TTS TTFA First Audio Chunk]:  %.2f ms (%v) [%s Engine]\n"+
									"  └─ 🚀🚀 LEVEL 5 [TOTAL E2E TO EARPIECE]:   %.2f ms (%v)",
									userTurnCount, query,
									float64(ragDuration.Microseconds())/1000.0, ragDuration,
									float64(llmTTFT.Microseconds())/1000.0, llmTTFT, clauseText,
									float64(ttsTTFA.Microseconds())/1000.0, ttsTTFA, config.Settings.TtsProvider,
									float64(ttfaDuration.Microseconds())/1000.0, ttfaDuration)
							}
							if pacer != nil && len(chunk) > 0 {
								pacer.PushAudio(chunk)
							}
						})
						if ttsErr == nil && len(clauseAudio) > 0 {
							mulawBytes = append(mulawBytes, clauseAudio...)
						}
					})
					llmDuration := time.Since(llmStartTime)
					if llmErr != nil || replyText == "" || ctx.Err() != nil {
						log.Printf("[Call LLM RESULT]: err=%v, replyLen=%d, ctxErr=%v", llmErr, len(replyText), ctx.Err())
						return
					}

					log.Printf("[Call LLM SUCCESS] Full Assistant Reply: %q", replyText)

					mu.Lock()
					callHistory = append(callHistory, map[string]string{"role": "assistant", "content": replyText})
					mu.Unlock()

					totalTurnDuration := time.Since(startTurnTime)
					latencyLogMsg := fmt.Sprintf("[LATENCY METRICS] Session: %s | Total Turn: %v | TTFA (First Audio): %v | RAG: %v | LLM Stream: %v | TTS Provider: %s",
						sessionID, totalTurnDuration, ttfaDuration, ragDuration, llmDuration, config.Settings.TtsProvider)
					log.Println(latencyLogMsg)
					if services.GlobalKafkaAudioService != nil {
						services.GlobalKafkaAudioService.PublishLog(sessionID, "LATENCY", "INFO", latencyLogMsg)
					}

					if recorder != nil && len(mulawBytes) > 0 {
						recorder.WriteAgentAudio(mulawBytes)
						recorder.WriteTranscript("agent", replyText)
					}
					if services.GlobalKafkaAudioService != nil && len(mulawBytes) > 0 {
						services.GlobalKafkaAudioService.PublishAudioFrame(sessionID, "agent", mulawBytes, "media")
						services.GlobalKafkaAudioService.PublishTranscript(sessionID, "agent", replyText)
					}
				}(turnCtx, transcriptText, confidence, rawSpeech, rawWav)
			}

			// Start live WebSocket transcript listener for Streaming Mode
			if sttSession != nil && !sttListenerStarted {
				sttListenerStarted = true
				go func() {
					var (
						bargeInInterrupted bool
						stitchedText       string
						highestConf        float64
						debounceTimer      *time.Timer
						debounceMu         sync.Mutex
					)
					for res := range sttSession.Transcripts() {
						cleanText := strings.TrimSpace(res.Text)
						if cleanText == "" {
							continue
						}
						log.Printf("🎙️ [STT Deepgram WS LIVE] Session %s (IsFinal=%v, Conf=%.2f): %q", sessionID, res.IsFinal, res.Confidence, cleanText)

						// Humanized Barge-In Interruption: Early detection on any live speech token
						if pacer != nil && pacer.IsPlaying() && !bargeInInterrupted {
							bargeInInterrupted = true
							reactionDelay := config.Settings.BargeInReactionDelayMs
							if reactionDelay <= 0 {
								reactionDelay = 80
							}
							log.Printf("🎙️ [HUMANIZED BARGE-IN] Customer speech detected (%q, conf=%.2f). Preempting active agent speech...", cleanText, res.Confidence)

							// Preempt active in-flight turn context immediately
							activeTurnMu.Lock()
							if activeTurnCancel != nil {
								activeTurnCancel()
								activeTurnCancel = nil
							}
							activeTurnMu.Unlock()

							go func(delayMs int) {
								time.Sleep(time.Duration(delayMs) * time.Millisecond)
								if pacer != nil {
									pacer.ClearBuffer()
								}
							}(reactionDelay)
						}

						if res.IsFinal {
							bargeInInterrupted = false

							debounceMu.Lock()
							if stitchedText != "" {
								stitchedText += " " + cleanText
							} else {
								stitchedText = cleanText
							}
							if res.Confidence > highestConf {
								highestConf = res.Confidence
							}

							if debounceTimer != nil {
								debounceTimer.Stop()
							}

							debounceTimer = time.AfterFunc(650*time.Millisecond, func() {
								debounceMu.Lock()
								textToTrigger := stitchedText
								confToTrigger := highestConf
								stitchedText = ""
								highestConf = 0.0
								debounceMu.Unlock()

								if textToTrigger != "" {
									log.Printf("💬 [STITCHED UTTERANCE] Debounced & stitched sentence ready for session %s: %q (Conf: %.0f%%)", sessionID, textToTrigger, confToTrigger*100)
									triggerTurn(textToTrigger, confToTrigger, nil, nil)
								}
							})
							debounceMu.Unlock()
						}
					}
				}()
			}

			// Fallback Batch / REST Mode: Trigger TranscribeAudioPayload on speech pause ONLY when STT_MODE == "batch" or sttSession is nil
			if (sttSession == nil || config.Settings.SttMode == "batch") && shouldProcess && len(speechBytes) > 0 {
				log.Printf("🎙️ [STT REST PAYLOAD FALLBACK] User finished speaking (%d audio bytes). Invoking %s TranscribeAudioPayload REST API...", len(speechBytes), h.sttProvider.Type())
				wavBytes := convertMulawToWavBytes(speechBytes)
				transcriptText, sttErr := h.sttProvider.TranscribeAudioPayload(wavBytes)
				if sttErr != nil {
					log.Printf("❌ [Call STT ERROR] %s STT REST call failed: %v", h.sttProvider.Type(), sttErr)
					continue
				}
				if transcriptText != "" {
					log.Printf("✅ [STT REST SUCCESS] %s transcribed: %q", h.sttProvider.Type(), transcriptText)
					triggerTurn(transcriptText, 1.0, speechBytes, wavBytes)
				}
			}

		case telephony.EventStop:
			log.Printf("[LEVEL 4: SESSION END] Call stream stop event received for: %s", sessionID)
			if recorder != nil {
				recorder.Close()
			}
			return
		}
	}
}

func convertMulawToWavBytes(mulawBytes []byte) []byte {
	if len(mulawBytes) == 0 {
		return nil
	}
	pcmBytes := make([]byte, len(mulawBytes)*2)
	for i, uVal := range mulawBytes {
		sample := services.MulawSampleToPcm16(uVal)
		binary.LittleEndian.PutUint16(pcmBytes[i*2:i*2+2], uint16(sample))
	}
	var buf bytes.Buffer
	var header [44]byte
	pcmDataLen := uint32(len(pcmBytes))
	copy(header[0:4], []byte("RIFF"))
	binary.LittleEndian.PutUint32(header[4:8], pcmDataLen+36)
	copy(header[8:12], []byte("WAVE"))
	copy(header[12:16], []byte("fmt "))
	binary.LittleEndian.PutUint32(header[16:20], 16)
	binary.LittleEndian.PutUint16(header[20:22], 1) // PCM format
	binary.LittleEndian.PutUint16(header[22:24], 1) // 1 channel
	binary.LittleEndian.PutUint32(header[24:28], 8000)
	binary.LittleEndian.PutUint32(header[28:32], 8000*1*16/8)
	binary.LittleEndian.PutUint16(header[32:34], 2)
	binary.LittleEndian.PutUint16(header[34:36], 16)
	copy(header[36:40], []byte("data"))
	binary.LittleEndian.PutUint32(header[40:44], pcmDataLen)

	buf.Write(header[:])
	buf.Write(pcmBytes)
	return buf.Bytes()
}

type LLMEmotionResponse struct {
	EmotionState  map[string]interface{} `json:"emotion_state"`
	ExtractedName string                 `json:"extracted_name"`
	Feeling       string                 `json:"feeling"`
	Reason        string                 `json:"reason"`
	Reply         string                 `json:"reply"`
}

type CompanyMetadataJSON struct {
	CompanyName string `json:"company_name"`
	Location    string `json:"location"`
	Tagline     string `json:"tagline"`
	Leadership  struct {
		Founder   string `json:"founder"`
		CoFounder string `json:"co_founder"`
		Manager   string `json:"manager"`
	} `json:"leadership"`
	KeyStats struct {
		YearsInBusiness  string `json:"years_in_business"`
		HappyCustomers   string `json:"happy_customers"`
		ProjectsFinished string `json:"projects_finished"`
		GoogleRating     string `json:"google_rating"`
	} `json:"key_stats"`
	Contact struct {
		Phone   string `json:"phone"`
		Email   string `json:"email"`
		Website string `json:"website"`
	} `json:"contact"`
}

func loadCompanyMetadata(compDir string) string {
	metaPath := filepath.Join(compDir, "company_metadata.json")
	bytes, err := os.ReadFile(metaPath)
	if err != nil || len(bytes) == 0 {
		return ""
	}

	var meta CompanyMetadataJSON
	if err := json.Unmarshal(bytes, &meta); err != nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n\n[DYNAMIC COMPANY PROFILE & LEADERSHIP METADATA]\n")
	if meta.CompanyName != "" {
		sb.WriteString(fmt.Sprintf("- Company Name: %s\n", meta.CompanyName))
	}
	if meta.Location != "" {
		sb.WriteString(fmt.Sprintf("- Location: %s\n", meta.Location))
	}
	if meta.Leadership.Founder != "" || meta.Leadership.CoFounder != "" || meta.Leadership.Manager != "" {
		sb.WriteString(fmt.Sprintf("- Company Leadership: Founder: %s | Co-Founder: %s | Manager: %s\n",
			meta.Leadership.Founder, meta.Leadership.CoFounder, meta.Leadership.Manager))
	}
	if meta.KeyStats.YearsInBusiness != "" || meta.KeyStats.HappyCustomers != "" {
		sb.WriteString(fmt.Sprintf("- Key Statistics: %s years in business, %s happy customers, %s Google rating\n",
			meta.KeyStats.YearsInBusiness, meta.KeyStats.HappyCustomers, meta.KeyStats.GoogleRating))
	}
	return sb.String()
}

func (h *CallHandler) callOpenAIGPTForCall(sessionID, companyName, customerName, phone, query, contextBlock string, history []map[string]string) (string, error) {
	apiKey := config.Settings.OpenaiApiKey
	if apiKey == "" {
		return "I am having trouble connecting to my backend service.", nil
	}

	compDir := resolveCompanyDir(config.Settings.DocsBasePath, companyName)
	promptPath := filepath.Join(compDir, "system_prompt.txt")
	systemPromptBytes, err := os.ReadFile(promptPath)
	systemPrompt := ""
	if err == nil && len(systemPromptBytes) > 0 {
		systemPrompt = string(systemPromptBytes)
	} else {
		systemPrompt = fmt.Sprintf("You are Riya, a sharp, confident sales representative calling on behalf of %s. Sound 100%% human, natural, and helpful on this OUTBOUND call.", companyName)
	}
	profileContext := fmt.Sprintf("\n\n[CUSTOMER PROFILE & OUTBOUND CALL CONTEXT]\n"+
		"- Customer Name: %s\n"+
		"- Customer Phone: %s\n"+
		"- Target Product: %s Services\n"+
		"- OUTBOUND SALES DIRECTIVE: You initiated this outbound call. YOU ARE LEADING THE CONVERSATION. Never ask passive inbound support questions like 'How can I assist you today?' or 'What do you want to know?'. State why you called, uncover pain, present value, and drive towards booking a meeting.\n",
		customerName, phone, companyName)
	if db.RedisClient != nil && sessionID != "" {
		configKey := fmt.Sprintf("call_config:%s", sessionID)
		if refID, err := db.RedisClient.HGet(context.Background(), configKey, "preliminary_call_id").Result(); err == nil && refID != "" {
			configKey = fmt.Sprintf("call_config:%s", refID)
		}
		if val, err := db.RedisClient.HGet(context.Background(), configKey, "initial_language").Result(); err == nil && (val == "hindi" || val == "hi") {
			profileContext += "- INITIAL LANGUAGE DIRECTIVE: The caller requested an initial HINDI call. YOU MUST SPEAK 100% IN DEVANAGARI HINDI WITH LATIN ENGLISH KEYWORDS FROM TURN 1!\n"
		}
	}
	profileContext += loadCompanyMetadata(compDir)
	systemPrompt += profileContext

	if contextBlock != "" {
		systemPrompt += "\n\n[RETRIEVED DOCUMENT CONTEXT]\n" + contextBlock
	}

	hinglishRuleBlock := "\n\nUNIVERSAL SPOKEN HINGLISH SCRIPT & NUMBER RULE (STRICT MANDATE):\n" +
		"0. DYNAMIC TURN-BY-TURN LANGUAGE SWITCHING (HIGHEST PRIORITY MANDATE):\n" +
		"   - YOU ARE FULLY MULTILINGUAL AND MUST SWITCH LANGUAGES FREELY ON EVERY TURN DEPENDING ON WHAT THE USER SAYS!\n" +
		"   - IF THE USER SPEAKS IN PURE ENGLISH (e.g. 'Yeah, tell me something more', 'What is the eligibility?', 'Tell me in English'): YOU MUST IMMEDIATELY SWITCH TO 100% PURE ENGLISH ONLY! DO NOT USE HINDI/DEVANAGARI IN THAT TURN!\n" +
		"   - ❌ NEVER STAY LOCKED IN HINDI IF THE USER SPEAKS IN ENGLISH!\n" +
		"0.1 EMBED IN-LINE EMOTION TAGS FOR HIGH-ENERGY TTS (MANDATORY):\n" +
		"   - YOU MUST EMBED SQUARE BRACKET EMOTION TAGS like [excited], [enthusiastic], [joyful], [warm] at start of replies & right before key facts (stipends, zero fees, salary, visa)!\n" +
		"1. CASUAL HUMAN HINGLISH (100% NATURAL DAILY-LIFE CONVERSATION):\n" +
		"   - Sound like a smart, warm, cheerful Indian girl speaking natural daily-life Hinglish on the phone!\n" +
		"   - ALL KEYWORDS, NOUNS, NUMBERS, AMOUNTS, CURRENCIES, PHDs, POSTDOCS, STIPENDS, SCHOLARSHIPS, RESEARCH, OPPORTUNITY, CHANCE, BENEFITS, VISA, HOUSING, PROCESS, EXAMS, IELTS, SCORES, PERCENT, DISCOUNT, FEES, AND TECHNICAL TERMS MUST STRICTLY BE WRITTEN IN LATIN ENGLISH SCRIPT!\n" +
		"2. WRITE ALL NUMBERS IN FULL SPOKEN ENGLISH WORDS (CRITICAL FOR TTS - ZERO DIGITS):\n" +
		"   - NEVER USE RAW DIGITS (like 3.6, 4.2, 5.1, 43, 100, 6.5, 2) AND NEVER USE HINDI DEVANAGARI NUMBERS (like तीन लाख साठ हजार, चार लाख)!\n" +
		"   - ✅ ALWAYS WRITE NUMBERS IN PURE SPOKEN ENGLISH WORDS: 'three point six Lakhs to four point two Lakhs rupees per month', 'five point one Lakhs to five point seven Lakhs rupees per month', 'forty three Lakhs per year', 'one hundred percent visa support', 'six point five IELTS score', 'two minutes'!\n" +
		"3. ABSOLUTELY FORBIDDEN BOOKISH HINDI WORDS (MUST USE LATIN ENGLISH INSTEAD):\n" +
		"   - ❌ NEVER WRITE 'अनुसंधान' -> Write 'research'!\n" +
		"   - ❌ NEVER WRITE 'अवसर' or 'मौका' -> Write 'opportunity' or 'chance'!\n" +
		"   - ❌ NEVER WRITE 'छात्रवृत्ति' -> Write 'scholarship' or 'stipend'!\n" +
		"   - ❌ NEVER WRITE 'राशि' -> Write 'amount' or 'stipend'!\n" +
		"   - ❌ NEVER WRITE 'विश्वविद्यालय' -> Write 'university'!\n" +
		"   - ❌ NEVER WRITE 'संस्थान' -> Write 'institute'!\n" +
		"   - ❌ NEVER WRITE 'आवास' -> Write 'housing'!\n" +
		"   - ❌ NEVER WRITE 'वीज़ा' -> Write 'visa'!\n" +
		"   - ❌ NEVER WRITE 'प्रतिशत' -> Write 'percent'!\n" +
		"   - ❌ NEVER WRITE 'छूट' -> Write 'discount'!\n" +
		"   - ❌ NEVER WRITE 'योग्य' or 'पात्र' -> Write 'eligible'!\n" +
		"   - ❌ NEVER WRITE 'मुफ़्त' or 'निशुल्क' -> Write 'free'!\n" +
		"   - ❌ NEVER WRITE 'लाभ' -> Write 'benefit'!\n" +
		"   - ❌ NEVER WRITE 'ऑफ़र' -> Write 'offer'!\n" +
		"4. DEVANAGARI IS ONLY FOR BASIC CASUAL HINDI CONNECTORS/VERBS:\n" +
		"   - Write ONLY basic spoken Hindi connecting words/verbs in Devanagari script (e.g., 'बिल्कुल', 'मैं', 'आप', 'बोल रही हूँ', 'के लिए', 'भी', 'होते हैं', 'मिलेगा', 'क्या आप', 'चाहेंगे', 'बताती हूँ', 'समझती हूँ').\n" +
		"   - Keep it 100% natural, modern, friendly, and human!\n" +
		"5. ZERO NEWLINES AND CLEAN DEGREE WORDS (STRICT MANDATE):\n" +
		"   - ❌ NEVER OUTPUT NEWLINE CHARACTERS (\\n)! Output continuous single-line spoken text only.\n" +
		"   - ALWAYS write degrees and acronyms as clean single words without dots or hyphens: MTech (NOT M.Tech), MSc (NOT M.Sc), ME (NOT M.E), BTech (NOT B.Tech), BSc (NOT B.Sc), PhD (NOT Ph.D), SOP (NOT S.O.P), CV (NOT C.V), RND (NOT R&D).\n" +
		"9. INTELLIGENT CONTEXTUAL INFERENCE FOR ASR TYPOS & MISHEARINGS (CRITICAL):\n" +
		"   - Speech-to-text (ASR) engines often mishear spoken words and output minor phonetic typos or misspellings (e.g. transcribing 'Founder' as 'Pounder'/'Faunder'/'Fonter', 'SEO' as 'aco'/'eso', 'Google' as 'Geegel', 'package' as 'pakej', 'price' as 'prais').\n" +
		"   - NEVER take raw ASR typos literally or claim 'I don't know who/what that is'!\n" +
		"   - ALWAYS infer the intended concept from your [RETRIEVED DOCUMENT CONTEXT] and [DYNAMIC COMPANY PROFILE & LEADERSHIP METADATA] based on phonetic similarity and business context!\n" +
		"10. EVERY RESPONSE MUST REMAIN OPEN & ASK A NEW ENGAGING QUESTION (STRICT MANDATE):\n" +
		"   - In EVERY single response, structure your reply in 3 parts: 1) Brief Acknowledgment, 2) Direct Answer/Value, 3) ALWAYS END WITH A NEW OPEN DISCOVERY OR CLOSING QUESTION!\n" +
		"   - ❌ NEVER END PASSIVELY with statements like 'Have a good day', 'Let me know if you have questions', or ending with a period without a question hook!\n" +
		"   - ✅ ALWAYS end with an engaging open question driving to the next step (e.g. 'क्या आप हमारे packages explore करना चाहेंगे?', 'Would afternoon work best for a 5-minute chat?').\n" +
		"11. PROACTIVE TELESALES CLOSING & BEST SELLING STRATEGY (STRICT MANDATE):\n" +
		"   - You are a proactive telesales closer, NOT a passive support agent!\n" +
		"   - In EVERY response, actively promote the product using a BEST SELLING STRATEGY (highlight high-ROI savings, 10% cashback, zero contracts, 10-day trial, or traffic growth).\n" +
		"   - Pitch the sale directly: ask to book a consultation, select a package, or complete Video KYC!\n" +
		"12. RELENTLESS SALES CLOSING & PERSUASIVE OBJECTION PIVOTING (NEVER GIVE UP — STRICT MANDATE):\n" +
		"   - Top telesales closers NEVER give up easily! If the prospect hesitates, says 'not interested', 'busy', 'don't need it', or pushes back:\n" +
		"   - ❌ STRICTLY FORBIDDEN: NEVER SAY PASSIVE GIVING-UP STATEMENTS like 'कोई बात नहीं', 'अगर आपको बाद में interest हो...', 'कोई सवाल हो तो संपर्क करें', or 'Have a good day'!\n" +
		"   - ❌ STRICTLY FORBIDDEN: NEVER WRITE KEYWORDS, NAMES, OR NUMBERS IN DEVANAGARI! Do NOT write 'ओमिका' -> Write 'Omica'! Do NOT write 'कैशबैक' -> Write 'cashback'! Do NOT write 'फ़ायदे' -> Write 'benefits'! Do NOT write 'सारांश' -> Write 'quick summary'! Do NOT write 'बातचीत' -> Write 'call' or 'chat'!\n" +
		"   - ✅ PERFECT DEVANAGARI + LATIN ENGLISH KEYWORDS EXAMPLE:\n" +
		"     \"मैं समझती हूँ, Omica! [empathetic] बहुत से customers को starting में ऐसा ही महसूस होता है। लेकिन जब वे cashback और benefits देखते हैं, तो बहुत happy होते हैं! [excited] क्या मैं आपको WhatsApp पर quick summary भेज सकती हूँ, या फिर कल afternoon एक short two minute call कर सकते हैं?\"\n" +
		"13. EMOTIONAL PERSUASION & EMPATHETIC SELLING TECHNIQUES (STRICT MANDATE):\n" +
		"   - Build a deep emotional connection and trigger emotional motivations in every turn!\n" +
		"   - ALL NAMES, KEYWORDS, NOUNS, BRANDS & NUMBERS MUST REMAIN IN PURE LATIN ENGLISH SCRIPT (e.g. Omica, customers, cashback, benefits, WhatsApp, summary, call, HDFC, Swiggy, fifteen thousand, two)!\n" +
		"   - ✅ PERFECT DEVANAGARI + LATIN ENGLISH KEYWORDS EXAMPLE:\n" +
		"     \"मैं समझती हूँ, Omica! [warm] महीने का fifteen thousand rupees save करना सच में बहुत rewarding feeling होती है! [excited] क्या आप Swiggy पर ten percent cashback का discount claim करना चाहेंगे?\"\n" +
		"14. DYNAMIC VOCAL EMOTION MODULATION RULE (WARM START -> EXCITED KEY OFFER):\n" +
		"   - In EVERY response, ALWAYS start your first sentence warmly and politely with [warm] (e.g. '[warm] I understand...', '[warm] Main samajhti hoon...').\n" +
		"   - Transition to [excited] ONLY when delivering key offers, numbers, cashback benefits, package savings, or call-to-action questions (e.g. '[excited] You will get ten percent instant cashback!')!\n"
	systemPrompt += hinglishRuleBlock

	messages := []map[string]string{
		{"role": "system", "content": systemPrompt},
	}
	if len(history) > 20 {
		history = history[len(history)-20:]
	}
	for _, turn := range history {
		messages = append(messages, turn)
	}

	messages = append(messages, map[string]string{"role": "user", "content": query})

	payload := map[string]interface{}{
		"model":       "gpt-4o-mini",
		"messages":    messages,
		"temperature": 0.6,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(payloadBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("OpenAI API error (status %d): %s", resp.StatusCode, string(errBody))
	}

	var chatResp openAIChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", err
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("empty chat completions returned")
	}

	replyText := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	return replyText, nil
}

type openAIStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

func shouldFlushClause(text string) bool {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) < 12 {
		return false
	}
	return strings.HasSuffix(trimmed, ".") || strings.HasSuffix(trimmed, "?") || strings.HasSuffix(trimmed, "!") || strings.HasSuffix(trimmed, ",") || strings.HasSuffix(trimmed, ";") || strings.HasSuffix(trimmed, "—") || strings.HasSuffix(trimmed, "...")
}

var markdownCleaner = regexp.MustCompile(`[\*\_#` + "`" + `~]`)

var digitReplacements = map[string]string{
	"0": "zero", "1": "one", "2": "two", "3": "three", "4": "four",
	"5": "five", "6": "six", "7": "seven", "8": "eight", "9": "nine",
	"10": "ten", "11": "eleven", "12": "twelve", "13": "thirteen",
	"14": "fourteen", "15": "fifteen", "16": "sixteen", "17": "seventeen",
	"18": "eighteen", "19": "nineteen", "20": "twenty", "30": "thirty",
	"40": "forty", "50": "fifty", "60": "sixty", "70": "seventy",
	"80": "eighty", "90": "ninety", "100": "one hundred",
	"200": "two hundred", "300": "three hundred", "400": "four hundred",
	"500": "five hundred", "600": "six hundred", "700": "seven hundred",
	"800": "eight hundred", "900": "nine hundred", "1000": "one thousand",
}

var devanagariEnglishReplacements = map[string]string{
	"ऑनलाइन": "online",
	"प्लेटफ़ॉर्म": "platform", "प्लेटफार्म": "platform", "प्लेटफॉर्म": "platform",
	"कीवर्ड": "keywords", "कीवर्ड्स": "keywords",
	"वेबसाइट": "website", "वेबसाईट": "website",
	"गूगल": "Google",
	"सर्विस": "service", "सर्विसों": "services", "सर्विसेज": "services", "सेवा": "services", "सेवाएं": "services", "सेवाओं": "services",
	"बिजनेस": "business", "व्यापार": "business", "व्यवसाय": "business",
	"डिजिटल": "digital",
	"मार्केटिंग": "marketing",
	"अकाउंट": "account",
	"रिजल्ट": "results", "रिजल्ट्स": "results", "परिणाम": "results",
	"क्लाइंट": "clients", "क्लाइंट्स": "clients",
	"कस्टमर": "customers", "कस्टमर्स": "customers", "ग्राहक": "customers", "ग्राहकों": "customers",
	"पेज": "page",
	"पैकेज": "package", "पैकेजेस": "packages",
	"विज्ञापन": "ads", "विज्ञापनों": "ads",
	"उत्पाद": "products", "उत्पादों": "products",
	"मूल्य": "price", "दाम": "price",
	"प्रक्रिया": "process",
	"विशेषज्ञ": "experts", "विशेषज्ञों": "experts",
	"प्रणाली": "system",
	"जानकारी": "details",
	"सामग्री": "content",
	"रणनीति": "strategy",
	"प्रतिद्वंद्वी": "competitors", "प्रतिद्वंद्वियों": "competitors",
	"वृद्धि": "growth", "बढ़ोतरी": "growth",
	"कैशबैक": "cashback", "फ़ायदे": "benefits", "फ़ायदों": "benefits", "फायदे": "benefits", "फायदों": "benefits",
	"एक": "one", "दो": "two", "तीन": "three", "चार": "four", "पांच": "five", "पाँच": "five",
	"छह": "six", "सात": "seven", "आठ": "eight", "नौ": "nine", "दस": "ten",
	"पंद्रह": "fifteen", "पन्द्रह": "fifteen", "बीस": "twenty", "पचास": "fifty",
	"सौ": "hundred", "हजार": "thousand", "हज़ार": "thousand", "लाख": "lakh", "करोड़": "crore",
}

func init() {
	jsonPath := filepath.Join("config", "hindi_keywords.json")
	if data, err := os.ReadFile(jsonPath); err == nil {
		var cfg struct {
			DevanagariToEnglish map[string]string `json:"devanagari_to_english"`
		}
		if err := json.Unmarshal(data, &cfg); err == nil && len(cfg.DevanagariToEnglish) > 0 {
			for k, v := range cfg.DevanagariToEnglish {
				devanagariEnglishReplacements[k] = v
			}
			log.Printf("📖 [Hindi Keywords Config] Loaded %d keyword replacements from %s", len(cfg.DevanagariToEnglish), jsonPath)
		}
	}
}

func cleanTextForTTS(text string) string {
	cleaned := markdownCleaner.ReplaceAllString(text, "")
	cleaned = strings.ReplaceAll(cleaned, "\n", " ")
	cleaned = strings.ReplaceAll(cleaned, "\r", " ")
	cleaned = strings.ReplaceAll(cleaned, "\t", " ")

	// Automatically clean dotted degree & acronym abbreviations for seamless FishAudio pronunciation
	abbrevReplacements := map[string]string{
		"M.Tech": "MTech",
		"M.Sc":   "MSc",
		"M.E":    "ME",
		"B.Tech": "BTech",
		"B.Sc":   "BSc",
		"B.E":    "BE",
		"Ph.D":   "PhD",
		"Ph.D.":  "PhD",
		"S.O.P":  "SOP",
		"C.V":    "CV",
		"R.&.D":  "RND",
		"R&D":    "RND",
		"I.E.L.T.S": "IELTS",
		"G.R.E":     "GRE",
		"G.M.A.T":   "GMAT",
	}
	for oldAbbrev, newAbbrev := range abbrevReplacements {
		cleaned = strings.ReplaceAll(cleaned, oldAbbrev, newAbbrev)
	}

	for strings.Contains(cleaned, "  ") {
		cleaned = strings.ReplaceAll(cleaned, "  ", " ")
	}

	// Auto-convert raw standalone digit strings to English words & Devanagari English terms to English
	words := strings.Split(cleaned, " ")
	for i, w := range words {
		cleanW := strings.Trim(w, ".,!?\"'")
		if wordEquivalent, ok := digitReplacements[cleanW]; ok {
			words[i] = strings.Replace(w, cleanW, wordEquivalent, 1)
		} else if engEquivalent, ok := devanagariEnglishReplacements[cleanW]; ok {
			words[i] = strings.Replace(w, cleanW, engEquivalent, 1)
		}
	}
	cleaned = strings.Join(words, " ")

	return strings.TrimSpace(cleaned)
}

func trimRedundantOpeningFluff(clauseText string) string {
	trimmed := strings.TrimSpace(clauseText)
	if trimmed == "" {
		return ""
	}

	var tagPrefix string
	if strings.HasPrefix(trimmed, "[") {
		endIdx := strings.Index(trimmed, "]")
		if endIdx != -1 {
			tagPrefix = trimmed[:endIdx+1] + " "
			trimmed = strings.TrimSpace(trimmed[endIdx+1:])
		}
	}

	redundantPrefixes := []string{
		"Absolutely!", "Absolutely,", "Absolutely",
		"Awesome!", "Awesome,", "Awesome",
		"Great!", "Great,", "Great",
		"Definitely!", "Definitely,", "Definitely",
		"Indeed!", "Indeed,", "Indeed",
		"Perfect!", "Perfect,", "Perfect",
		"Exactly!", "Exactly,", "Exactly",
		"Of course!", "Of course,", "Of course",
		"Ji bilkul!", "Ji bilkul,", "Ji bilkul",
		"Haan!", "Haan,", "Haan",
		"Ji!", "Ji,",
		"Sure!", "Sure,", "Sure",
		"Alright!", "Alright,", "Alright",
		"Got it!", "Got it,", "Got it",
		"Okay!", "Okay,", "Okay",
		"Understood!", "Understood,", "Understood",
	}

	for _, prefix := range redundantPrefixes {
		if strings.HasPrefix(trimmed, prefix) {
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
			break
		}
	}

	if trimmed == "" {
		return ""
	}
	return tagPrefix + trimmed
}

func (h *CallHandler) callOpenAIGPTStreamForCall(ctx context.Context, sessionID, companyName, customerName, phone, query, contextBlock, spokenFillerText string, history []map[string]string, onClause func(string)) (string, error) {
	apiKey := config.Settings.OpenaiApiKey
	if apiKey == "" {
		fallbackMsg := "I am having trouble connecting to my backend service."
		if onClause != nil {
			onClause(fallbackMsg)
		}
		return fallbackMsg, nil
	}

	compDir := resolveCompanyDir(config.Settings.DocsBasePath, companyName)
	promptPath := filepath.Join(compDir, "system_prompt.txt")
	systemPromptBytes, err := os.ReadFile(promptPath)
	systemPrompt := ""
	if err == nil && len(systemPromptBytes) > 0 {
		systemPrompt = string(systemPromptBytes)
	} else {
		systemPrompt = fmt.Sprintf("You are Riya, a sharp, confident sales representative calling on behalf of %s. Sound 100%% human, natural, and helpful on this OUTBOUND call.", companyName)
	}

	guardrailsPath := filepath.Join(compDir, "guardrails.txt")
	if guardrailBytes, err := os.ReadFile(guardrailsPath); err == nil && len(guardrailBytes) > 0 {
		systemPrompt += "\n\n[STRICT TOPIC BOUNDARY & CONVERSATIONAL GUARDRAILS]\n" + string(guardrailBytes)
	}
	profileContext := fmt.Sprintf("\n\n[CUSTOMER PROFILE & OUTBOUND CALL CONTEXT]\n"+
		"- Customer Name: %s\n"+
		"- Customer Phone: %s\n"+
		"- Target Product: %s Services\n"+
		"- OUTBOUND SALES DIRECTIVE: You initiated this outbound call. YOU ARE LEADING THE CONVERSATION. Never ask passive inbound support questions like 'How can I assist you today?' or 'What do you want to know?'. State why you called, uncover pain, present value, and drive towards booking a meeting.\n",
		customerName, phone, companyName)
	if spokenFillerText != "" {
		profileContext += fmt.Sprintf("- SPOKEN FILLER ACKNOWLEDGEMENT: The audio player ALREADY SPOKE this filler out loud to the customer: %q. DO NOT repeat filler words, opening affirmations, or redundant statements like 'Absolutely!', 'Sure!', 'Got it!', 'Awesome!', 'Great!', 'Ji bilkul!', 'Haan', 'Definitely!', or 'Of course!'. CONTINUE Seamlessly from the filler by jumping DIRECTLY into the core explanation or answer!\n", spokenFillerText)
	}
	if db.RedisClient != nil && sessionID != "" {
		configKey := fmt.Sprintf("call_config:%s", sessionID)
		if refID, err := db.RedisClient.HGet(context.Background(), configKey, "preliminary_call_id").Result(); err == nil && refID != "" {
			configKey = fmt.Sprintf("call_config:%s", refID)
		}
		if val, err := db.RedisClient.HGet(context.Background(), configKey, "initial_language").Result(); err == nil && (val == "hindi" || val == "hi") {
			profileContext += "- INITIAL LANGUAGE PREFERENCE: The caller initially chose HINDI for Turn 1. HOWEVER, YOU MUST DYNAMICALLY MATCH THE CALLER'S LANGUAGE ON EVERY SINGLE TURN! IF THE CALLER SPEAKS IN PURE ENGLISH (e.g. 'Yeah, tell me something more', 'What is the process?', 'Can you speak in English?'), YOU MUST IMMEDIATELY SWITCH TO 100% PURE ENGLISH FOR THAT TURN!\n"
		}
	}
	profileContext += loadCompanyMetadata(compDir)
	systemPrompt += profileContext

	if contextBlock != "" {
		systemPrompt += "\n\n[RETRIEVED DOCUMENT CONTEXT]\n" + contextBlock
	}

	hinglishRuleBlock := "\n\nUNIVERSAL DYNAMIC LANGUAGE MATCHING & TTS SCRIPT RULE:\n" +
		"1. DYNAMIC FLUID LANGUAGE SWITCHING (HIGHEST PRIORITY MANDATE):\n" +
		"   - YOU ARE FULLY MULTILINGUAL AND MUST SWITCH LANGUAGES FREELY ON EVERY TURN DEPENDING ON WHAT THE USER SAYS!\n" +
		"   - IF THE USER SPEAKS IN PURE ENGLISH (e.g. 'Yeah, tell me something more', 'What is the eligibility?', 'Tell me in English'): YOU MUST IMMEDIATELY SWITCH TO 100% PURE ENGLISH ONLY! DO NOT USE HINDI/DEVANAGARI IN THAT TURN!\n" +
		"   - IF THE USER SPEAKS IN HINDI OR HINGLISH: REPLY IN NATURAL HINGLISH (DEVANAGARI HINDI CONNECTORS WITH LATIN ENGLISH KEYWORDS)!\n" +
		"   - ❌ NEVER STAY LOCKED IN HINDI IF THE USER SPEAKS IN ENGLISH! MATCH THE CALLER'S LANGUAGE DYNAMICALLY ON EVERY TURN!\n" +
		"1.1 EMBED IN-LINE EMOTION TAGS FOR HIGH-ENERGY TTS (MANDATORY):\n" +
		"   - YOU MUST EMBED SQUARE BRACKET EMOTION TAGS like [excited], [enthusiastic], [joyful], [warm] at start of replies & right before key facts (stipends, zero fees, salary, visa)!\n" +
		"2. CASUAL NATURAL SPOKEN HINGLISH (100% HUMAN LIKE URBAN INDIAN PROFESSIONALS):\n" +
		"   - You must sound like a real, smart, energetic Indian sales girl speaking natural everyday Hinglish on the phone!\n" +
		"   - ALL KEYWORDS, NOUNS, NUMBERS, AMOUNTS, CURRENCIES, PHDs, POSTDOCS, STIPENDS, SCHOLARSHIPS, RESEARCH, OPPORTUNITY, CHANCE, BENEFITS, VISA, HOUSING, PROCESS, EXAMS, IELTS, SCORES, PERCENT, DISCOUNT, FEES, AND TECHNICAL TERMS MUST STRICTLY BE WRITTEN IN PURE ENGLISH LATIN SCRIPT!\n" +
		"3. WRITE ALL NUMBERS IN FULL SPOKEN ENGLISH WORDS (CRITICAL FOR TTS - ZERO DIGITS):\n" +
		"   - NEVER USE RAW DIGITS (like 3.6, 4.2, 5.1, 43, 100, 6.5, 2) AND NEVER USE HINDI DEVANAGARI NUMBERS (like तीन लाख साठ हजार, चार लाख)!\n" +
		"   - ✅ ALWAYS WRITE NUMBERS IN PURE SPOKEN ENGLISH WORDS: 'three point six Lakhs to four point two Lakhs rupees per month', 'five point one Lakhs to five point seven Lakhs rupees per month', 'forty three Lakhs per year', 'one hundred percent visa support', 'six point five IELTS score', 'two minutes'!\n" +
		"4. ABSOLUTELY FORBIDDEN BOOKISH HINDI WORDS (MUST USE LATIN ENGLISH INSTEAD):\n" +
		"   - ❌ NEVER WRITE 'अनुसंधान' -> Write 'research'!\n" +
		"   - ❌ NEVER WRITE 'अवसर' or 'मौका' -> Write 'opportunity' or 'chance'!\n" +
		"   - ❌ NEVER WRITE 'छात्रवृत्ति' -> Write 'scholarship' or 'stipend'!\n" +
		"   - ❌ NEVER WRITE 'राशि' -> Write 'amount' or 'stipend'!\n" +
		"   - ❌ NEVER WRITE 'विश्वविद्यालय' -> Write 'university'!\n" +
		"   - ❌ NEVER WRITE 'संस्थान' -> Write 'institute'!\n" +
		"   - ❌ NEVER WRITE 'आवास' -> Write 'housing'!\n" +
		"   - ❌ NEVER WRITE 'वीज़ा' -> Write 'visa'!\n" +
		"   - ❌ NEVER WRITE 'प्रतिशत' -> Write 'percent'!\n" +
		"   - ❌ NEVER WRITE 'छूट' -> Write 'discount'!\n" +
		"   - ❌ NEVER WRITE 'योग्य' or 'पात्र' -> Write 'eligible'!\n" +
		"   - ❌ NEVER WRITE 'मुफ़्त' or 'निशुल्क' -> Write 'free'!\n" +
		"   - ❌ NEVER WRITE 'लाभ' -> Write 'benefit'!\n" +
		"   - ❌ NEVER WRITE 'ऑफ़र' -> Write 'offer'!\n" +
		"5. DEVANAGARI IS ONLY FOR BASIC CASUAL HINDI CONNECTORS/VERBS:\n" +
		"   - Write ONLY basic spoken Hindi connecting words/verbs in Devanagari script (e.g., 'बिल्कुल', 'मैं', 'आप', 'बोल रही हूँ', 'के लिए', 'भी', 'होते हैं', 'मिलेगा', 'क्या आप', 'चाहेंगे', 'बताती हूँ', 'समझती हूँ').\n" +
		"   - Keep it 100% natural, modern, friendly, and human!\n" +
		"   - ❌ DO NOT USE 'व्यवसाय' or 'व्यापार' -> Write 'business'!\n" +
		"   - ✅ ALWAYS USE NATURAL SPOKEN ENGLISH NOUNS inside Hindi sentences!\n" +
		"6. ZERO NEWLINES AND CLEAN DEGREE WORDS (STRICT MANDATE):\n" +
		"   - ❌ NEVER OUTPUT NEWLINE CHARACTERS (\\n)! Output continuous single-line spoken text only.\n" +
		"   - ALWAYS write degrees and acronyms as clean single words without dots or hyphens: MTech (NOT M.Tech), MSc (NOT M.Sc), ME (NOT M.E), BTech (NOT B.Tech), BSc (NOT B.Sc), PhD (NOT Ph.D), SOP (NOT S.O.P), CV (NOT C.V), RND (NOT R&D).\n" +
		"9. INTELLIGENT CONTEXTUAL INFERENCE FOR ASR TYPOS & MISHEARINGS (CRITICAL):\n" +
		"   - Speech-to-text (ASR) engines often mishear spoken words and output minor phonetic typos or misspellings (e.g. transcribing 'Founder' as 'Pounder'/'Faunder'/'Fonter', 'SEO' as 'aco'/'eso', 'Google' as 'Geegel', 'package' as 'pakej', 'price' as 'prais').\n" +
		"   - NEVER take raw ASR typos literally or claim 'I don't know who/what that is'!\n" +
		"   - ALWAYS infer the intended concept from your [RETRIEVED DOCUMENT CONTEXT] and [DYNAMIC COMPANY PROFILE & LEADERSHIP METADATA] based on phonetic similarity and business context!\n" +
		"10. EVERY RESPONSE MUST REMAIN OPEN & ASK A NEW ENGAGING QUESTION (STRICT MANDATE):\n" +
		"   - In EVERY single response, structure your reply in 3 parts: 1) Brief Acknowledgment, 2) Direct Answer/Value, 3) ALWAYS END WITH A NEW OPEN DISCOVERY OR CLOSING QUESTION!\n" +
		"   - ❌ NEVER END PASSIVELY with statements like 'Have a good day', 'Let me know if you have questions', or ending with a period without a question hook!\n" +
		"   - ✅ ALWAYS end with an engaging open question driving to the next step (e.g. 'क्या आप हमारे packages explore करना चाहेंगे?', 'Would afternoon work best for a 5-minute chat?').\n" +
		"11. PROACTIVE TELESALES CLOSING & BEST SELLING STRATEGY (STRICT MANDATE):\n" +
		"   - You are a proactive telesales closer, NOT a passive support agent!\n" +
		"   - In EVERY response, actively promote the product using a BEST SELLING STRATEGY (highlight high-ROI savings, 10% cashback, zero contracts, 10-day trial, or traffic growth).\n" +
		"   - Pitch the sale directly: ask to book a consultation, select a package, or complete Video KYC!\n" +
		"12. RELENTLESS SALES CLOSING & PERSUASIVE OBJECTION PIVOTING (NEVER GIVE UP — STRICT MANDATE):\n" +
		"   - Top telesales closers NEVER give up easily! If the prospect hesitates, says 'not interested', 'busy', 'don't need it', or pushes back:\n" +
		"   - ❌ STRICTLY FORBIDDEN: NEVER SAY PASSIVE GIVING-UP STATEMENTS like 'कोई बात नहीं', 'अगर आपको बाद में interest हो...', 'कोई सवाल हो तो संपर्क करें', or 'Have a good day'!\n" +
		"   - ❌ STRICTLY FORBIDDEN: NEVER WRITE KEYWORDS, NAMES, OR NUMBERS IN DEVANAGARI! Do NOT write 'ओमिका' -> Write 'Omica'! Do NOT write 'कैशबैक' -> Write 'cashback'! Do NOT write 'फ़ायदे' -> Write 'benefits'! Do NOT write 'सारांश' -> Write 'quick summary'! Do NOT write 'बातचीत' -> Write 'call' or 'chat'!\n" +
		"   - ✅ PERFECT DEVANAGARI + LATIN ENGLISH KEYWORDS EXAMPLE:\n" +
		"     \"मैं समझती हूँ, Omica! [empathetic] बहुत से customers को starting में ऐसा ही महसूस होता है। लेकिन जब वे cashback और benefits देखते हैं, तो बहुत happy होते हैं! [excited] क्या मैं आपको WhatsApp पर quick summary भेज सकती हूँ, या फिर कल afternoon एक short two minute call कर सकते हैं?\"\n" +
		"13. EMOTIONAL PERSUASION & EMPATHETIC SELLING TECHNIQUES (STRICT MANDATE):\n" +
		"   - Build a deep emotional connection and trigger emotional motivations in every turn!\n" +
		"   - ALL NAMES, KEYWORDS, NOUNS, BRANDS & NUMBERS MUST REMAIN IN PURE LATIN ENGLISH SCRIPT (e.g. Omica, customers, cashback, benefits, WhatsApp, summary, call, HDFC, Swiggy, fifteen thousand, two)!\n" +
		"   - ✅ PERFECT DEVANAGARI + LATIN ENGLISH KEYWORDS EXAMPLE:\n" +
		"     \"मैं समझती हूँ, Omica! [warm] महीने का fifteen thousand rupees save करना सच में बहुत rewarding feeling होती है! [excited] क्या आप Swiggy पर ten percent cashback का discount claim करना चाहेंगे?\"\n"
	systemPrompt += hinglishRuleBlock

	messages := []map[string]string{
		{"role": "system", "content": systemPrompt},
	}
	if len(history) > 20 {
		history = history[len(history)-20:]
	}
	for _, turn := range history {
		messages = append(messages, turn)
	}
	messages = append(messages, map[string]string{"role": "user", "content": query})

	payload := map[string]interface{}{
		"model":       "gpt-4o-mini",
		"messages":    messages,
		"temperature": 0.6,
		"stream":      true,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(payloadBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("OpenAI API error (status %d): %s", resp.StatusCode, string(errBody))
	}

	scanner := bufio.NewScanner(resp.Body)
	var fullReply strings.Builder
	var currentClause strings.Builder

	for scanner.Scan() {
		if ctx.Err() != nil {
			log.Printf("🚨 [TURN PREEMPTED] OpenAI HTTP stream context cancelled mid-sentence for session %s", sessionID)
			return fullReply.String(), ctx.Err()
		}
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break
			}
			var chunk openAIStreamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err == nil && len(chunk.Choices) > 0 {
				content := chunk.Choices[0].Delta.Content
				if content != "" {
					fullReply.WriteString(content)
					currentClause.WriteString(content)

					if shouldFlushClause(currentClause.String()) {
						clauseText := strings.TrimSpace(currentClause.String())
						if clauseText != "" && onClause != nil {
							onClause(clauseText)
						}
						currentClause.Reset()
					}
				}
			}
		}
	}

	if remaining := strings.TrimSpace(currentClause.String()); remaining != "" {
		if onClause != nil {
			onClause(remaining)
		}
	}

	return strings.TrimSpace(fullReply.String()), nil
}

func resolveCompanyDir(basePath, companyName string) string {
	docsDir := filepath.Join(basePath, companyName)
	if _, err := os.Stat(docsDir); err == nil {
		return docsDir
	}
	targetDir := strings.ToLower(strings.TrimSpace(companyName))
	if entries, err := os.ReadDir(basePath); err == nil {
		for _, entry := range entries {
			if entry.IsDir() && strings.ToLower(entry.Name()) == targetDir {
				return filepath.Join(basePath, entry.Name())
			}
		}
	}
	return docsDir
}
