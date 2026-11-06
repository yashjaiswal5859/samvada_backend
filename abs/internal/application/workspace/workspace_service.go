// =============================================================================
// Samvada Platform — ABS Workspace Application Service
// =============================================================================

package workspace

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/samvada/abs/internal/domain"
	"github.com/samvada/abs/internal/infrastructure/keto"
)

type CreateWorkspaceRequest struct {
	Name               string    `json:"name" binding:"required,min=2,max=100"`
	Slug               string    `json:"slug"`
	Icon               string    `json:"icon"`
	Description        *string   `json:"description,omitempty"`
	MonthlyMinuteLimit int       `json:"monthly_minute_limit"`
	CreatorUserID      uuid.UUID `json:"-"`
}

type UpdateWorkspaceRequest struct {
	Name               *string `json:"name,omitempty"`
	Icon               *string `json:"icon,omitempty"`
	Description        *string `json:"description,omitempty"`
	MonthlyMinuteLimit *int    `json:"monthly_minute_limit,omitempty"`
}

type WorkspaceService interface {
	SetKetoClient(ketoClient keto.Client)
	CreateWorkspace(ctx context.Context, orgID uuid.UUID, req CreateWorkspaceRequest) (*domain.Workspace, error)
	GetWorkspace(ctx context.Context, id uuid.UUID) (*domain.Workspace, error)
	GetWorkspaceBySlug(ctx context.Context, orgID uuid.UUID, slug string) (*domain.Workspace, error)
	ListWorkspaces(ctx context.Context, orgID uuid.UUID) ([]*domain.Workspace, error)
	ListUserWorkspaces(ctx context.Context, userID uuid.UUID) ([]*domain.Workspace, error)
	UpdateWorkspace(ctx context.Context, id uuid.UUID, req UpdateWorkspaceRequest) (*domain.Workspace, error)
	DeleteWorkspace(ctx context.Context, id uuid.UUID) error

	AddMember(ctx context.Context, workspaceID, userID uuid.UUID, role string) (*domain.WorkspaceMember, error)
	ListMembers(ctx context.Context, workspaceID uuid.UUID) ([]*domain.WorkspaceMember, error)
	UpdateMemberRole(ctx context.Context, workspaceID, userID uuid.UUID, role string) error
	RemoveMember(ctx context.Context, workspaceID, userID uuid.UUID) error
}

type workspaceService struct {
	wsRepo     domain.WorkspaceRepository
	ketoClient keto.Client
	log        *zap.Logger
}

func NewWorkspaceService(wsRepo domain.WorkspaceRepository, log *zap.Logger) WorkspaceService {
	return &workspaceService{
		wsRepo: wsRepo,
		log:    log.Named("workspace_service"),
	}
}

func (s *workspaceService) SetKetoClient(ketoClient keto.Client) {
	s.ketoClient = ketoClient
}

func (s *workspaceService) CreateWorkspace(
	ctx context.Context,
	orgID uuid.UUID,
	req CreateWorkspaceRequest,
) (*domain.Workspace, error) {
	slug := req.Slug
	if slug == "" {
		slug = generateSlug(req.Name)
	}

	if _, err := s.wsRepo.GetBySlug(ctx, orgID, slug); err == nil {
		slug = fmt.Sprintf("%s-%d", slug, time.Now().Unix()%10000)
	}

	icon := req.Icon
	if icon == "" {
		icon = "💼"
	}

	minuteLimit := req.MonthlyMinuteLimit
	if minuteLimit <= 0 {
		minuteLimit = 10000
	}

	wsID := uuid.New()
	ws := &domain.Workspace{
		ID:                 wsID,
		OrgID:              orgID,
		Name:               req.Name,
		Slug:               slug,
		Icon:               icon,
		Description:        req.Description,
		MonthlyMinuteLimit: minuteLimit,
		IsDefault:          false,
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}

	if err := s.wsRepo.Create(ctx, ws); err != nil {
		s.log.Error("Failed to create workspace", zap.Error(err))
		return nil, err
	}

	// Add creator as workspace admin
	if req.CreatorUserID != uuid.Nil {
		member := &domain.WorkspaceMember{
			ID:            uuid.New(),
			WorkspaceID:   wsID,
			UserID:        req.CreatorUserID,
			WorkspaceRole: "workspace_admin",
			CreatedAt:     time.Now().UTC(),
		}
		_ = s.wsRepo.AddMember(ctx, member)

		// Mirror relationship tuples in Ory Keto ReBAC
		if s.ketoClient != nil {
			_ = s.ketoClient.CreateRelationship(ctx, "workspaces", wsID.String(), "owner", req.CreatorUserID.String())
			_ = s.ketoClient.CreateRelationship(ctx, "workspaces", wsID.String(), "admin", req.CreatorUserID.String())
		}
	}

	return ws, nil
}

