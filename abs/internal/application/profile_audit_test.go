// =============================================================================
// Samvada Platform — ABS Profile & Audit Application Services Tests
// =============================================================================

package application_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/samvada/abs/internal/application/audit"
	"github.com/samvada/abs/internal/application/profile"
	"github.com/samvada/abs/internal/domain"
	"github.com/samvada/abs/internal/infrastructure/postgres/repos"
)

func TestProfileService_LifecycleAndGDPR(t *testing.T) {
	db := setupTestDB(t)
	if db == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	log := zap.NewNop()
	profileRepo := repos.NewProfileRepository(db)
	notifRepo := repos.NewNotificationRepository(db)
	orgRepo := repos.NewOrgRepository(db)
	wsRepo := repos.NewWorkspaceRepository(db)

	svc := profile.NewProfileService(profileRepo, notifRepo, orgRepo, wsRepo, nil, log)

	testUserID := uuid.New()

	// 1. Get empty profile
	p, err := svc.GetProfile(ctx, testUserID)
	require.NoError(t, err)
	assert.Equal(t, testUserID, p.UserID)

	// 2. Update profile
	req := profile.UpdateProfileRequest{
		FirstName: "Siddharth",
		LastName:  "Sharma",
		Email:     "siddharth@example.com",
		Phone:     "+919876543210",
		Company:   "Samvada AI Labs",
		JobTitle:  "Head of Engineering",
		Timezone:  "Asia/Kolkata",
		Language:  "en",
		Bio:       "Building the future of voice AI in India",
	}
	updated, err := svc.UpdateProfile(ctx, testUserID, req)
	require.NoError(t, err)
	assert.Equal(t, "Siddharth", updated.FirstName)
	assert.Equal(t, "Asia/Kolkata", updated.Timezone)

	// 3. Export GDPR Article 20
	export, err := svc.ExportData(ctx, testUserID)
	require.NoError(t, err)
	assert.Equal(t, testUserID, export.UserID)
	assert.NotNil(t, export.Profile)
	assert.Equal(t, "Siddharth", export.Profile.FirstName)
	assert.Contains(t, export.ComplianceNotice, "General Data Protection Regulation (GDPR) Article 20")

	// 4. Delete GDPR Article 17
	err = svc.DeleteData(ctx, testUserID)
	require.NoError(t, err)

	// 5. Verify erasure
	afterDelete, err := profileRepo.GetByUserID(ctx, testUserID)
	require.NoError(t, err)
	assert.Nil(t, afterDelete)
}

func TestAuditService_Lifecycle(t *testing.T) {
	db := setupTestDB(t)
	if db == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	log := zap.NewNop()
	auditRepo := repos.NewAuditRepository(db)
	svc := audit.NewAuditService(auditRepo, log)

	wsID := uuid.New()
	userID := uuid.New()

	// 1. Record an audit log
	err := svc.Record(ctx, &domain.AuditLog{
		WorkspaceID:  &wsID,
		UserID:       userID,
		Action:       "agent.publish",
		ResourceType: "agent",
		ResourceID:   "agent-123",
		IPAddress:    "192.168.1.1",
		UserAgent:    "Mozilla/5.0",
		Details:      `{"version": 2, "model": "gemini-2.0-flash"}`,
		CreatedAt:    time.Now().UTC(),
	})
	require.NoError(t, err)

	// 2. Query audit logs
	logs, total, err := svc.List(ctx, domain.AuditLogFilter{
		WorkspaceID: &wsID,
		Limit:       10,
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, total, int64(1))
	assert.NotEmpty(t, logs)
	assert.Equal(t, "agent.publish", logs[0].Action)
}
