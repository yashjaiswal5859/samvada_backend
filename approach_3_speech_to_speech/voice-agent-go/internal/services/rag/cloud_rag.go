package rag

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"voice-agent-go/internal/config"
	"voice-agent-go/internal/db"
	"voice-agent-go/internal/models"

	"github.com/google/uuid"
)

type RAGDocument struct {
	Text      string
	Source    string
	Embedding []float32
}

type CloudRAG struct {
	client         *http.Client
	sessionIndices map[string][]RAGDocument
	mu             sync.Mutex
}

func NewCloudRAG() *CloudRAG {
	return &CloudRAG{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		sessionIndices: make(map[string][]RAGDocument),
	}
}

type openaiEmbeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

func (c *CloudRAG) computeEmbedding(text string) ([]float32, error) {
	apiKey := config.Settings.OpenaiApiKey
	if apiKey == "" {
		// Mock local embedding generation if API Key is not configured for dev
		log.Printf("[Cloud RAG] Warning: OPENAI_API_KEY empty. Generating mock embedding for: %q", text)
		embedding := make([]float32, 384)
		// Deterministic hash-based values
		var sum float64
		for i, char := range text {
			idx := i % 384
			val := float32(math.Sin(float64(char) + float64(i)))
			embedding[idx] += val
		}
		for i := range embedding {
			sum += float64(embedding[i] * embedding[i])
		}
		norm := math.Sqrt(sum)
		if norm > 0 {
			for i := range embedding {
				embedding[i] = float32(float64(embedding[i]) / norm)
			}
		}
		return embedding, nil
	}

	payload := map[string]interface{}{
		"input": text,
		"model": "text-embedding-3-small",
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/embeddings", bytes.NewBuffer(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OpenAI API returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var embedResp openaiEmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&embedResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(embedResp.Data) == 0 {
		return nil, fmt.Errorf("OpenAI returned empty embedding array")
	}

		// Normalize L2
	embedding := embedResp.Data[0].Embedding
	var sum float32
	for _, val := range embedding {
		sum += val * val
	}
	norm := float32(math.Sqrt(float64(sum)))
	if norm > 0 {
		for i := range embedding {
			embedding[i] /= norm
		}
	}

	return embedding, nil
}

func (c *CloudRAG) dotProduct(a, b []float32) float64 {
	var dot float64
	for i := 0; i < len(a) && i < len(b); i++ {
		dot += float64(a[i] * b[i])
	}
	return dot
}

func (c *CloudRAG) Initialize(sessionID string, companyName string, tenantID string, phone string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	log.Printf("[Cloud RAG] Initializing index for session: %s", sessionID)

	var docs []RAGDocument

	// 1. Resolve active tenant docs directory from TENANT_DOCS_PATH or fallback
	docsDir := config.Settings.TenantDocsPath
	if docsDir == "" || func() bool { _, err := os.Stat(docsDir); return os.IsNotExist(err) }() {
		docsDir = filepath.Join(config.Settings.DocsBasePath, companyName)
	}

	// Check for a dedicated /context subfolder (clean plug-and-play architecture)
	contextDir := filepath.Join(docsDir, "context")
	scanDir := docsDir
	if _, err := os.Stat(contextDir); err == nil {
		scanDir = contextDir
		log.Printf("[Cloud RAG] Plug & Play Context Folder Detected: %s", scanDir)
	}

	entries, err := os.ReadDir(scanDir)
	if err != nil {
		log.Printf("[Cloud RAG] Warning: could not read docs dir %s: %v. Starting with empty index.", scanDir, err)
	} else {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			// Skip non-knowledge system files
			if name == "system_prompt.txt" || name == "greeting.json" || name == "greeting.txt" || name == "greeting_anonymous.txt" || strings.HasSuffix(name, ".csv") {
				continue
			}
			if !strings.HasSuffix(name, ".txt") && !strings.HasSuffix(name, ".md") {
				continue
			}
			content, err := os.ReadFile(filepath.Join(scanDir, name))
			if err != nil {
				log.Printf("[Cloud RAG] Warning: could not read file %s: %v", name, err)
				continue
			}
			emb, err := c.computeEmbedding(string(content))
			if err != nil {
				log.Printf("[Cloud RAG] Warning: embedding failed for %s: %v", name, err)
				continue
			}
			docs = append(docs, RAGDocument{Text: string(content), Source: name, Embedding: emb})
			log.Printf("[Cloud RAG] Indexed file: %s (%d chars)", name, len(content))
		}
	}

	// 2. Query historical preferences from PostgreSQL via GORM
	if phone != "" && db.DB != nil {
		tenantUUID, err := uuid.Parse(tenantID)
		if err == nil {
			var memories []models.CustomerMemory
			if err := db.DB.Where("phone = ? AND tenant_id = ?", phone, tenantUUID).Order("created_at desc").Limit(5).Find(&memories).Error; err == nil {
				for _, mem := range memories {
					emb, err := c.computeEmbedding(mem.MemoryText)
					if err == nil {
						docs = append(docs, RAGDocument{
							Text:      mem.MemoryText,
							Source:    "postgres_customer_memories",
							Embedding: emb,
						})
					}
				}
				log.Printf("[Cloud RAG] Loaded %d historical memories from Postgres for phone: %s", len(memories), phone)
			}
		}
	}

	c.sessionIndices[sessionID] = docs
	return nil
}

