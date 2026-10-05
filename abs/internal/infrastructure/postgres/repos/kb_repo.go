// =============================================================================
// Samvada Platform — ABS Knowledge Base Repository (GORM PostgreSQL)
// =============================================================================

package repos

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/samvada/abs/internal/domain"
)

type kbRepo struct {
	db *gorm.DB
}

func NewKBRepository(db *gorm.DB) domain.KBRepository {
	return &kbRepo{db: db}
}

func (r *kbRepo) CreateKB(ctx context.Context, kb *domain.KnowledgeBase) error {
	if kb.ID == uuid.Nil {
		kb.ID = uuid.New()
	}
	return r.db.WithContext(ctx).Create(kb).Error
}

func (r *kbRepo) GetKBByID(ctx context.Context, workspaceID, id uuid.UUID) (*domain.KnowledgeBase, error) {
	var kb domain.KnowledgeBase
	if err := r.db.WithContext(ctx).
		Preload("Documents").
		First(&kb, "id = ? AND workspace_id = ?", id, workspaceID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrKBNotFound
		}
		return nil, err
	}
	return &kb, nil
}

func (r *kbRepo) ListKBByWorkspace(ctx context.Context, workspaceID uuid.UUID) ([]*domain.KnowledgeBase, error) {
	var kbs []*domain.KnowledgeBase
	err := r.db.WithContext(ctx).
		Where("workspace_id = ?", workspaceID).
		Preload("Documents").
		Order("created_at DESC").
		Find(&kbs).Error
	return kbs, err
}

func (r *kbRepo) UpdateKB(ctx context.Context, kb *domain.KnowledgeBase) error {
	return r.db.WithContext(ctx).Save(kb).Error
}

func (r *kbRepo) DeleteKB(ctx context.Context, workspaceID, id uuid.UUID) error {
	res := r.db.WithContext(ctx).
		Delete(&domain.KnowledgeBase{}, "id = ? AND workspace_id = ?", id, workspaceID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrKBNotFound
	}
	return nil
}

func (r *kbRepo) CreateDocument(ctx context.Context, doc *domain.KBDocument) error {
	if doc.ID == uuid.Nil {
		doc.ID = uuid.New()
	}
	return r.db.WithContext(ctx).Create(doc).Error
}

func (r *kbRepo) GetDocumentByID(ctx context.Context, id uuid.UUID) (*domain.KBDocument, error) {
	var doc domain.KBDocument
	if err := r.db.WithContext(ctx).First(&doc, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrKBDocNotFound
		}
		return nil, err
	}
	return &doc, nil
}

func (r *kbRepo) ListDocuments(ctx context.Context, kbID uuid.UUID) ([]*domain.KBDocument, error) {
	var docs []*domain.KBDocument
	err := r.db.WithContext(ctx).
		Where("kb_id = ?", kbID).
		Order("created_at DESC").
		Find(&docs).Error
	return docs, err
}

func (r *kbRepo) UpdateDocumentStatus(
	ctx context.Context,
	id uuid.UUID,
	status string,
	chunkCount int,
	errMsg *string,
) error {
	updates := map[string]interface{}{
		"status":        status,
		"chunk_count":   chunkCount,
		"error_message": errMsg,
	}
	res := r.db.WithContext(ctx).
		Model(&domain.KBDocument{}).
		Where("id = ?", id).
		Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrKBDocNotFound
	}
	return nil
}

func (r *kbRepo) DeleteDocument(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Delete(&domain.KBDocument{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrKBDocNotFound
	}
	return nil
}
