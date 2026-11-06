package websocket

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
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
	"voice-agent-go/internal/services/rag"
	"voice-agent-go/internal/services/stt"

	"github.com/gorilla/websocket"
)

type ChatHandler struct {
	ragProvider    rag.RAGProvider
	ttsService     *services.TTSService
	sttProvider    stt.STTProvider
	emotionService *services.EmotionService
	httpClient     *http.Client
	sessions       map[string]*ChatSession
	mu             sync.Mutex
}

type ChatSession struct {
	SessionID     string
	CompanyName   string
	CustomerName  string
	CustomerPhone string
	LLMModel      string
	History       []map[string]string
}

func NewChatHandler(ragProvider rag.RAGProvider, ttsService *services.TTSService, sttProvider stt.STTProvider, emotionService *services.EmotionService) *ChatHandler {
	return &ChatHandler{
		ragProvider:    ragProvider,
		ttsService:     ttsService,
		sttProvider:    sttProvider,
		emotionService: emotionService,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		sessions: make(map[string]*ChatSession),
	}
}

// Serve Chat workspace HTML Page
func (h *ChatHandler) HandleRenderWorkspace(w http.ResponseWriter, r *http.Request) {
	model := r.URL.Query().Get("llm_model")
	companyName := r.URL.Query().Get("company_name")
	ragProvider := r.URL.Query().Get("rag_provider")
	customerName := r.URL.Query().Get("customer_name")
	customerPhone := r.URL.Query().Get("customer_phone")

	if r.Method == http.MethodPost {
		var req struct {
			LLMModel      string `json:"llm_model"`
			CompanyName   string `json:"company_name"`
			RagProvider   string `json:"rag_provider"`
			CustomerName  string `json:"customer_name"`
			CustomerPhone string `json:"customer_phone"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if model == "" {
			model = req.LLMModel
		}
		if companyName == "" {
			companyName = req.CompanyName
		}
		if ragProvider == "" {
			ragProvider = req.RagProvider
		}
		if customerName == "" {
			customerName = req.CustomerName
		}
		if customerPhone == "" {
			customerPhone = req.CustomerPhone
		}
	}
	if model == "" {
		model = "openai:gpt-4o-mini"
	}
	if companyName == "" {
		companyName = "HDFC"
	}
	if customerName == "" {
		customerName = "Yash"
	}
	if customerPhone == "" {
		customerPhone = "+15550199"
	}

	sessionID := fmt.Sprintf("chat_%d", time.Now().UnixNano())

	if ragProvider != "" && db.RedisClient != nil {
		ctx := context.Background()
		_ = db.RedisClient.HSet(ctx, fmt.Sprintf("call_config:%s", sessionID), "rag_provider", ragProvider).Err()
	}
	h.mu.Lock()
	h.sessions[sessionID] = &ChatSession{
		SessionID:     sessionID,
		CompanyName:   companyName,
		CustomerName:  customerName,
		CustomerPhone: customerPhone,
		LLMModel:      model,
		History:       []map[string]string{},
	}
	h.mu.Unlock()

	// Initialize RAG on the configured provider using the target company namespace
	err := h.ragProvider.Initialize(sessionID, companyName, "00000000-0000-0000-0000-000000000001", "+15550199")
	if err != nil {
		log.Printf("[Chat Handler] RAG initialization warning: %v", err)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	t, _ := template.New("workspace").Parse(workspaceHTML)
	t.Execute(w, map[string]string{
		"SessionID":   sessionID,
		"LLMModel":    model,
		"CompanyName": companyName,
		"STTProvider": h.sttProvider.Type(),
	})
}

// Handle WebSocket Text Chat loop (supports text + Deepgram audio streaming)
func (h *ChatHandler) HandleWSChat(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[Chat WS] Upgrade failed: %v", err)
		return
	}
	defer conn.Close()
	conn.SetReadLimit(32 * 1024 * 1024) // 32MB limit for large audio payloads

	// Thread-safe WebSocket writer (Deepgram transcript goroutine + main loop both write)
	var writeMu sync.Mutex
	safeWrite := func(payload interface{}) {
		data, _ := json.Marshal(payload)
		writeMu.Lock()
		conn.WriteMessage(websocket.TextMessage, data)
		writeMu.Unlock()
	}

	// Extract session ID from URL path or query
	parts := strings.Split(r.URL.Path, "/")
	sessionID := parts[len(parts)-1]

	h.mu.Lock()
	sess, exists := h.sessions[sessionID]
	h.mu.Unlock()

	if !exists {
		log.Printf("[Chat WS] Session not found: %s", sessionID)
		return
	}

	log.Printf("[Chat WS] Connection opened for session: %s (STT: %s)", sessionID, h.sttProvider.Type())

	// Send initial greeting loaded from file
	greetingPath := filepath.Join(config.Settings.DocsBasePath, sess.CompanyName, "greeting.txt")
	greetingBytes, err := os.ReadFile(greetingPath)
	greetingText := "Hello! How can I help you today?"
	if err == nil {
		greetingText = strings.TrimSpace(string(greetingBytes))
	} else {
		log.Printf("[Chat WS] Warning: greeting.txt not found for %s, using default", sess.CompanyName)
	}
	sess.History = append(sess.History, map[string]string{"role": "assistant", "content": greetingText})

	safeWrite(map[string]interface{}{
		"sender": "assistant",
		"text":   greetingText,
		"rag":    []interface{}{},
	})

	defer func() {
		h.mu.Lock()
		delete(h.sessions, sessionID)
		h.mu.Unlock()
		h.ragProvider.Destroy(sessionID)
		log.Printf("[Chat WS] Connection closed & RAG memory freed for session: %s", sessionID)
	}()

	// Active Deepgram STT session (nil when mic is off)
	type activeSTT struct {
		session stt.STTSession
		cancel  context.CancelFunc
	}
	var currentSTT *activeSTT
	stopCurrentSTT := func() {
		if currentSTT != nil {
			if currentSTT.cancel != nil {
				currentSTT.cancel()
			}
			if currentSTT.session != nil {
				currentSTT.session.Close()
			}
			currentSTT = nil
		}
	}
	defer stopCurrentSTT()

	// processQuery handles the full RAG + LLM + TTS pipeline for a user query
	processQuery := func(query string, audioBytes []byte) {
		log.Printf("[Chat WS] User Query: %q", query)

		// 1. Perform C++ Text & Audio Emotion Classification over gRPC
		var emotionBlock string
		if h.emotionService != nil {
			textScores, textErr := h.emotionService.ClassifyText(query, sessionID)
			if textErr != nil {
				log.Printf("[Chat CIE ERROR] Text emotion classification failed: %v", textErr)
			} else if textScores != nil {
				log.Printf("[Chat CIE SUCCESS] Text Emotion -> Joy: %.2f, Anger: %.2f, Sadness: %.2f, Neutral: %.2f",
					textScores.Joy, textScores.Anger, textScores.Sadness, textScores.Neutral)
			}

			var audioScores *services.EmotionScores
			if len(audioBytes) > 0 {
				var audioErr error
				audioScores, audioErr = h.emotionService.ClassifyAudio(audioBytes, sessionID)
				if audioErr != nil {
					log.Printf("[Chat CIE ERROR] Audio emotion classification failed: %v", audioErr)
				} else if audioScores != nil {
					log.Printf("[Chat CIE SUCCESS] Audio Emotion -> Joy: %.2f, Anger: %.2f, Sadness: %.2f, Neutral: %.2f",
						audioScores.Joy, audioScores.Anger, audioScores.Sadness, audioScores.Neutral)
				}
			}

			if textScores != nil || audioScores != nil {
				emotionBlock = "\n[DETECTED CUSTOMER EMOTION STATE (C++ ML MODELS)]\n"
				if textScores != nil {
					emotionBlock += fmt.Sprintf("- Semantic Text Sentiment: Joy: %.0f%%, Anger: %.0f%%, Sadness: %.0f%%, Neutral: %.0f%%\n",
						textScores.Joy*100, textScores.Anger*100, textScores.Sadness*100, textScores.Neutral*100)
				}
				if audioScores != nil {
					emotionBlock += fmt.Sprintf("- Acoustic Vocal Tone: Joy: %.0f%%, Anger: %.0f%%, Sadness: %.0f%%, Neutral: %.0f%%\n",
						audioScores.Joy*100, audioScores.Anger*100, audioScores.Sadness*100, audioScores.Neutral*100)
				}
				emotionBlock += "Adapt your conversational tone appropriately based on the user's emotion."
			}
		}

		// 2. Perform dynamic RAG search
		ragResults, searchErr := h.ragProvider.Search(sessionID, query, "00000000-0000-0000-0000-000000000001")
		if searchErr != nil {
			log.Printf("[Chat WS] RAG search error: %v", searchErr)
		}

		// Format context block for LLM prompt
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

		// 2. Call OpenAI Chat Completions API
		replyText, llmErr := h.callOpenAIGPT(sess, query, contextBlock)
		if llmErr != nil {
			replyText = fmt.Sprintf("Error calling LLM API: %v", llmErr)
		}

		// 3. Synthesize Cartesia / OpenAI voice audio for Web UI playback
		audioB64 := ""
		if h.ttsService != nil && replyText != "" {
			ttsCtx, ttsCancel := context.WithTimeout(context.Background(), 8*time.Second)
			mulawBytes, ttsErr := h.ttsService.SynthesizeSpeechMulaw(ttsCtx, replyText)
			ttsCancel()
			if ttsErr != nil {
				log.Printf("[Chat WS TTS ERROR] Failed to synthesize audio: %v", ttsErr)
			} else if len(mulawBytes) == 0 {
				log.Printf("[Chat WS TTS ERROR] TTS returned 0 bytes (empty audio)")
			} else {
				audioB64 = encodeMulawToWavBase64(mulawBytes)
				log.Printf("[Chat WS TTS OK] Synthesized %d mulaw bytes -> base64 WAV (%d chars) for Web UI", len(mulawBytes), len(audioB64))
			}
		}

		// 4. Save exchange in session history
		sess.History = append(sess.History, map[string]string{"role": "user", "content": query})
		sess.History = append(sess.History, map[string]string{"role": "assistant", "content": replyText})

		// Send reply along with the RAG matched sources & Cartesia audio back to UI
		safeWrite(map[string]interface{}{
			"sender":    "assistant",
			"text":      replyText,
			"rag":       ragResults,
			"audio_b64": audioB64,
		})
	}

	for {
		_, message, readErr := conn.ReadMessage()
		if readErr != nil {
			break
		}

		var wsMsg map[string]string
		if err := json.Unmarshal(message, &wsMsg); err != nil {
			continue
		}

		msgType := wsMsg["type"]

		switch msgType {
		case "start_stt":
			// User clicked mic — start Deepgram streaming session
			if !h.sttProvider.RequiresServerProcessing() {
				continue // Chrome STT handles everything in browser
			}
			stopCurrentSTT()

			newSession, sttErr := h.sttProvider.NewSession(sessionID)
			if sttErr != nil {
				log.Printf("[Chat WS STT ERROR] Failed to start STT session: %v", sttErr)
				safeWrite(map[string]interface{}{"type": "stt_error", "text": sttErr.Error()})
				continue
			}
			safeWrite(map[string]interface{}{"type": "stt_ready"})

			// Background goroutine: read Deepgram transcripts and relay to browser
			sttCtx, cancel := context.WithCancel(context.Background())
			currentSTT = &activeSTT{
				session: newSession,
				cancel:  cancel,
			}

			go func(ctx context.Context, session stt.STTSession) {
				for {
					select {
					case <-ctx.Done():
						return
					case result, ok := <-session.Transcripts():
						if !ok {
							return
						}
						safeWrite(map[string]interface{}{
							"type":     "transcript",
							"text":     result.Text,
							"is_final": result.IsFinal,
						})
					}
				}
			}(sttCtx, newSession)

		case "audio":
			// Raw audio chunk from browser mic → relay to Deepgram
			if currentSTT == nil || currentSTT.session == nil {
				log.Printf("[Chat WS STT WARN] Audio chunk received but sttSession is nil")
				continue
			}
			audioData := wsMsg["data"]
			if audioData == "" {
				continue
			}
			pcmBytes, decErr := base64.StdEncoding.DecodeString(audioData)
			if decErr != nil {
				log.Printf("[Chat WS STT ERROR] Base64 decode error: %v", decErr)
				continue
			}
			if sendErr := currentSTT.session.SendAudio(pcmBytes); sendErr != nil {
				log.Printf("[Chat WS STT ERROR] Audio send error: %v", sendErr)
			} else {
				log.Printf("[Chat WS STT] Relayed %d audio bytes to Deepgram", len(pcmBytes))
			}

		case "stt_audio_payload":
			// Approach A: Full recorded audio buffer payload from browser mic
			audioB64 := wsMsg["data"]
			if audioB64 == "" {
				continue
			}
			audioBytes, decErr := base64.StdEncoding.DecodeString(audioB64)
			if decErr != nil {
				log.Printf("[Chat WS STT ERROR] Base64 payload decode error: %v", decErr)
				safeWrite(map[string]interface{}{"type": "stt_error", "text": "Audio decoding error"})
				continue
			}
			log.Printf("[Chat WS STT] Processing complete audio payload (%d bytes) via Deepgram REST...", len(audioBytes))
			transcriptText, sttErr := h.sttProvider.TranscribeAudioPayload(audioBytes)
			if sttErr != nil {
				log.Printf("[Chat WS STT ERROR] Payload STT error: %v", sttErr)
				safeWrite(map[string]interface{}{"type": "stt_error", "text": sttErr.Error()})
				continue
			}
			log.Printf("[Chat WS STT SUCCESS] Full audio transcript: %q", transcriptText)
			if transcriptText != "" {
				safeWrite(map[string]interface{}{
					"type":     "transcript",
					"text":     transcriptText,
					"is_final": true,
				})
				processQuery(transcriptText, audioBytes)
			}

		case "stop_stt":
			// User released mic — gracefully close Deepgram session
			if currentSTT != nil {
				toStop := currentSTT
				currentSTT = nil
				go func(s *activeSTT) {
					if s.session != nil {
						s.session.Close()
					}
					if s.cancel != nil {
						s.cancel()
					}
				}(toStop)
			}

		default:
			// Regular text message (typed input or Chrome STT transcript)
			query := wsMsg["text"]
			if query == "" {
				continue
			}
			processQuery(query, nil)
		}
	}

	// Cleanup STT session on disconnect
	stopCurrentSTT()
}

type openAIChoice struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
}

type openAIChatResponse struct {
	Choices []openAIChoice `json:"choices"`
}

func (h *ChatHandler) callOpenAIGPT(sess *ChatSession, query string, contextBlock string) (string, error) {
	apiKey := config.Settings.OpenaiApiKey
	if apiKey == "" {
		return "Please configure your OPENAI_API_KEY in the Go gateway `.env` file to chat.", nil
	}

	promptPath := filepath.Join(config.Settings.DocsBasePath, sess.CompanyName, "system_prompt.txt")
	systemPromptBytes, err := os.ReadFile(promptPath)
	systemPrompt := ""
	if err == nil {
		systemPrompt = string(systemPromptBytes)
	} else {
		systemPrompt = "You are a helpful customer assistant. Rely primarily on the provided retrieved context to answer. Keep your answers brief and natural."
		log.Printf("[Chat Handler] Warning: could not read system prompt file for %s, using fallback: %v", sess.CompanyName, err)
	}
	profileContext := fmt.Sprintf("\n\n[CUSTOMER PROFILE CONTEXT]\n- Customer Name: %s\n- Customer Phone: %s\n- Target Product: %s Credit Card\n", sess.CustomerName, sess.CustomerPhone, sess.CompanyName)
	systemPrompt += profileContext

	if contextBlock != "" {
		systemPrompt += "\n\n[RETRIEVED DOCUMENT CONTEXT]\n" + contextBlock
		log.Printf("[Chat Handler LLM Context] Injected C++ RAG & Emotion Context:\n%s", contextBlock)
	}

	messages := []map[string]string{
		{"role": "system", "content": systemPrompt},
	}
	// Append conversation history (truncate to last 10 turns = 20 messages to stay within token limits)
	history := sess.History
	if len(history) > 20 {
		history = history[len(history)-20:]
	}
	for _, turn := range history {
		messages = append(messages, turn)
	}
	// Append current turn
	messages = append(messages, map[string]string{"role": "user", "content": query})

	// Use the model from session config (strip provider prefix like "openai:")
	modelName := sess.LLMModel
	if idx := strings.Index(modelName, ":"); idx != -1 {
		modelName = modelName[idx+1:]
	}
	payload := map[string]interface{}{
		"model":    modelName,
		"messages": messages,
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

	log.Printf("[Chat Handler] Dispatching live HTTP POST to OpenAI API (Endpoint: https://api.openai.com/v1/chat/completions, Model: %s)...", modelName)
	resp, err := h.httpClient.Do(req)
	if err != nil {
		log.Printf("[Chat Handler] Network error reaching OpenAI API: %v", err)
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		log.Printf("[Chat Handler] OpenAI API error (status %d): %s", resp.StatusCode, string(errBody))
		return "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(errBody))
	}

	var chatResp openAIChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		log.Printf("[Chat Handler] Error decoding OpenAI response: %v", err)
		return "", err
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("empty chat completions returned")
	}

	responseText := chatResp.Choices[0].Message.Content
	log.Printf("[Chat Handler] OpenAI API 200 OK Success! Generated response: %q", responseText)
	return responseText, nil
}

// Gorgeous responsive UI for the verification chat interface
const workspaceHTML = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Voice AI RAG Verification Workspace</title>
    <link href="https://fonts.googleapis.com/css2?family=Outfit:wght@300;400;500;600;700&display=swap" rel="stylesheet">
    <style>
        * {
            box-sizing: border-box;
            margin: 0;
            padding: 0;
            font-family: 'Outfit', sans-serif;
        }
        body {
            background: linear-gradient(135deg, #0e0b1e 0%, #08070f 100%);
            color: #e5e0f5;
            height: 100vh;
            display: flex;
            justify-content: center;
            align-items: center;
            overflow: hidden;
        }
        .container {
            width: 90%;
            max-width: 1200px;
            height: 85vh;
            background: rgba(25, 20, 45, 0.45);
            backdrop-filter: blur(16px);
            border: 1px solid rgba(255, 255, 255, 0.08);
            border-radius: 24px;
            display: flex;
            box-shadow: 0 20px 50px rgba(0, 0, 0, 0.5);
            overflow: hidden;
        }
        .chat-section {
            flex: 2;
            display: flex;
            flex-direction: column;
            border-right: 1px solid rgba(255, 255, 255, 0.08);
            background: rgba(10, 8, 20, 0.3);
        }
        .sidebar-section {
            flex: 1.1;
            display: flex;
            flex-direction: column;
            background: rgba(15, 12, 28, 0.6);
            padding: 24px;
            overflow-y: auto;
        }
        .header {
            padding: 20px 28px;
            background: rgba(30, 24, 55, 0.3);
            border-bottom: 1px solid rgba(255, 255, 255, 0.08);
            display: flex;
            align-items: center;
            justify-content: space-between;
        }
        .header h2 {
            font-size: 1.3rem;
            font-weight: 600;
            color: #b3a5ff;
            display: flex;
            align-items: center;
            gap: 10px;
        }
        .status-dot {
            width: 8px;
            height: 8px;
            background: #00ff88;
            border-radius: 50%;
            box-shadow: 0 0 10px #00ff88;
        }
        .chat-history {
            flex: 1;
            padding: 24px;
            overflow-y: auto;
            display: flex;
            flex-direction: column;
            gap: 16px;
        }
        .message {
            max-width: 80%;
            padding: 14px 18px;
            border-radius: 18px;
            font-size: 0.95rem;
            line-height: 1.5;
            animation: fadeIn 0.3s ease;
        }
        @keyframes fadeIn {
            from { opacity: 0; transform: translateY(10px); }
            to { opacity: 1; transform: translateY(0); }
        }
        .message.assistant {
            align-self: flex-start;
            background: rgba(60, 50, 110, 0.3);
            border: 1px solid rgba(135, 120, 240, 0.25);
            border-bottom-left-radius: 4px;
        }
        .message.user {
            align-self: flex-end;
            background: linear-gradient(135deg, #6c5ce7 0%, #a29bfe 100%);
            color: #fff;
            border-bottom-right-radius: 4px;
            box-shadow: 0 4px 15px rgba(108, 92, 231, 0.3);
        }
        /* Chat Input Area with Mic Button */
        .chat-input-area {
            padding: 20px 24px;
            background: rgba(18, 14, 34, 0.5);
            border-top: 1px solid rgba(255, 255, 255, 0.08);
            display: flex;
            gap: 12px;
        }
        .input-box {
            flex: 1;
            background: rgba(255, 255, 255, 0.04);
            border: 1px solid rgba(255, 255, 255, 0.1);
            border-radius: 14px;
            padding: 14px 20px;
            color: #fff;
            font-size: 0.95rem;
            outline: none;
            transition: all 0.3s ease;
        }
        .input-box:focus {
            border-color: #6c5ce7;
            background: rgba(255, 255, 255, 0.08);
            box-shadow: 0 0 10px rgba(108, 92, 231, 0.2);
        }
        .mic-btn {
            background: rgba(255, 255, 255, 0.06);
            color: #ff4757;
            border: 1px solid rgba(255, 71, 87, 0.3);
            border-radius: 14px;
            padding: 0 18px;
            font-size: 1.1rem;
            cursor: pointer;
            transition: all 0.3s ease;
            display: flex;
            align-items: center;
            justify-content: center;
        }
        .mic-btn.recording {
            background: #ff4757;
            color: #fff;
            box-shadow: 0 0 15px rgba(255, 71, 87, 0.6);
            animation: pulse 1.2s infinite;
        }
        @keyframes pulse {
            0% { transform: scale(1); }
            50% { transform: scale(1.05); }
            100% { transform: scale(1); }
        }
        .send-btn {
            background: #6c5ce7;
            color: #fff;
            border: none;
            border-radius: 14px;
            padding: 0 24px;
            font-size: 0.95rem;
            font-weight: 500;
            cursor: pointer;
            transition: all 0.3s ease;
        }
        .send-btn:hover {
            background: #5b4bc4;
            box-shadow: 0 4px 15px rgba(108, 92, 231, 0.4);
        }
        .sidebar-header {
            font-size: 1.1rem;
            font-weight: 600;
            color: #b3a5ff;
            margin-bottom: 20px;
            border-bottom: 1px solid rgba(255, 255, 255, 0.1);
            padding-bottom: 10px;
        }
        .source-card {
            background: rgba(255, 255, 255, 0.03);
            border: 1px solid rgba(255, 255, 255, 0.06);
            border-radius: 14px;
            padding: 16px;
            margin-bottom: 14px;
            font-size: 0.85rem;
            line-height: 1.45;
            transition: all 0.3s ease;
        }
        .source-card:hover {
            border-color: rgba(135, 120, 240, 0.4);
            background: rgba(135, 120, 240, 0.05);
        }
        .source-meta {
            display: flex;
            justify-content: space-between;
            margin-bottom: 8px;
            font-weight: 600;
            color: #a29bfe;
        }
        .source-score {
            color: #00ff88;
        }
        .empty-sources {
            text-align: center;
            color: rgba(255, 255, 255, 0.3);
            margin-top: 50px;
            font-style: italic;
            font-size: 0.9rem;
        }
    </style>
</head>
<body>
    <div class="container">
        <!-- Chat History Column -->
        <div class="chat-section">
            <div class="header">
                <h2><span class="status-dot"></span> Voice & RAG Testing Panel</h2>
                <span style="font-size: 0.85rem; color: rgba(255,255,255,0.4)">Model: {{.LLMModel}}</span>
            </div>
            
            <div class="chat-history" id="chatHistory">
                <!-- Messages populated dynamically -->
            </div>
            
            <div class="chat-input-area">
                <button class="mic-btn" id="micBtn" onclick="toggleMicrophone()" title="Speak using Microphone">🎙️</button>
                <input type="text" class="input-box" id="userInput" placeholder="Ask a question or click 🎙️ to speak..." onkeydown="if(event.key==='Enter') sendMessage()">
                <button class="send-btn" onclick="sendMessage()">Send</button>
            </div>
        </div>

        <!-- Sidebar RAG Source Column -->
        <div class="sidebar-section">
            <div class="sidebar-header">Retrieved RAG Vector Context</div>
            <div id="sourcesContainer">
                <div class="empty-sources">Ask a question or speak into microphone...</div>
            </div>
        </div>
    </div>

    <script>
        const sessionID = "{{.SessionID}}";
        const sttProvider = "{{.STTProvider}}"; // "deepgram" or "chrome"
        const wsProtocol = window.location.protocol === "https:" ? "wss:" : "ws:";
        const ws = new WebSocket(wsProtocol + "//" + window.location.host + "/v1/test/ws-chat/" + sessionID);

        const chatHistory = document.getElementById("chatHistory");
        const sourcesContainer = document.getElementById("sourcesContainer");
        const userInput = document.getElementById("userInput");
        const micBtn = document.getElementById("micBtn");

        let isRecording = false;

        // ==================== CHROME STT MODE ====================
        let recognition = null;
        if (sttProvider === "chrome") {
            if ('webkitSpeechRecognition' in window || 'SpeechRecognition' in window) {
                const SpeechRecognition = window.SpeechRecognition || window.webkitSpeechRecognition;
                recognition = new SpeechRecognition();
                recognition.continuous = false;
                recognition.interimResults = false;
                recognition.lang = 'hi-IN';

                recognition.onresult = (event) => {
                    const speechToText = event.results[0][0].transcript;
                    userInput.value = speechToText;
                    stopMicrophone();
                    sendMessage();
                };
                recognition.onerror = (event) => {
                    console.error("Mic error:", event.error);
                    stopMicrophone();
                };
                recognition.onend = () => { stopMicrophone(); };
            }
        }

        // ==================== DEEPGRAM STT MODE (Approach A: Record Full Audio → Send Payload) ====================
        let mediaStream = null;
        let mediaRecorder = null;
        let recordedChunks = [];

        function startDeepgramMic() {
            recordedChunks = [];
            navigator.mediaDevices.getUserMedia({ audio: { echoCancellation: true, noiseSuppression: true } })
            .then(stream => {
                mediaStream = stream;
                const options = MediaRecorder.isTypeSupported('audio/webm;codecs=opus')
                    ? { mimeType: 'audio/webm;codecs=opus' }
                    : (MediaRecorder.isTypeSupported('audio/webm') ? { mimeType: 'audio/webm' } : {});

                mediaRecorder = new MediaRecorder(stream, options);

                mediaRecorder.ondataavailable = (e) => {
                    if (e.data && e.data.size > 0) {
                        recordedChunks.push(e.data);
                    }
                };

                mediaRecorder.start(100); // Collect audio data slices
                console.log("[Deepgram STT] Recording started (Approach A: Full Audio Payload Mode)");
                userInput.placeholder = "Listening... Speak your query";
            })
            .catch(err => {
                console.error("Mic access error:", err);
                stopMicrophone();
            });
        }

        function stopDeepgramMic() {
            if (mediaRecorder && mediaRecorder.state !== 'inactive') {
                mediaRecorder.onstop = async () => {
                    if (recordedChunks.length === 0) return;
                    userInput.placeholder = "Transcribing audio...";
                    const audioBlob = new Blob(recordedChunks, { type: 'audio/webm' });
                    recordedChunks = [];

                    const buffer = await audioBlob.arrayBuffer();
                    const uint8 = new Uint8Array(buffer);
                    let binary = '';
                    for (let i = 0; i < uint8.length; i++) {
                        binary += String.fromCharCode(uint8[i]);
                    }
                    const b64 = btoa(binary);

                    if (ws.readyState === WebSocket.OPEN) {
                        console.log("[Deepgram STT] Sending full audio payload (" + audioBlob.size + " bytes) to server...");
                        ws.send(JSON.stringify({ type: "stt_audio_payload", data: b64 }));
                    }
                };
                try { mediaRecorder.stop(); } catch(e){}
                mediaRecorder = null;
            }

            if (mediaStream) {
                mediaStream.getTracks().forEach(t => t.stop());
                mediaStream = null;
            }
            console.log("[Deepgram STT] Mic stopped, payload dispatched");
        }

        // ==================== UNIFIED MIC CONTROLS ====================
        function toggleMicrophone() {
            if (isRecording) {
                stopMicrophone();
            } else {
                startMicrophone();
            }
        }

        function startMicrophone() {
            isRecording = true;
            micBtn.classList.add("recording");
            if (sttProvider === "deepgram") {
                startDeepgramMic();
            } else if (recognition) {
                recognition.start();
            } else {
                alert("Speech recognition is not supported in this browser.");
                stopMicrophone();
            }
        }

        function stopMicrophone() {
            isRecording = false;
            micBtn.classList.remove("recording");
            userInput.placeholder = "Type a message or click 🎙️ to talk...";
            if (sttProvider === "deepgram") {
                stopDeepgramMic();
            } else if (recognition) {
                try { recognition.stop(); } catch(e){}
            }
        }

        // ==================== WEB AUDIO API (TTS PLAYBACK) ====================
        let audioCtx = null;

        function getAudioContext() {
            if (!audioCtx) {
                const AudioCtx = window.AudioContext || window.webkitAudioContext;
                audioCtx = new AudioCtx();
            }
            if (audioCtx.state === 'suspended') {
                audioCtx.resume();
            }
            return audioCtx;
        }

        function base64ToArrayBuffer(base64) {
            const binaryString = window.atob(base64);
            const len = binaryString.length;
            const bytes = new Uint8Array(len);
            for (let i = 0; i < len; i++) {
                bytes[i] = binaryString.charCodeAt(i);
            }
            return bytes.buffer;
        }

        function playCartesiaAudio(base64Wav) {
            if (!base64Wav) return;
            try {
                const ctx = getAudioContext();
                const arrayBuffer = base64ToArrayBuffer(base64Wav);
                ctx.decodeAudioData(arrayBuffer, (buffer) => {
                    const source = ctx.createBufferSource();
                    source.buffer = buffer;
                    source.connect(ctx.destination);
                    source.start(0);
                    console.log("[Web UI] Playing Cartesia voice (" + buffer.duration.toFixed(2) + "s)");
                }, (err) => {
                    console.warn("[Web UI Audio] Decode fallback:", err);
                    const audio = new Audio("data:audio/wav;base64," + base64Wav);
                    audio.play().catch(e => console.error("Audio error:", e));
                });
            } catch(e) {
                console.error("[Web UI Audio]:", e);
            }
        }

        // ==================== WEBSOCKET MESSAGE HANDLER ====================
        ws.onmessage = (event) => {
            const data = JSON.parse(event.data);

            // Deepgram WebSocket handshake ready signal
            if (data.type === "stt_ready") {
                connectAudioPipeline();
                return;
            }

            // Handle Deepgram transcript messages
            if (data.type === "transcript") {
                if (data.is_final && data.text) {
                    appendMessage("user", data.text);
                    userInput.value = "";
                    userInput.placeholder = "Type a message or click 🎙️ to talk...";
                }
                return;
            }

            // Handle STT error
            if (data.type === "stt_error") {
                console.error("[STT Error]:", data.text);
                stopMicrophone();
                return;
            }

            // Handle assistant response
            if (data.sender) {
                appendMessage(data.sender, data.text);
            }
            if (data.sender === "assistant") {
                renderSources(data.rag);
                if (data.audio_b64) {
                    playCartesiaAudio(data.audio_b64);
                }
            }
        };

        // ==================== UI FUNCTIONS ====================
        function appendMessage(sender, text) {
            const msgDiv = document.createElement("div");
            msgDiv.classList.add("message", sender);
            msgDiv.innerText = text;
            chatHistory.appendChild(msgDiv);
            chatHistory.scrollTop = chatHistory.scrollHeight;
        }

        function renderSources(ragResults) {
            if (!ragResults || ragResults.length === 0) {
                sourcesContainer.innerHTML = '<div class="empty-sources">No documents matched the query.</div>';
                return;
            }
            sourcesContainer.innerHTML = "";
            ragResults.forEach(res => {
                const card = document.createElement("div");
                card.classList.add("source-card");
                card.innerHTML = 
                    "<div class=\"source-meta\">" +
                        "<span>📄 " + res.source + "</span>" +
                        "<span class=\"source-score\">Match: " + (res.score * 100).toFixed(1) + "%</span>" +
                    "</div>" +
                    "<div>" + res.text + "</div>";
                sourcesContainer.appendChild(card);
            });
        }

        function sendMessage() {
            getAudioContext();
            const text = userInput.value.trim();
            if (!text) return;
            appendMessage("user", text);
            ws.send(JSON.stringify({ text: text }));
            userInput.value = "";
        }
    </script>
</body>
</html>
`

var mulawToPcmTable = [256]int16{
	-32124, -31100, -30076, -29052, -28028, -27004, -25980, -24956,
	-23932, -22908, -21884, -20860, -19836, -18812, -17788, -16764,
	-15996, -15484, -14972, -14460, -13948, -13436, -12924, -12412,
	-11900, -11388, -10876, -10364, -9852, -9340, -8828, -8316,
	-7932, -7676, -7420, -7164, -6908, -6652, -6396, -6140,
	-5884, -5628, -5372, -5116, -4860, -4604, -4348, -4092,
	-3900, -3772, -3644, -3516, -3388, -3260, -3132, -3004,
	-2876, -2748, -2620, -2492, -2364, -2236, -2108, -1980,
	-1884, -1820, -1756, -1692, -1628, -1564, -1500, -1436,
	-1372, -1308, -1244, -1180, -1116, -1052, -988, -924,
	-876, -844, -812, -780, -748, -716, -684, -652,
	-620, -588, -556, -524, -492, -460, -428, -396,
	-372, -356, -340, -324, -308, -292, -276, -260,
	-244, -228, -212, -196, -180, -164, -148, -132,
	-120, -112, -104, -96, -88, -80, -72, -64,
	-56, -48, -40, -32, -24, -16, -8, 0,
	32124, 31100, 30076, 29052, 28028, 27004, 25980, 24956,
	23932, 22908, 21884, 20860, 19836, 18812, 17788, 16764,
	15996, 15484, 14972, 14460, 13948, 13436, 12924, 12412,
	11900, 11388, 10876, 10364, 9852, 9340, 8828, 8316,
	7932, 7676, 7420, 7164, 6908, 6652, 6396, 6140,
	5884, 5628, 5372, 5116, 4860, 4604, 4348, 4092,
	3900, 3772, 3644, 3516, 3388, 3260, 3132, 3004,
	2876, 2748, 2620, 2492, 2364, 2236, 2108, 1980,
	1884, 1820, 1756, 1692, 1628, 1564, 1500, 1436,
	1372, 1308, 1244, 1180, 1116, 1052, 988, 924,
	876, 844, 812, 780, 748, 716, 684, 652,
	620, 588, 556, 524, 492, 460, 428, 396,
	372, 356, 340, 324, 308, 292, 276, 260,
	244, 228, 212, 196, 180, 164, 148, 132,
	120, 112, 104, 96, 88, 80, 72, 64,
	56, 48, 40, 32, 24, 16, 8, 0,
}

func encodeMulawToWavBase64(mulawBytes []byte) string {
	if len(mulawBytes) == 0 {
		return ""
	}
	pcmBytes := make([]byte, len(mulawBytes)*2)
	for i, b := range mulawBytes {
		sample := services.MulawSampleToPcm16(b)
		binary.LittleEndian.PutUint16(pcmBytes[i*2:i*2+2], uint16(sample))
	}

	buf := new(bytes.Buffer)
	buf.WriteString("RIFF")
	binary.Write(buf, binary.LittleEndian, uint32(36+len(pcmBytes)))
	buf.WriteString("WAVEfmt ")
	binary.Write(buf, binary.LittleEndian, uint32(16))
	binary.Write(buf, binary.LittleEndian, uint16(1)) // PCM
	binary.Write(buf, binary.LittleEndian, uint16(1)) // Mono
	binary.Write(buf, binary.LittleEndian, uint32(8000))
	binary.Write(buf, binary.LittleEndian, uint32(16000))
	binary.Write(buf, binary.LittleEndian, uint16(2))
	binary.Write(buf, binary.LittleEndian, uint16(16))
	buf.WriteString("data")
	binary.Write(buf, binary.LittleEndian, uint32(len(pcmBytes)))
	buf.Write(pcmBytes)

	return base64.StdEncoding.EncodeToString(buf.Bytes())
}
