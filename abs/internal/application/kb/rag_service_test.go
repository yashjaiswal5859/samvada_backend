// =============================================================================
// Samvada Platform — ABS Knowledge Base Service Unit Tests
// =============================================================================

package kb_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	appKB "github.com/samvada/abs/internal/application/kb"
	"github.com/samvada/abs/internal/domain"
	"github.com/samvada/abs/internal/infrastructure/storage"
)

type mockKBRepo struct {
	mu        sync.Mutex
	kbs       map[uuid.UUID]*domain.KnowledgeBase
	documents map[uuid.UUID]*domain.KBDocument
}

func newMockKBRepo() *mockKBRepo {
	return &mockKBRepo{
		kbs:       make(map[uuid.UUID]*domain.KnowledgeBase),
		documents: make(map[uuid.UUID]*domain.KBDocument),
	}
}

func (m *mockKBRepo) CreateKB(ctx context.Context, kb *domain.KnowledgeBase) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.kbs[kb.ID] = kb
	return nil
}

func (m *mockKBRepo) GetKBByID(ctx context.Context, workspaceID, id uuid.UUID) (*domain.KnowledgeBase, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	kb, exists := m.kbs[id]
	if !exists || kb.WorkspaceID != workspaceID {
		return nil, fmt.Errorf("not found")
	}
	return kb, nil
}

func (m *mockKBRepo) ListKBByWorkspace(ctx context.Context, workspaceID uuid.UUID) ([]*domain.KnowledgeBase, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []*domain.KnowledgeBase
	for _, kb := range m.kbs {
		if kb.WorkspaceID == workspaceID {
			list = append(list, kb)
		}
	}
	return list, nil
}

func (m *mockKBRepo) UpdateKB(ctx context.Context, kb *domain.KnowledgeBase) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.kbs[kb.ID] = kb
	return nil
}

func (m *mockKBRepo) DeleteKB(ctx context.Context, workspaceID, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.kbs, id)
	return nil
}

func (m *mockKBRepo) CreateDocument(ctx context.Context, doc *domain.KBDocument) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.documents[doc.ID] = doc
	return nil
}

func (m *mockKBRepo) GetDocumentByID(ctx context.Context, id uuid.UUID) (*domain.KBDocument, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	doc, exists := m.documents[id]
	if !exists {
		return nil, fmt.Errorf("not found")
	}
	return doc, nil
}

func (m *mockKBRepo) ListDocuments(ctx context.Context, kbID uuid.UUID) ([]*domain.KBDocument, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []*domain.KBDocument
	for _, doc := range m.documents {
		if doc.KBID == kbID {
			list = append(list, doc)
		}
	}
	return list, nil
}

func (m *mockKBRepo) UpdateDocumentStatus(ctx context.Context, id uuid.UUID, status string, chunkCount int, errMsg *string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if doc, exists := m.documents[id]; exists {
		doc.Status = status
		doc.ChunkCount = chunkCount
		doc.ErrorMessage = errMsg
	}
	return nil
}

func (m *mockKBRepo) DeleteDocument(ctx context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.documents, id)
	return nil
}

func (m *mockKBRepo) SearchChunks(ctx context.Context, workspaceID uuid.UUID, kbIDs []uuid.UUID, embeddingStr string, limit int) ([]*domain.KBSearchResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(kbIDs) == 0 {
		return []*domain.KBSearchResult{}, nil
	}
	return []*domain.KBSearchResult{
		{
			ChunkID:    uuid.New(),
			KBID:       kbIDs[0],
			DocumentID: uuid.New(),
			Content:    "Sample matched chunk text",
			Similarity: 0.89,
		},
	}, nil
}


