// =============================================================================
// Samvada Platform — ABS Knowledge Base & RAG Application Service
// =============================================================================

package kb

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/samvada/abs/internal/domain"
	"github.com/samvada/abs/internal/infrastructure/storage"
)

type KBService interface {
	CreateKB(ctx context.Context, workspaceID uuid.UUID, name string, description *string) (*domain.KnowledgeBase, error)
	GetKB(ctx context.Context, workspaceID, id uuid.UUID) (*domain.KnowledgeBase, error)
	ListKBs(ctx context.Context, workspaceID uuid.UUID) ([]*domain.KnowledgeBase, error)
	DeleteKB(ctx context.Context, workspaceID, id uuid.UUID) error

	AddDocument(ctx context.Context, kbID uuid.UUID, name, docType, sourcePath string, sizeBytes int64) (*domain.KBDocument, error)
	UploadAndProcessDocument(ctx context.Context, kbID uuid.UUID, fileName, contentType string, reader io.Reader, sizeBytes int64) (*domain.KBDocument, error)
	ListDocuments(ctx context.Context, kbID uuid.UUID) ([]*domain.KBDocument, error)
	DeleteDocument(ctx context.Context, id uuid.UUID) error
	SyncKB(ctx context.Context, workspaceID, kbID uuid.UUID) error
}

type kbService struct {
	kbRepo    domain.KBRepository
	storage   storage.Storage
	processor DocumentProcessor
	log       *zap.Logger
}

func NewKBService(
	kbRepo domain.KBRepository,
	store storage.Storage,
	processor DocumentProcessor,
	log *zap.Logger,
) KBService {
	return &kbService{
		kbRepo:    kbRepo,
		storage:   store,
		processor: processor,
		log:       log.Named("kb_service"),
	}
}

func (s *kbService) CreateKB(
	ctx context.Context,
	workspaceID uuid.UUID,
	name string,
	description *string,
) (*domain.KnowledgeBase, error) {
	kb := &domain.KnowledgeBase{
		ID:            uuid.New(),
		WorkspaceID:   workspaceID,
		Name:          name,
		Description:   description,
		DocumentCount: 0,
		ChunkCount:    0,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}

	if err := s.kbRepo.CreateKB(ctx, kb); err != nil {
		s.log.Error("Failed to create knowledge base", zap.Error(err))
		return nil, err
	}
	return kb, nil
}

func (s *kbService) GetKB(ctx context.Context, workspaceID, id uuid.UUID) (*domain.KnowledgeBase, error) {
	return s.kbRepo.GetKBByID(ctx, workspaceID, id)
}

func (s *kbService) ListKBs(ctx context.Context, workspaceID uuid.UUID) ([]*domain.KnowledgeBase, error) {
	return s.kbRepo.ListKBByWorkspace(ctx, workspaceID)
}

func (s *kbService) DeleteKB(ctx context.Context, workspaceID, id uuid.UUID) error {
	// 1. Fetch documents to cleanup any stored files
	docs, err := s.kbRepo.ListDocuments(ctx, id)
	if err == nil {
		for _, doc := range docs {
			if doc.DocType == "file" && doc.SourceURLOrPath != "" && s.storage != nil {
				_ = s.storage.Delete(ctx, doc.SourceURLOrPath)
			}
		}
	}

	return s.kbRepo.DeleteKB(ctx, workspaceID, id)
}

func (s *kbService) AddDocument(
	ctx context.Context,
	kbID uuid.UUID,
	name, docType, sourcePath string,
	sizeBytes int64,
) (*domain.KBDocument, error) {
	doc := &domain.KBDocument{
		ID:              uuid.New(),
		KBID:            kbID,
		Name:            name,
		DocType:         docType,
		SourceURLOrPath: sourcePath,
		SizeBytes:       &sizeBytes,
		Status:          "indexing",
		ChunkCount:      0,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}

	if err := s.kbRepo.CreateDocument(ctx, doc); err != nil {
		s.log.Error("Failed to add KB document", zap.Error(err))
		return nil, err
	}

	// Trigger asynchronous background indexing
	if s.processor != nil {
		s.processor.EnqueueDocument(doc)
	}

	return doc, nil
}

func (s *kbService) UploadAndProcessDocument(
	ctx context.Context,
	kbID uuid.UUID,
	fileName, contentType string,
	reader io.Reader,
	sizeBytes int64,
) (*domain.KBDocument, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read file data: %w", err)
	}

	docID := uuid.New()
	key := fmt.Sprintf("kb/%s/%s_%s", kbID.String(), docID.String()[:8], filepath.Base(fileName))

	if s.storage != nil {
		if _, err := s.storage.Upload(ctx, key, data, contentType); err != nil {
			return nil, fmt.Errorf("upload to storage failed: %w", err)
		}
	}

	dataSize := int64(len(data))
	doc := &domain.KBDocument{
		ID:              docID,
		KBID:            kbID,
		Name:            fileName,
		DocType:         "file",
		SourceURLOrPath: key,
		SizeBytes:       &dataSize,
		Status:          "indexing",
		ChunkCount:      0,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}

	if err := s.kbRepo.CreateDocument(ctx, doc); err != nil {
		s.log.Error("Failed to persist document record", zap.Error(err))
		return nil, err
	}

	if s.processor != nil {
		s.processor.EnqueueDocument(doc)
	}

	return doc, nil
}

func (s *kbService) ListDocuments(ctx context.Context, kbID uuid.UUID) ([]*domain.KBDocument, error) {
	return s.kbRepo.ListDocuments(ctx, kbID)
}

func (s *kbService) DeleteDocument(ctx context.Context, id uuid.UUID) error {
	doc, err := s.kbRepo.GetDocumentByID(ctx, id)
	if err != nil {
		return err
	}

	if doc.DocType == "file" && doc.SourceURLOrPath != "" && s.storage != nil {
		_ = s.storage.Delete(ctx, doc.SourceURLOrPath)
	}

	return s.kbRepo.DeleteDocument(ctx, id)
}

func (s *kbService) SyncKB(ctx context.Context, workspaceID, kbID uuid.UUID) error {
	_, err := s.kbRepo.GetKBByID(ctx, workspaceID, kbID)
	if err != nil {
		return err
	}

	docs, err := s.kbRepo.ListDocuments(ctx, kbID)
	if err != nil {
		return err
	}

	for _, doc := range docs {
		_ = s.kbRepo.UpdateDocumentStatus(ctx, doc.ID, "indexing", 0, nil)
		if s.processor != nil {
			s.processor.EnqueueDocument(doc)
		}
	}

	s.log.Info("Triggered full KB re-indexing sync",
		zap.String("kb_id", kbID.String()),
		zap.Int("doc_count", len(docs)),
	)
	return nil
}
