// =============================================================================
// Samvada Platform — ABS Agent Application Service
// =============================================================================

package agent

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/datatypes"

	"github.com/samvada/abs/internal/domain"
)

type CreateAgentRequest struct {
	Name            string         `json:"name" binding:"required,min=2,max=100"`
	Role            string         `json:"role" binding:"required"`
	Description     *string        `json:"description,omitempty"`
	AvatarURL       *string        `json:"avatar_url,omitempty"`
	SystemPrompt    string         `json:"system_prompt" binding:"required"`
	VoiceID         string         `json:"voice_id" binding:"required"`
	VoiceProvider   string         `json:"voice_provider"`
	VoiceName       string         `json:"voice_name"`
	Language        string         `json:"language"`
	Model           string         `json:"model"`
	Status          string         `json:"status,omitempty"`
	AssignedPhone   *string        `json:"assigned_phone,omitempty"`
	ToolsConfig     datatypes.JSON `json:"tools_config"`
	CreatedByUserID uuid.UUID      `json:"-"`
}

type UpdateAgentRequest struct {
	Name          *string         `json:"name,omitempty"`
	Role          *string         `json:"role,omitempty"`
	Description   *string         `json:"description,omitempty"`
	AvatarURL     *string         `json:"avatar_url,omitempty"`
	SystemPrompt  *string         `json:"system_prompt,omitempty"`
	VoiceID       *string         `json:"voice_id,omitempty"`
	VoiceProvider *string         `json:"voice_provider,omitempty"`
	VoiceName     *string         `json:"voice_name,omitempty"`
	Language      *string         `json:"language,omitempty"`
	Model         *string         `json:"model,omitempty"`
	AssignedPhone *string         `json:"assigned_phone,omitempty"`
	Status        *string         `json:"status,omitempty"`
	ToolsConfig   *datatypes.JSON `json:"tools_config,omitempty"`
}

type AgentService interface {
	CreateAgent(ctx context.Context, workspaceID uuid.UUID, req CreateAgentRequest) (*domain.Agent, error)
	GetAgent(ctx context.Context, workspaceID, id uuid.UUID) (*domain.Agent, error)
	ListAgents(ctx context.Context, workspaceID uuid.UUID) ([]*domain.Agent, error)
	UpdateAgent(ctx context.Context, workspaceID, id uuid.UUID, req UpdateAgentRequest) (*domain.Agent, error)
	DeleteAgent(ctx context.Context, workspaceID, id uuid.UUID) error

	PublishVersion(ctx context.Context, workspaceID, agentID, publishedBy uuid.UUID, summary string) (*domain.AgentVersion, error)
	ListVersions(ctx context.Context, workspaceID, agentID uuid.UUID) ([]*domain.AgentVersion, error)
	RollbackVersion(ctx context.Context, workspaceID, agentID uuid.UUID, versionNum int) (*domain.Agent, error)
}

type agentService struct {
	agentRepo domain.AgentRepository
	wsRepo    domain.WorkspaceRepository
	log       *zap.Logger
}

func NewAgentService(
	agentRepo domain.AgentRepository,
	wsRepo domain.WorkspaceRepository,
	log *zap.Logger,
) AgentService {
	return &agentService{
		agentRepo: agentRepo,
		wsRepo:    wsRepo,
		log:       log.Named("agent_service"),
	}
}

func (s *agentService) CreateAgent(
	ctx context.Context,
	workspaceID uuid.UUID,
	req CreateAgentRequest,
) (*domain.Agent, error) {
	// 1. Quota Check: count current agents in workspace
	count, err := s.agentRepo.CountByWorkspace(ctx, workspaceID)
	if err == nil && count >= 50 {
		return nil, domain.ErrAgentLimitExceeded
	}

	voiceProvider := req.VoiceProvider
	if voiceProvider == "" {
		voiceProvider = "sarvam"
	}
	voiceName := req.VoiceName
	if voiceName == "" {
		voiceName = "Sarvam Aravind"
	}
	lang := req.Language
	if lang == "" {
		lang = "en-US"
	}
	model := req.Model
	if model == "" {
		model = "gemini-2.5-flash"
	}

	status := req.Status
	if status == "" {
		status = "active"
	}

	agentID := uuid.New()
	ag := &domain.Agent{
		ID:               agentID,
		WorkspaceID:      workspaceID,
		Name:             req.Name,
		Role:             req.Role,
		Description:      req.Description,
		AvatarURL:        req.AvatarURL,
		SystemPrompt:     req.SystemPrompt,
		VoiceID:          req.VoiceID,
		VoiceProvider:    voiceProvider,
		VoiceName:        voiceName,
		Language:         lang,
		Model:            model,
		Status:           status,
		ActiveVersionNum: 1,
		AssignedPhone:    req.AssignedPhone,
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	}

	if err := s.agentRepo.Create(ctx, ag); err != nil {
		s.log.Error("Failed to create agent", zap.Error(err))
		return nil, err
	}

	// 2. Create initial draft version (v1)
	toolsCfg := req.ToolsConfig
	if len(toolsCfg) == 0 {
		toolsCfg = datatypes.JSON(`[]`)
	}

	v1 := &domain.AgentVersion{
		ID:           uuid.New(),
		AgentID:      agentID,
		VersionNum:   1,
		SystemPrompt: req.SystemPrompt,
		Model:        model,
		VoiceID:      req.VoiceID,
		Language:     lang,
		ToolsConfig:  toolsCfg,
		PublishedBy:  req.CreatedByUserID,
		CreatedAt:    time.Now().UTC(),
	}
	_ = s.agentRepo.CreateVersion(ctx, v1)

	s.log.Info("Agent successfully created",
		zap.String("agent_id", agentID.String()),
		zap.String("workspace_id", workspaceID.String()),
		zap.String("name", req.Name),
	)

	return ag, nil
}