func TestKBService_PresignAndCompleteUpload(t *testing.T) {
	logger := zap.NewNop()
	repo := newMockKBRepo()
	tempDir := t.TempDir()
	store, err := storage.NewLocalStorage(tempDir, "http://localhost:8082", logger)
	require.NoError(t, err)

	svc := appKB.NewKBService(repo, store, nil, logger)

	ctx := context.Background()
	wsID := uuid.New()

	// 1. Create a Knowledge Base
	desc := "Clinic manual"
	kb, err := svc.CreateKB(ctx, wsID, "Dental Clinic", &desc)
	require.NoError(t, err)
	require.NotNil(t, kb)

	// 2. Reject Non-PDF uploads (Security)
	_, err = svc.PresignDocumentUpload(ctx, wsID, kb.ID, "malware.exe")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "only PDF documents")

	_, err = svc.PresignDocumentUpload(ctx, wsID, kb.ID, "notes.docx")
	assert.Error(t, err)

	// 3. Successfully generate Presigned Upload URL for PDF
	presignRes, err := svc.PresignDocumentUpload(ctx, wsID, kb.ID, "clinic_policy.pdf")
	require.NoError(t, err)
	require.NotNil(t, presignRes)
	assert.NotEqual(t, uuid.Nil, presignRes.DocumentID)
	assert.True(t, strings.HasPrefix(presignRes.S3Key, fmt.Sprintf("workspaces/%s/kb/%s/", wsID.String(), kb.ID.String())))
	assert.True(t, strings.HasSuffix(presignRes.S3Key, ".pdf"))
	assert.NotEmpty(t, presignRes.UploadURL)

	// 4. Attempt to complete before file is uploaded (Anti-Spoofing check)
	_, err = svc.CompleteDocumentUpload(ctx, wsID, kb.ID, presignRes.DocumentID, "clinic_policy.pdf", presignRes.S3Key)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "object does not exist in storage")

	// 5. Simulate client direct PUT upload to storage
	dummyPDF := []byte("%PDF-1.4 mock pdf content for testing")
	_, err = store.Upload(ctx, presignRes.S3Key, dummyPDF, "application/pdf")
	require.NoError(t, err)

	// 6. Complete upload now succeeds
	doc, err := svc.CompleteDocumentUpload(ctx, wsID, kb.ID, presignRes.DocumentID, "clinic_policy.pdf", presignRes.S3Key)
	require.NoError(t, err)
	require.NotNil(t, doc)
	assert.Equal(t, presignRes.DocumentID, doc.ID)
	assert.Equal(t, "indexing", doc.Status)
	assert.Equal(t, int64(len(dummyPDF)), *doc.SizeBytes)
}

func TestKBService_DynamicNotesSwap(t *testing.T) {
	logger := zap.NewNop()
	repo := newMockKBRepo()
	tempDir := t.TempDir()
	store, err := storage.NewLocalStorage(tempDir, "http://localhost:8082", logger)
	require.NoError(t, err)

	svc := appKB.NewKBService(repo, store, nil, logger)

	ctx := context.Background()
	wsID := uuid.New()
	kb, err := svc.CreateKB(ctx, wsID, "Daily Specials", nil)
	require.NoError(t, err)

	// 1. Initial note save
	note1, err := svc.SaveNote(ctx, wsID, kb.ID, "Dr. Dave is out until Friday.")
	require.NoError(t, err)
	require.NotNil(t, note1)
	assert.Equal(t, "Quick Notes", note1.Name)
	assert.Equal(t, "Dr. Dave is out until Friday.", note1.SourceURLOrPath)

	// 2. Fetch note
	fetched, err := svc.GetNote(ctx, wsID, kb.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.Equal(t, note1.ID, fetched.ID)

	// 3. Update note (Simulate in-place swap -> ensures old note is deleted, new note is created)
	note2, err := svc.SaveNote(ctx, wsID, kb.ID, "Dr. Dave is back. Summer sale 20% off.")
	require.NoError(t, err)
	require.NotNil(t, note2)
	assert.NotEqual(t, note1.ID, note2.ID) // Fresh ID created, old ID deleted
	assert.Equal(t, "Dr. Dave is back. Summer sale 20% off.", note2.SourceURLOrPath)

	// 4. Verify only 1 note document exists in repository
	docs, err := svc.ListDocuments(ctx, kb.ID)
	require.NoError(t, err)
	assert.Len(t, docs, 1)
	assert.Equal(t, note2.ID, docs[0].ID)

	// 5. Clear note by saving empty string
	cleared, err := svc.SaveNote(ctx, wsID, kb.ID, "")
	require.NoError(t, err)
	assert.Nil(t, cleared)

	docsAfterClear, err := svc.ListDocuments(ctx, kb.ID)
	require.NoError(t, err)
	assert.Empty(t, docsAfterClear)
}

func TestKBService_SearchKnowledge(t *testing.T) {
	logger := zap.NewNop()
	repo := newMockKBRepo()
	tempDir := t.TempDir()
	store, err := storage.NewLocalStorage(tempDir, "http://localhost:8082", logger)
	require.NoError(t, err)

	svc := appKB.NewKBService(repo, store, nil, logger)

	ctx := context.Background()
	wsID := uuid.New()
	kb, err := svc.CreateKB(ctx, wsID, "Support Docs", nil)
	require.NoError(t, err)

	// 1. Empty query returns empty results
	emptyRes, err := svc.SearchKnowledge(ctx, wsID, []uuid.UUID{kb.ID}, "   ", 5)
	require.NoError(t, err)
	assert.Empty(t, emptyRes)

	// 2. Empty KB list returns empty results
	noKBsRes, err := svc.SearchKnowledge(ctx, wsID, []uuid.UUID{}, "how to reset password", 5)
	require.NoError(t, err)
	assert.Empty(t, noKBsRes)

	// 3. Valid search query returns ranked chunks
	results, err := svc.SearchKnowledge(ctx, wsID, []uuid.UUID{kb.ID}, "how to reset password", 3)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, kb.ID, results[0].KBID)
	assert.Equal(t, "Sample matched chunk text", results[0].Content)
	assert.Equal(t, 0.89, results[0].Similarity)
}