func (c *CloudRAG) Search(sessionID string, query string, tenantID string) ([]RAGResult, error) {
	c.mu.Lock()
	docs, exists := c.sessionIndices[sessionID]
	c.mu.Unlock()

	if !exists {
		return nil, fmt.Errorf("session RAG index not initialized: %s", sessionID)
	}

	queryEmb, err := c.computeEmbedding(query)
	if err != nil {
		return nil, fmt.Errorf("failed to compute query embedding: %w", err)
	}

	var results []RAGResult
	for _, doc := range docs {
		score := c.dotProduct(queryEmb, doc.Embedding)
		results = append(results, RAGResult{
			Text:   doc.Text,
			Score:  score,
			Source: doc.Source,
		})
	}

	// Sort by similarity descending
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[i].Score < results[j].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	// Limit to top 3
	limit := 3
	if len(results) < limit {
		limit = len(results)
	}

	return results[:limit], nil
}

func (c *CloudRAG) IndexDynamic(sessionID string, text string, source string, tenantID string) error {
	c.mu.Lock()
	docs, exists := c.sessionIndices[sessionID]
	if !exists {
		c.mu.Unlock()
		return fmt.Errorf("session RAG index not initialized: %s", sessionID)
	}

	emb, err := c.computeEmbedding(text)
	if err != nil {
		c.mu.Unlock()
		return fmt.Errorf("failed to compute embedding: %w", err)
	}

	c.sessionIndices[sessionID] = append(docs, RAGDocument{
		Text:      text,
		Source:    source,
		Embedding: emb,
	})
	c.mu.Unlock()

	log.Printf("[Cloud RAG] Dynamically indexed memory in session %s: %s", sessionID, text)
	return nil
}

func (c *CloudRAG) Persist(sessionID string, phone string, tenantID string, memories []string, sourceCallID string) error {
	if db.DB == nil {
		log.Printf("[Cloud RAG] Warning: Postgres DB not initialized. Skipping database write of memories.")
		return nil
	}

	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		return fmt.Errorf("invalid tenant ID: %w", err)
	}

	callUUID, err := uuid.Parse(sourceCallID)
	if err != nil {
		return fmt.Errorf("invalid call ID: %w", err)
	}

	for _, memory := range memories {
		memRecord := &models.CustomerMemory{
			TenantID:     tenantUUID,
			Phone:        phone,
			MemoryText:   memory,
			SourceCallID: callUUID,
		}
		if err := db.DB.Create(memRecord).Error; err != nil {
			return fmt.Errorf("failed to save memory to postgres: %w", err)
		}
	}

	log.Printf("[Cloud RAG] Successfully persisted %d memories to database for call: %s", len(memories), sourceCallID)
	return nil
}

func (c *CloudRAG) Destroy(sessionID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.sessionIndices, sessionID)
	log.Printf("[Cloud RAG] Destroyed in-memory index for session: %s", sessionID)
	return nil
}
