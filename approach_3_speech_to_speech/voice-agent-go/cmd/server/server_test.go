package main

import (
	"log"
	"strings"
	"testing"

	"voice-agent-go/internal/config"
	"voice-agent-go/internal/services"
	"voice-agent-go/internal/services/rag"
)

func TestIntegrationPipeline(t *testing.T) {
	// 1. Initialize configuration with local C++ gRPC endpoints
	config.Settings.CppServerUrl = "localhost:50051"

	log.Println("[Test] Testing Go integration pipeline. Checking C++ gRPC connection...")

	// 2. Initialize the gRPC clients
	ragProvider, err := rag.NewHybridRAG()
	if err != nil {
		t.Logf("[Test] C++ Server not running on localhost:50051 (skipped real C++ check): %v", err)
		return // Skip integration check if local C++ server is not running
	}
	defer ragProvider.Close()

	emotionService, err := services.NewEmotionService()
	if err != nil {
		t.Fatalf("[Test] Failed to create emotion service: %v", err)
	}
	defer emotionService.Close()

	// 3. Test Local RAG session lifecycle calls to C++ gRPC
	sessionID := "test_session_123"
	company := "Swiggy"
	tenant := "00000000-0000-0000-0000-000000000001"
	phone := "+19999999"

	log.Println("[Test] Initializing RAG session on C++...")
	err = ragProvider.Initialize(sessionID, company, tenant, phone)
	if err != nil {
		if strings.Contains(err.Error(), "connection error") || strings.Contains(err.Error(), "Unavailable") {
			t.Skipf("[Test] C++ Server is offline. Skipping integration run: %v", err)
			return
		}
		t.Fatalf("[Test] Initialize failed: %v", err)
	}

	log.Println("[Test] Indexing dynamic memory on C++...")
	err = ragProvider.IndexDynamic(sessionID, "Customer requested vegan options.", "live_stream", tenant)
	if err != nil {
		t.Fatalf("[Test] IndexDynamic failed: %v", err)
	}

	log.Println("[Test] Searching RAG session on C++...")
	results, err := ragProvider.Search(sessionID, "vegan", tenant)
	if err != nil {
		t.Fatalf("[Test] Search failed: %v", err)
	}

	found := false
	for _, res := range results {
		if strings.Contains(res.Text, "vegan") {
			found = true
		}
	}
	if !found {
		t.Errorf("[Test] Expected to find 'vegan' in search results")
	}

	log.Println("[Test] Classifying text emotion on C++...")
	emotions, err := emotionService.ClassifyText("I am very happy with the service.", sessionID)
	if err != nil {
		t.Fatalf("[Test] ClassifyText failed: %v", err)
	}
	if emotions.Joy < 0.5 {
		t.Errorf("[Test] Expected joy score to be high, got: %.2f", emotions.Joy)
	}

	log.Println("[Test] Destroying RAG session on C++...")
	err = ragProvider.Destroy(sessionID)
	if err != nil {
		t.Fatalf("[Test] Destroy failed: %v", err)
	}
	log.Println("[Test] All local C++ ML integration endpoints passed successfully!")
}

func TestGoCloudProviders(t *testing.T) {
	// Set Go env to route cloud requests inside Go (OpenAI Client)
	t.Setenv("RAG_PROVIDER", "cloud")
	t.Setenv("TEXT_EMOTION_PROVIDER", "cloud")
	t.Setenv("AUDIO_EMOTION_PROVIDER", "cloud")

	log.Println("[Test] Testing Go-side Cloud providers (RAG and CIE)...")

	// 1. Create RAG provider from factory (should return Go CloudRAG)
	ragProvider, err := rag.CreateRAGProvider()
	if err != nil {
		t.Fatalf("[Test] Failed to create CloudRAG provider: %v", err)
	}

	emotionService, err := services.NewEmotionService()
	if err != nil {
		t.Fatalf("[Test] Failed to create EmotionService: %v", err)
	}
	defer emotionService.Close()

	// 2. Test Cloud RAG Lifecycle (uses mock embeddings if OPENAI_API_KEY is not configured)
	sessionID := "cloud_test_session"
	company := "PizzaHut"
	tenant := "00000000-0000-0000-0000-000000000001"
	phone := "+18889999"

	log.Println("[Test] Initializing Cloud RAG in Go...")
	err = ragProvider.Initialize(sessionID, company, tenant, phone)
	if err != nil {
		t.Fatalf("[Test] Cloud Initialize failed: %v", err)
	}

	log.Println("[Test] Indexing dynamic preference in Cloud RAG...")
	err = ragProvider.IndexDynamic(sessionID, "Customer requested gluten-free pizza options.", "chat_transcript", tenant)
	if err != nil {
		t.Fatalf("[Test] Cloud IndexDynamic failed: %v", err)
	}

	log.Println("[Test] Searching Cloud RAG...")
	results, err := ragProvider.Search(sessionID, "gluten", tenant)
	if err != nil {
		t.Fatalf("[Test] Cloud Search failed: %v", err)
	}

	found := false
	for _, res := range results {
		if strings.Contains(strings.ToLower(res.Text), "gluten") {
			found = true
		}
	}
	if !found {
		t.Errorf("[Test] Expected to find 'gluten' in Cloud RAG search results")
	}

	// 3. Test Cloud Emotion Service (calls OpenAI if key present, else fallbacks gracefully)
	log.Println("[Test] Classifying text emotion in Go Cloud provider...")
	emotions, err := emotionService.ClassifyText("This is a test transcript.", sessionID)
	if err != nil {
		t.Fatalf("[Test] Cloud ClassifyText failed: %v", err)
	}
	if emotions.Neutral == 0 {
		t.Errorf("[Test] Expected neutral score to be populated, got 0")
	}

	log.Println("[Test] Destroying Cloud RAG...")
	err = ragProvider.Destroy(sessionID)
	if err != nil {
		t.Fatalf("[Test] Cloud Destroy failed: %v", err)
	}

	log.Println("[Test] Go-side Cloud providers tests completed successfully!")
}

