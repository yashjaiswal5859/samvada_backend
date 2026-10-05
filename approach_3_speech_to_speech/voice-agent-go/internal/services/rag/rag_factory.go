package rag

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"voice-agent-go/internal/config"
	"voice-agent-go/internal/db"
)

type MockRAG struct{}

func (m *MockRAG) Initialize(sessionID string, companyName string, tenantID string, phone string) error {
	log.Printf("[Mock RAG] Initialized session: %s", sessionID)
	return nil
}

func (m *MockRAG) Search(sessionID string, query string, tenantID string) ([]RAGResult, error) {
	log.Printf("[Mock RAG] Search query: %s", query)
	return []RAGResult{
		{Text: "Mock document matching " + query + " (simulated search result).", Score: 0.85, Source: "mock_kb.txt"},
	}, nil
}

func (m *MockRAG) IndexDynamic(sessionID string, text string, source string, tenantID string) error {
	log.Printf("[Mock RAG] IndexDynamic: %s", text)
	return nil
}

func (m *MockRAG) Persist(sessionID string, phone string, tenantID string, memories []string, sourceCallID string) error {
	log.Printf("[Mock RAG] Persist %d memories", len(memories))
	return nil
}

func (m *MockRAG) Destroy(sessionID string) error {
	log.Printf("[Mock RAG] Destroyed session: %s", sessionID)
	return nil
}

type DynamicRAGRouter struct {
	localRAG *HybridRAG
	cloudRAG *CloudRAG
	mockRAG  *MockRAG
	sessions map[string]string // sessionID -> resolved provider ("local" / "cloud" / "mock")
	mu       sync.RWMutex
}

func NewDynamicRAGRouter() (*DynamicRAGRouter, error) {
	local, err := NewHybridRAG()
	if err != nil {
		return nil, err
	}

	return &DynamicRAGRouter{
		localRAG: local,
		cloudRAG: NewCloudRAG(),
		mockRAG:  &MockRAG{},
		sessions: make(map[string]string),
	}, nil
}

func (r *DynamicRAGRouter) Close() error {
	if r.localRAG != nil {
		return r.localRAG.Close()
	}
	return nil
}

func (r *DynamicRAGRouter) getProvider(sessionID string) string {
	r.mu.RLock()
	p, exists := r.sessions[sessionID]
	r.mu.RUnlock()
	if exists {
		return p
	}

	// 1. Try Redis configuration mapping for the call
	provider := ""
	ctx := context.Background()
	if db.RedisClient != nil {
		configKey := fmt.Sprintf("call_config:%s", sessionID)
		if val, err := db.RedisClient.HGet(ctx, configKey, "rag_provider").Result(); err == nil && val != "" {
			provider = val
			log.Printf("[RAG Router] Resolved model from Redis call_config for SID %s: %s", sessionID, provider)
		}
	}

	// 2. Try Config / Env fallback for scripts & testing
	if provider == "" {
		provider = config.Settings.RagProvider
	}
	if provider == "" {
		provider = os.Getenv("RAG_PROVIDER")
	}

	// 3. Smart fallback: default to OpenAI cloud RAG if key is present, else local C++
	if provider == "" {
		if config.Settings.OpenaiApiKey != "" {
			provider = "openai"
		} else {
			provider = "all-minilm"
		}
	}

	providerLower := strings.ToLower(provider)
	var resolved string
	if strings.Contains(providerLower, "openai") || providerLower == "cloud" {
		resolved = "cloud"
	} else if providerLower == "mock" {
		resolved = "mock"
	} else {
		resolved = "local" // defaults to local C++ HybridRAG
	}

	r.mu.Lock()
	r.sessions[sessionID] = resolved
	r.mu.Unlock()

	log.Printf("[RAG Router] Session %s routed to provider: %s", sessionID, resolved)
	return resolved
}

func (r *DynamicRAGRouter) Initialize(sessionID string, companyName string, tenantID string, phone string) error {
	p := r.getProvider(sessionID)
	switch p {
	case "local":
		return r.localRAG.Initialize(sessionID, companyName, tenantID, phone)
	case "cloud":
		return r.cloudRAG.Initialize(sessionID, companyName, tenantID, phone)
	default:
		return r.mockRAG.Initialize(sessionID, companyName, tenantID, phone)
	}
}

func (r *DynamicRAGRouter) Search(sessionID string, query string, tenantID string) ([]RAGResult, error) {
	p := r.getProvider(sessionID)
	switch p {
	case "local":
		return r.localRAG.Search(sessionID, query, tenantID)
	case "cloud":
		return r.cloudRAG.Search(sessionID, query, tenantID)
	default:
		return r.mockRAG.Search(sessionID, query, tenantID)
	}
}

func (r *DynamicRAGRouter) IndexDynamic(sessionID string, text string, source string, tenantID string) error {
	p := r.getProvider(sessionID)
	switch p {
	case "local":
		return r.localRAG.IndexDynamic(sessionID, text, source, tenantID)
	case "cloud":
		return r.cloudRAG.IndexDynamic(sessionID, text, source, tenantID)
	default:
		return r.mockRAG.IndexDynamic(sessionID, text, source, tenantID)
	}
}

func (r *DynamicRAGRouter) Persist(sessionID string, phone string, tenantID string, memories []string, sourceCallID string) error {
	p := r.getProvider(sessionID)
	switch p {
	case "local":
		return r.localRAG.Persist(sessionID, phone, tenantID, memories, sourceCallID)
	case "cloud":
		return r.cloudRAG.Persist(sessionID, phone, tenantID, memories, sourceCallID)
	default:
		return r.mockRAG.Persist(sessionID, phone, tenantID, memories, sourceCallID)
	}
}

func (r *DynamicRAGRouter) Destroy(sessionID string) error {
	p := r.getProvider(sessionID)
	var err error
	switch p {
	case "local":
		err = r.localRAG.Destroy(sessionID)
	case "cloud":
		err = r.cloudRAG.Destroy(sessionID)
	default:
		err = r.mockRAG.Destroy(sessionID)
	}

	r.mu.Lock()
	delete(r.sessions, sessionID)
	r.mu.Unlock()

	return err
}

func CreateRAGProvider() (RAGProvider, error) {
	log.Println("[RAG Factory] Initializing Dynamic RAG Router...")
	return NewDynamicRAGRouter()
}