func (s *agentService) GetAgent(ctx context.Context, workspaceID, id uuid.UUID) (*domain.Agent, error) {
	return s.agentRepo.GetByID(ctx, workspaceID, id)
}

func (s *agentService) ListAgents(ctx context.Context, workspaceID uuid.UUID) ([]*domain.Agent, error) {
	return s.agentRepo.ListByWorkspace(ctx, workspaceID)
}

func (s *agentService) UpdateAgent(
	ctx context.Context,
	workspaceID, id uuid.UUID,
	req UpdateAgentRequest,
) (*domain.Agent, error) {
	ag, err := s.agentRepo.GetByID(ctx, workspaceID, id)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		ag.Name = *req.Name
	}
	if req.Role != nil {
		ag.Role = *req.Role
	}
	if req.Description != nil {
		ag.Description = req.Description
	}
	if req.AvatarURL != nil {
		ag.AvatarURL = req.AvatarURL
	}
	if req.SystemPrompt != nil {
		ag.SystemPrompt = *req.SystemPrompt
	}
	if req.VoiceID != nil {
		ag.VoiceID = *req.VoiceID
	}
	if req.VoiceProvider != nil {
		ag.VoiceProvider = *req.VoiceProvider
	}
	if req.VoiceName != nil {
		ag.VoiceName = *req.VoiceName
	}
	if req.Language != nil {
		ag.Language = *req.Language
	}
	if req.Model != nil {
		ag.Model = *req.Model
	}
	if req.AssignedPhone != nil {
		ag.AssignedPhone = req.AssignedPhone
	}
	if req.Status != nil {
		ag.Status = *req.Status
	}
	ag.UpdatedAt = time.Now().UTC()

	if req.ToolsConfig != nil {
		latest, lErr := s.agentRepo.GetLatestVersion(ctx, id)
		if lErr == nil && latest != nil {
			latest.ToolsConfig = *req.ToolsConfig
			if req.SystemPrompt != nil {
				latest.SystemPrompt = *req.SystemPrompt
			}
			if req.Model != nil {
				latest.Model = *req.Model
			}
			if req.VoiceID != nil {
				latest.VoiceID = *req.VoiceID
			}
			if req.Language != nil {
				latest.Language = *req.Language
			}
			_ = s.agentRepo.UpdateVersion(ctx, latest)
		}
	}

	if err := s.agentRepo.Update(ctx, ag); err != nil {
		return nil, err
	}
	return ag, nil
}

func (s *agentService) DeleteAgent(ctx context.Context, workspaceID, id uuid.UUID) error {
	return s.agentRepo.Delete(ctx, workspaceID, id)
}

func (s *agentService) PublishVersion(
	ctx context.Context,
	workspaceID, agentID, publishedBy uuid.UUID,
	summary string,
) (*domain.AgentVersion, error) {
	ag, err := s.agentRepo.GetByID(ctx, workspaceID, agentID)
	if err != nil {
		return nil, err
	}

	latest, err := s.agentRepo.GetLatestVersion(ctx, agentID)
	nextVersionNum := 1
	if err == nil && latest != nil {
		nextVersionNum = latest.VersionNum + 1
	}

	var sumPtr *string
	if summary != "" {
		sumPtr = &summary
	}

	newVersion := &domain.AgentVersion{
		ID:            uuid.New(),
		AgentID:       agentID,
		VersionNum:    nextVersionNum,
		SystemPrompt:  ag.SystemPrompt,
		Model:         ag.Model,
		VoiceID:       ag.VoiceID,
		Language:      ag.Language,
		PublishedBy:   publishedBy,
		ChangeSummary: sumPtr,
		CreatedAt:     time.Now().UTC(),
	}

	if err := s.agentRepo.CreateVersion(ctx, newVersion); err != nil {
		return nil, err
	}

	ag.Status = "active"
	ag.ActiveVersionNum = nextVersionNum
	ag.UpdatedAt = time.Now().UTC()
	_ = s.agentRepo.Update(ctx, ag)

	s.log.Info("Agent version published",
		zap.String("agent_id", agentID.String()),
		zap.Int("version_num", nextVersionNum),
	)

	return newVersion, nil
}

func (s *agentService) ListVersions(ctx context.Context, workspaceID, agentID uuid.UUID) ([]*domain.AgentVersion, error) {
	// Verify workspace ownership
	if _, err := s.agentRepo.GetByID(ctx, workspaceID, agentID); err != nil {
		return nil, err
	}
	return s.agentRepo.ListVersions(ctx, agentID)
}

func (s *agentService) RollbackVersion(
	ctx context.Context,
	workspaceID, agentID uuid.UUID,
	versionNum int,
) (*domain.Agent, error) {
	ag, err := s.agentRepo.GetByID(ctx, workspaceID, agentID)
	if err != nil {
		return nil, err
	}

	targetVersion, err := s.agentRepo.GetVersion(ctx, agentID, versionNum)
	if err != nil {
		return nil, err
	}

	ag.SystemPrompt = targetVersion.SystemPrompt
	ag.Model = targetVersion.Model
	ag.VoiceID = targetVersion.VoiceID
	ag.Language = targetVersion.Language
	ag.ActiveVersionNum = targetVersion.VersionNum
	ag.UpdatedAt = time.Now().UTC()

	if err := s.agentRepo.Update(ctx, ag); err != nil {
		return nil, err
	}

	s.log.Info("Agent rolled back to version",
		zap.String("agent_id", agentID.String()),
		zap.Int("rolled_back_to", versionNum),
	)

	return ag, nil
}
