// =============================================================================
// Samvada Platform — ABS Knowledge Base Domain Entity & Repository
// =============================================================================

package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type KnowledgeBase struct {
	ID            uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	FolderID      *uuid.UUID `gorm:"-" json:"folder_id,omitempty"`
	WorkspaceID   uuid.UUID  `gorm:"type:uuid;not null;index:idx_knowledge_bases_workspace" json:"workspace_id"`
	Name          string     `gorm:"type:varchar(255);not null" json:"name"`
	FolderName    string     `gorm:"-" json:"folder_name,omitempty"`
	Description   *string    `gorm:"type:text" json:"description,omitempty"`
	DocumentCount int        `gorm:"not null;default:0" json:"document_count"`
	DocCount      *int       `gorm:"-" json:"doc_count,omitempty"`
	ChunkCount    int        `gorm:"not null;default:0" json:"chunk_count"`
	CreatedAt     time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt     time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`

	// Associations
	Documents []KBDocument `gorm:"foreignKey:KBID;constraint:OnDelete:CASCADE" json:"documents,omitempty"`
}

func (kb *KnowledgeBase) PopulateFolderAliases() *KnowledgeBase {
	if kb == nil {
		return nil
	}
	kb.FolderID = &kb.ID
	kb.FolderName = kb.Name
	docCount := kb.DocumentCount
	kb.DocCount = &docCount
	return kb
}

func (KnowledgeBase) TableName() string {
	return "knowledge_bases"
}

type KBDocument struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	KBID            uuid.UUID `gorm:"column:kb_id;type:uuid;not null;index:idx_kb_documents_kb_id" json:"kb_id"`
	Name            string    `gorm:"type:varchar(255);not null" json:"name"`
	DocType         string    `gorm:"type:varchar(50);not null" json:"doc_type"` // file, url, text
	SourceURLOrPath string    `gorm:"column:source_url_or_path;type:text;not null" json:"source_url_or_path"`
	SizeBytes       *int64    `gorm:"type:bigint" json:"size_bytes,omitempty"`
	Status          string    `gorm:"type:varchar(50);default:'indexing'" json:"status"` // indexing, indexed, failed
	ErrorMessage    *string   `gorm:"type:text" json:"error_message,omitempty"`
	ChunkCount      int       `gorm:"not null;default:0" json:"chunk_count"`
	CreatedAt       time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt       time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`

	KnowledgeBase *KnowledgeBase `gorm:"foreignKey:KBID" json:"knowledge_base,omitempty"`
}

func (KBDocument) TableName() string {
	return "kb_documents"
}

type KBSearchResult struct {
	ChunkID    uuid.UUID `json:"chunk_id"`
	KBID       uuid.UUID `json:"kb_id"`
	DocumentID uuid.UUID `json:"document_id"`
	Content    string    `json:"content"`
	Similarity float64   `json:"similarity"`
}

// KBRepository defines data access methods for Knowledge Base aggregates
type KBRepository interface {
	CreateKB(ctx context.Context, kb *KnowledgeBase) error
	GetKBByID(ctx context.Context, workspaceID, id uuid.UUID) (*KnowledgeBase, error)
	ListKBByWorkspace(ctx context.Context, workspaceID uuid.UUID) ([]*KnowledgeBase, error)
	UpdateKB(ctx context.Context, kb *KnowledgeBase) error
	DeleteKB(ctx context.Context, workspaceID, id uuid.UUID) error

	CreateDocument(ctx context.Context, doc *KBDocument) error
	GetDocumentByID(ctx context.Context, id uuid.UUID) (*KBDocument, error)
	ListDocuments(ctx context.Context, kbID uuid.UUID) ([]*KBDocument, error)
	UpdateDocumentStatus(ctx context.Context, id uuid.UUID, status string, chunkCount int, errMsg *string) error
	DeleteDocument(ctx context.Context, id uuid.UUID) error
	SearchChunks(ctx context.Context, workspaceID uuid.UUID, kbIDs []uuid.UUID, embeddingStr string, limit int) ([]*KBSearchResult, error)
}
