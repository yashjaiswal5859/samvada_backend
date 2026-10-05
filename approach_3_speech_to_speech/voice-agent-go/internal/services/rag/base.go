package rag

type RAGResult struct {
	Text   string  `json:"text"`
	Score  float64 `json:"score"`
	Source string  `json:"source"`
}

type RAGProvider interface {
	Initialize(sessionID string, companyName string, tenantID string, phone string) error
	Search(sessionID string, query string, tenantID string) ([]RAGResult, error)
	IndexDynamic(sessionID string, text string, source string, tenantID string) error
	Persist(sessionID string, phone string, tenantID string, memories []string, sourceCallID string) error
	Destroy(sessionID string) error
}
