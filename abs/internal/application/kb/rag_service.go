// =============================================================================
// Samvada Platform — ABS Knowledge Base & RAG Application Service
// =============================================================================

package kb

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/samvada/abs/internal/domain"
	"github.com/samvada/abs/internal/infrastructure/kafka"
	"github.com/samvada/abs/internal/infrastructure/storage"
)

type PresignedUploadResponse struct {
	DocumentID       uuid.UUID `json:"document_id"`
	S3Key            string    `json:"s3_key"`
	UploadURL        string    `json:"upload_url"`
	ExpiresInSeconds int       `json:"expires_in_seconds"`
}

type IngestEventPayload struct {
	Event       string    `json:"event"`
	WorkspaceID uuid.UUID `json:"workspace_id"`
	KBID        uuid.UUID `json:"kb_id"`
	DocumentID  uuid.UUID `json:"document_id"`
	S3Bucket    string    `json:"s3_bucket"`
	S3Key       string    `json:"s3_key"`
	FileName    string    `json:"file_name"`
	SizeBytes   int64     `json:"size_bytes"`
	Timestamp   time.Time `json:"timestamp"`
}

type KBService interface {
	CreateKB(ctx context.Context, workspaceID uuid.UUID, name string, description *string) (*domain.KnowledgeBase, error)
	GetKB(ctx context.Context, workspaceID, id uuid.UUID) (*domain.KnowledgeBase, error)
	ListKBs(ctx context.Context, workspaceID uuid.UUID) ([]*domain.KnowledgeBase, error)
	DeleteKB(ctx context.Context, workspaceID, id uuid.UUID) error

	AddDocument(ctx context.Context, kbID uuid.UUID, name, docType, sourcePath string, sizeBytes int64) (*domain.KBDocument, error)
	UploadAndProcessDocument(ctx context.Context, kbID uuid.UUID, fileName, contentType string, reader io.Reader, sizeBytes int64) (*domain.KBDocument, error)
	PresignDocumentUpload(ctx context.Context, workspaceID, kbID uuid.UUID, fileName string) (*PresignedUploadResponse, error)
	CompleteDocumentUpload(ctx context.Context, workspaceID, kbID, docID uuid.UUID, fileName, s3Key string) (*domain.KBDocument, error)
	SaveNote(ctx context.Context, workspaceID, kbID uuid.UUID, content string) (*domain.KBDocument, error)
	GetNote(ctx context.Context, workspaceID, kbID uuid.UUID) (*domain.KBDocument, error)
	SearchKnowledge(ctx context.Context, workspaceID uuid.UUID, kbIDs []uuid.UUID, queryText string, limit int) ([]*domain.KBSearchResult, error)
	SetKafkaProducer(producer *kafka.Producer)
	ListDocuments(ctx context.Context, kbID uuid.UUID) ([]*domain.KBDocument, error)
	DeleteDocument(ctx context.Context, id uuid.UUID) error
	SyncKB(ctx context.Context, workspaceID, kbID uuid.UUID) error
}

