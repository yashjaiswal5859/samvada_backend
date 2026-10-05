// =============================================================================
// Samvada Platform — ABS Audit Log Domain Entity & Repository (Phase 6.4)
// =============================================================================

package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type AuditLog struct {
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	WorkspaceID  *uuid.UUID `gorm:"type:uuid;index" json:"workspace_id,omitempty"`
	OrgID        *uuid.UUID `gorm:"type:uuid" json:"org_id,omitempty"`
	UserID       uuid.UUID  `gorm:"type:uuid;not null;index" json:"user_id"`
	Action       string     `gorm:"type:varchar(64);not null;index" json:"action"`
	ResourceType string     `gorm:"type:varchar(64);not null" json:"resource_type"`
	ResourceID   string     `gorm:"type:varchar(128);not null;default:''" json:"resource_id"`
	IPAddress    string     `gorm:"type:varchar(45);not null;default:''" json:"ip_address"`
	UserAgent    string     `gorm:"type:text;not null;default:''" json:"user_agent"`
	Details      string     `gorm:"type:jsonb;not null;default:'{}'" json:"details"`
	CreatedAt    time.Time  `gorm:"autoCreateTime;index" json:"created_at"`
}

func (AuditLog) TableName() string {
	return "audit_logs"
}

type AuditLogFilter struct {
	WorkspaceID  *uuid.UUID
	UserID       *uuid.UUID
	Action       string
	ResourceType string
	Limit        int
	Offset       int
}

type AuditRepository interface {
	Record(ctx context.Context, log *AuditLog) error
	List(ctx context.Context, filter AuditLogFilter) ([]*AuditLog, int64, error)
}
