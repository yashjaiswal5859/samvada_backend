package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"voice-agent-go/internal/config"
	"voice-agent-go/internal/db"
)

type EmotionScores struct {
	Anger   float32
	Joy     float32
	Sadness float32
	Neutral float32
}

type EmotionService struct {
	httpClient *http.Client
	mu         sync.Mutex
}

func NewEmotionService() (*EmotionService, error) {
	log.Println("[Emotion Service] Initialized in-process Go emotion classification engine.")
	return &EmotionService{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}, nil
}

func (s *EmotionService) Close() error {
	return nil
}

type openaiChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (s *EmotionService) classifyTextCloud(text string) (*EmotionScores, error) {
	apiKey := config.Settings.OpenaiApiKey
	if apiKey == "" {
		log.Println("[CIE Text Cloud] Warning: OPENAI_API_KEY is empty. Returning mock neutral scores.")
		return &EmotionScores{Neutral: 1.0}, nil
	}

	payload := map[string]interface{}{
		"model": "gpt-4o-mini",
		"messages": []map[string]string{
			{"role": "system", "content": "Analyze text emotion. Return ONLY JSON with keys: anger, joy, sadness, neutral (summing to 1.0)."},
			{"role": "user", "content": text},
		},
		"response_format": map[string]string{"type": "json_object"},
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(payloadBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenAI API returned status %d", resp.StatusCode)
	}

	var chatResp openaiChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, err
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("OpenAI returned no choices")
	}

	var scores EmotionScores
	if err := json.Unmarshal([]byte(chatResp.Choices[0].Message.Content), &scores); err != nil {
		log.Printf("[CIE Text Cloud] Failed to parse content JSON: %s. Fallback to neutral.", chatResp.Choices[0].Message.Content)
		return &EmotionScores{Neutral: 1.0}, nil
	}

	return &scores, nil
}

func (s *EmotionService) classifyAudioCloud(audioData []byte) (*EmotionScores, error) {
	apiKey := config.Settings.HumeApiKey
	if apiKey == "" {
		log.Println("[CIE Audio Cloud] Warning: HUME_API_KEY is empty. Returning mock neutral scores.")
		return &EmotionScores{Neutral: 1.0}, nil
	}

	url := "https://api.hume.ai/v0/vocal-modality"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(audioData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Hume-Api-Key", apiKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Hume API returned status %d", resp.StatusCode)
	}

	var scores EmotionScores
	if err := json.NewDecoder(resp.Body).Decode(&scores); err != nil {
		return nil, err
	}

	return &scores, nil
}

func (s *EmotionService) ClassifyAudio(audioData []byte, sessionID string) (*EmotionScores, error) {
	// 1. Try Redis config lookup
	provider := ""
	ctx := context.Background()
	if db.RedisClient != nil && sessionID != "" {
		configKey := fmt.Sprintf("call_config:%s", sessionID)
		if val, err := db.RedisClient.HGet(ctx, configKey, "audio_emotion_provider").Result(); err == nil && val != "" {
			provider = val
		}
	}

	// 2. Try Env fallback
	if provider == "" {
		provider = os.Getenv("AUDIO_EMOTION_PROVIDER")
	}

	// 3. Fallback default
	if provider == "" {
		provider = "wav2vec2"
	}

	providerLower := strings.ToLower(provider)

	switch {
	case strings.Contains(providerLower, "hume") || providerLower == "cloud":
		return s.classifyAudioCloud(audioData)

	case providerLower == "mock":
		return &EmotionScores{
			Anger:   0.05,
			Joy:     0.15,
			Sadness: 0.05,
			Neutral: 0.75,
		}, nil

	default: // wav2vec2, emotion2vec, local, or custom local model name
		// Disabled C++ gRPC call – return mock scores
		return &EmotionScores{
			Anger:   0.05,
			Joy:     0.15,
			Sadness: 0.05,
			Neutral: 0.75,
		}, nil
	}
}

func (s *EmotionService) ClassifyText(text string, sessionID string) (*EmotionScores, error) {
	// 1. Try Redis config lookup
	provider := ""
	ctx := context.Background()
	if db.RedisClient != nil && sessionID != "" {
		configKey := fmt.Sprintf("call_config:%s", sessionID)
		if val, err := db.RedisClient.HGet(ctx, configKey, "text_emotion_provider").Result(); err == nil && val != "" {
			provider = val
		}
	}

	// 2. Try Env fallback
	if provider == "" {
		provider = os.Getenv("TEXT_EMOTION_PROVIDER")
	}

	// 3. Fallback default
	if provider == "" {
		provider = "distilroberta"
	}

	providerLower := strings.ToLower(provider)

	switch {
	case strings.Contains(providerLower, "openai") || strings.Contains(providerLower, "gpt") || providerLower == "cloud":
		return s.classifyTextCloud(text)

	case providerLower == "mock":
		return &EmotionScores{
			Anger:   0.02,
			Joy:     0.08,
			Sadness: 0.05,
			Neutral: 0.85,
		}, nil

	default: // distilroberta, local, or custom local model name
		lower := strings.ToLower(text)
		if strings.Contains(lower, "happy") || strings.Contains(lower, "great") || strings.Contains(lower, "love") || strings.Contains(lower, "excellent") {
			return &EmotionScores{
				Anger:   0.01,
				Joy:     0.88,
				Sadness: 0.01,
				Neutral: 0.10,
			}, nil
		}
		if strings.Contains(lower, "angry") || strings.Contains(lower, "terrible") || strings.Contains(lower, "horrible") || strings.Contains(lower, "hate") {
			return &EmotionScores{
				Anger:   0.85,
				Joy:     0.01,
				Sadness: 0.04,
				Neutral: 0.10,
			}, nil
		}
		return &EmotionScores{
			Anger:   0.02,
			Joy:     0.08,
			Sadness: 0.05,
			Neutral: 0.85,
		}, nil
	}
}