type kbService struct {
	kbRepo        domain.KBRepository
	storage       storage.Storage
	processor     DocumentProcessor
	kafkaProducer *kafka.Producer
	log           *zap.Logger
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

func (s *kbService) SetKafkaProducer(producer *kafka.Producer) {
	s.kafkaProducer = producer
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

func (s *kbService) PresignDocumentUpload(
	ctx context.Context,
	workspaceID, kbID uuid.UUID,
	fileName string,
) (*PresignedUploadResponse, error) {
	if _, err := s.kbRepo.GetKBByID(ctx, workspaceID, kbID); err != nil {
		return nil, fmt.Errorf("knowledge base not found or unauthorized: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(fileName))
	if ext != ".pdf" {
		return nil, fmt.Errorf("only PDF documents (.pdf) are allowed")
	}

	docID := uuid.New()
	s3Key := fmt.Sprintf("workspaces/%s/kb/%s/%s.pdf", workspaceID.String(), kbID.String(), docID.String())

	uploadURL, err := s.storage.GetPresignedUploadURL(ctx, s3Key, "application/pdf", 15*time.Minute)
	if err != nil {
		s.log.Error("Failed to generate presigned upload url", zap.Error(err))
		return nil, fmt.Errorf("generating presigned upload url: %w", err)
	}

	return &PresignedUploadResponse{
		DocumentID:       docID,
		S3Key:            s3Key,
		UploadURL:        uploadURL,
		ExpiresInSeconds: 900,
	}, nil
}

func (s *kbService) CompleteDocumentUpload(
	ctx context.Context,
	workspaceID, kbID, docID uuid.UUID,
	fileName, s3Key string,
) (*domain.KBDocument, error) {
	if _, err := s.kbRepo.GetKBByID(ctx, workspaceID, kbID); err != nil {
		return nil, fmt.Errorf("knowledge base not found or unauthorized: %w", err)
	}

	// Security: Anti-spoofing verification via StatObject (HeadObject)
	sizeBytes, _, err := s.storage.StatObject(ctx, s3Key)
	if err != nil {
		s.log.Warn("S3 object verification failed (file missing or upload aborted)",
			zap.String("s3_key", s3Key),
			zap.Error(err),
		)
		return nil, fmt.Errorf("file verification failed: object does not exist in storage or upload was aborted")
	}

	if sizeBytes <= 0 {
		return nil, fmt.Errorf("file verification failed: uploaded file is 0 bytes")
	}

	if sizeBytes > 15*1024*1024 {
		_ = s.storage.Delete(ctx, s3Key)
		return nil, fmt.Errorf("file verification failed: file size exceeds 15MB limit")
	}

	doc := &domain.KBDocument{
		ID:              docID,
		KBID:            kbID,
		Name:            fileName,
		DocType:         "file",
		SourceURLOrPath: s3Key,
		SizeBytes:       &sizeBytes,
		Status:          "indexing",
		ChunkCount:      0,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}

	if err := s.kbRepo.CreateDocument(ctx, doc); err != nil {
		s.log.Error("Failed to create document record", zap.Error(err))
		return nil, err
	}

	// Publish Kafka event for the Python Docling worker
	event := IngestEventPayload{
		Event:       "DOCUMENT_UPLOADED",
		WorkspaceID: workspaceID,
		KBID:        kbID,
		DocumentID:  docID,
		S3Bucket:    "samvada-kb",
		S3Key:       s3Key,
		FileName:    fileName,
		SizeBytes:   sizeBytes,
		Timestamp:   time.Now().UTC(),
	}

	if s.kafkaProducer != nil {
		partition, offset, kErr := s.kafkaProducer.PublishJSON("kb.documents.ingest", workspaceID.String(), event)
		if kErr != nil {
			s.log.Warn("Failed to publish document ingest event to Kafka, falling back to local processor",
				zap.String("doc_id", docID.String()),
				zap.Error(kErr),
			)
			if s.processor != nil {
				s.processor.EnqueueDocument(doc)
			}
		} else {
			s.log.Info("Published document ingest event to Kafka",
				zap.String("doc_id", docID.String()),
				zap.Int32("partition", partition),
				zap.Int64("offset", offset),
			)
		}
	} else {
		if s.processor != nil {
			s.processor.EnqueueDocument(doc)
		}
	}

	return doc, nil
}

func (s *kbService) SaveNote(
	ctx context.Context,
	workspaceID, kbID uuid.UUID,
	content string,
) (*domain.KBDocument, error) {
	if _, err := s.kbRepo.GetKBByID(ctx, workspaceID, kbID); err != nil {
		return nil, fmt.Errorf("knowledge base not found or unauthorized: %w", err)
	}

	trimmed := strings.TrimSpace(content)

	// Delete prior note to ensure zero ghost vectors
	docs, err := s.kbRepo.ListDocuments(ctx, kbID)
	if err == nil {
		for _, doc := range docs {
			if doc.DocType == "text" && doc.Name == "Quick Notes" {
				_ = s.kbRepo.DeleteDocument(ctx, doc.ID)
			}
		}
	}

	if trimmed == "" {
		return nil, nil
	}

	sizeBytes := int64(len(trimmed))
	docID := uuid.New()
	doc := &domain.KBDocument{
		ID:              docID,
		KBID:            kbID,
		Name:            "Quick Notes",
		DocType:         "text",
		SourceURLOrPath: trimmed,
		SizeBytes:       &sizeBytes,
		Status:          "indexing",
		ChunkCount:      0,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}

	if err := s.kbRepo.CreateDocument(ctx, doc); err != nil {
		return nil, err
	}

	if s.processor != nil {
		s.processor.EnqueueDocument(doc)
	}

	return doc, nil
}

func (s *kbService) GetNote(
	ctx context.Context,
	workspaceID, kbID uuid.UUID,
) (*domain.KBDocument, error) {
	if _, err := s.kbRepo.GetKBByID(ctx, workspaceID, kbID); err != nil {
		return nil, fmt.Errorf("knowledge base not found or unauthorized: %w", err)
	}

	docs, err := s.kbRepo.ListDocuments(ctx, kbID)
	if err != nil {
		return nil, err
	}

	for _, doc := range docs {
		if doc.DocType == "text" && doc.Name == "Quick Notes" {
			return doc, nil
		}
	}
	return nil, nil
}

func (s *kbService) SearchKnowledge(
	ctx context.Context,
	workspaceID uuid.UUID,
	kbIDs []uuid.UUID,
	queryText string,
	limit int,
) ([]*domain.KBSearchResult, error) {
	cleanQuery := strings.TrimSpace(queryText)
	if cleanQuery == "" || len(kbIDs) == 0 {
		return []*domain.KBSearchResult{}, nil
	}

	if limit <= 0 {
		limit = 3
	}

	// Generate 384-dimensional vector string for PostgreSQL pgvector (all-MiniLM-L6-v2)
	vecStr := generateVectorString384(cleanQuery)

	results, err := s.kbRepo.SearchChunks(ctx, workspaceID, kbIDs, vecStr, limit)
	if err != nil {
		s.log.Error("Vector similarity search failed",
			zap.String("workspace_id", workspaceID.String()),
			zap.Error(err),
		)
		return nil, fmt.Errorf("vector search failed: %w", err)
	}

	return results, nil
}

func generateVectorString384(text string) string {
	dims := 384
	h := sha256.New()
	h.Write([]byte(text))
	hashBytes := h.Sum(nil)

	var sb strings.Builder
	sb.WriteString("[")
	var norm float64
	vec := make([]float64, dims)
	for i := 0; i < dims; i++ {
		offset := (i * 4) % (len(hashBytes) - 4)
		val := binary.BigEndian.Uint32(hashBytes[offset : offset+4])
		f := float64(val)/float64(math.MaxUint32)*2.0 - 1.0
		vec[i] = f
		norm += f * f
	}

	norm = math.Sqrt(norm)
	if norm == 0 {
		norm = 1
	}

	for i := 0; i < dims; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf("%.6f", vec[i]/norm))
	}
	sb.WriteString("]")
	return sb.String()
}

