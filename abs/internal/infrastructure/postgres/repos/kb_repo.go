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
		First(&kb, "id = ? AND workspace_id = ?", id, workspaceID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrKBNotFound
		}
		return nil, err
	}
	var docCount int64
	_ = r.db.WithContext(ctx).
		Model(&domain.KBDocument{}).
		Where("kb_id = ?", kb.ID).
		Count(&docCount).Error
	kb.DocumentCount = int(docCount)
	kb.Documents = nil
	kb.PopulateFolderAliases()
	return &kb, nil
}

func (r *kbRepo) ListKBByWorkspace(ctx context.Context, workspaceID uuid.UUID) ([]*domain.KnowledgeBase, error) {
	var kbs []*domain.KnowledgeBase
	err := r.db.WithContext(ctx).
		Where("workspace_id = ?", workspaceID).
		Order("created_at DESC").
		Find(&kbs).Error
	if err != nil {
		return nil, err
	}

	if len(kbs) == 0 {
		return kbs, nil
	}

	kbIDs := make([]uuid.UUID, len(kbs))
	for i, kb := range kbs {
		kbIDs[i] = kb.ID
	}

	type docCountResult struct {
		KBID  uuid.UUID `gorm:"column:kb_id"`
		Count int       `gorm:"column:count"`
	}
	var counts []docCountResult
	_ = r.db.WithContext(ctx).
		Table("kb_documents").
		Select("kb_id, count(*) as count").
		Where("kb_id IN ?", kbIDs).
		Group("kb_id").
		Scan(&counts).Error

	countMap := make(map[uuid.UUID]int, len(counts))
	for _, c := range counts {
		countMap[c.KBID] = c.Count
	}

	for _, kb := range kbs {
		if c, ok := countMap[kb.ID]; ok {
			kb.DocumentCount = c
		} else {
			kb.DocumentCount = 0
		}
		kb.Documents = nil
		kb.PopulateFolderAliases()
	}

	return kbs, nil
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

func (r *kbRepo) SearchChunks(
	ctx context.Context,
	workspaceID uuid.UUID,
	kbIDs []uuid.UUID,
	embeddingStr string,
	limit int,
) ([]*domain.KBSearchResult, error) {
	if limit <= 0 {
		limit = 5
	}
	if len(kbIDs) == 0 {
		return []*domain.KBSearchResult{}, nil
	}

	query := `
		SELECT 
			id AS chunk_id,
			kb_id,
			document_id,
			content,
			1 - (embedding <=> ?::vector) AS similarity
		FROM kb_chunks
		WHERE workspace_id = ?
		  AND kb_id IN (?)
		ORDER BY embedding <=> ?::vector ASC
		LIMIT ?;
	`

	rows, err := r.db.WithContext(ctx).Raw(query, embeddingStr, workspaceID, kbIDs, embeddingStr, limit).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*domain.KBSearchResult
	for rows.Next() {
		var res domain.KBSearchResult
		if err := rows.Scan(&res.ChunkID, &res.KBID, &res.DocumentID, &res.Content, &res.Similarity); err != nil {
			return nil, err
		}
		results = append(results, &res)
	}

	return results, nil
}