func (s *workspaceService) GetWorkspace(ctx context.Context, id uuid.UUID) (*domain.Workspace, error) {
	return s.wsRepo.GetByID(ctx, id)
}

func (s *workspaceService) GetWorkspaceBySlug(ctx context.Context, orgID uuid.UUID, slug string) (*domain.Workspace, error) {
	return s.wsRepo.GetBySlug(ctx, orgID, slug)
}

func (s *workspaceService) ListWorkspaces(ctx context.Context, orgID uuid.UUID) ([]*domain.Workspace, error) {
	return s.wsRepo.ListByOrg(ctx, orgID)
}

func (s *workspaceService) ListUserWorkspaces(ctx context.Context, userID uuid.UUID) ([]*domain.Workspace, error) {
	return s.wsRepo.ListForUser(ctx, userID)
}

func (s *workspaceService) UpdateWorkspace(ctx context.Context, id uuid.UUID, req UpdateWorkspaceRequest) (*domain.Workspace, error) {
	ws, err := s.wsRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.Name != nil {
		ws.Name = *req.Name
	}
	if req.Icon != nil {
		ws.Icon = *req.Icon
	}
	if req.Description != nil {
		ws.Description = req.Description
	}
	if req.MonthlyMinuteLimit != nil {
		ws.MonthlyMinuteLimit = *req.MonthlyMinuteLimit
	}
	ws.UpdatedAt = time.Now().UTC()
	if err := s.wsRepo.Update(ctx, ws); err != nil {
		return nil, err
	}
	return ws, nil
}

func (s *workspaceService) DeleteWorkspace(ctx context.Context, id uuid.UUID) error {
	return s.wsRepo.Delete(ctx, id)
}

func (s *workspaceService) AddMember(ctx context.Context, workspaceID, userID uuid.UUID, role string) (*domain.WorkspaceMember, error) {
	if role == "" {
		role = "viewer"
	}
	member := &domain.WorkspaceMember{
		ID:            uuid.New(),
		WorkspaceID:   workspaceID,
		UserID:        userID,
		WorkspaceRole: role,
		CreatedAt:     time.Now().UTC(),
	}
	if err := s.wsRepo.AddMember(ctx, member); err != nil {
		return nil, err
	}

	if s.ketoClient != nil {
		_ = s.ketoClient.CreateRelationship(ctx, "workspaces", workspaceID.String(), role, userID.String())
	}

	return member, nil
}

func (s *workspaceService) ListMembers(ctx context.Context, workspaceID uuid.UUID) ([]*domain.WorkspaceMember, error) {
	return s.wsRepo.ListMembers(ctx, workspaceID)
}

func (s *workspaceService) UpdateMemberRole(ctx context.Context, workspaceID, userID uuid.UUID, role string) error {
	return s.wsRepo.UpdateMemberRole(ctx, workspaceID, userID, role)
}

func (s *workspaceService) RemoveMember(ctx context.Context, workspaceID, userID uuid.UUID) error {
	return s.wsRepo.RemoveMember(ctx, workspaceID, userID)
}

var nonAlphanumericRegex = regexp.MustCompile(`[^a-z0-9]+`)

func generateSlug(name string) string {
	slug := strings.ToLower(strings.TrimSpace(name))
	slug = nonAlphanumericRegex.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "ws"
	}
	return slug
}
