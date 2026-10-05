// =============================================================================
// Samvada Platform — ABS Agent Runtime gRPC Server
// =============================================================================

package server

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/samvada/abs/internal/application/agent"
	"github.com/samvada/abs/internal/application/billing"
	"github.com/samvada/abs/internal/application/workspace"
	absv1 "github.com/samvada/proto/gen/go/abs/v1"
)

type AgentRuntimeServer struct {
	absv1.UnimplementedAgentRuntimeServiceServer
	agentService     agent.AgentService
	workspaceService workspace.WorkspaceService
	walletService    billing.WalletService
	log              *zap.Logger
}

func NewAgentRuntimeServer(
	agentService agent.AgentService,
	workspaceService workspace.WorkspaceService,
	walletService billing.WalletService,
	log *zap.Logger,
) *AgentRuntimeServer {
	return &AgentRuntimeServer{
		agentService:     agentService,
		workspaceService: workspaceService,
		walletService:    walletService,
		log:              log.Named("grpc_agent_runtime"),
	}
}

// GetAgentRuntimeConfig supplies the Speech-to-Speech (S2S) Engine with real-time prompt, voice, model & tools
func (s *AgentRuntimeServer) GetAgentRuntimeConfig(
	ctx context.Context,
	req *absv1.GetAgentRuntimeConfigRequest,
) (*absv1.GetAgentRuntimeConfigResponse, error) {
	s.log.Info("gRPC GetAgentRuntimeConfig received",
		zap.String("agent_id", req.GetAgentId()),
		zap.String("workspace_id", req.GetWorkspaceId()),
	)

	agentID, err := uuid.Parse(req.GetAgentId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid agent_id UUID: %v", err)
	}

	workspaceID, err := uuid.Parse(req.GetWorkspaceId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid workspace_id UUID: %v", err)
	}

	ag, err := s.agentService.GetAgent(ctx, workspaceID, agentID)
	if err != nil {
		s.log.Warn("Failed to find agent for runtime config",
			zap.String("agent_id", req.GetAgentId()),
			zap.Error(err),
		)
		return nil, status.Errorf(codes.NotFound, "agent %s not found in workspace %s", agentID, workspaceID)
	}

	var toolsJSON string
	if len(ag.Versions) > 0 {
		toolsJSON = string(ag.Versions[0].ToolsConfig)
	}

	return &absv1.GetAgentRuntimeConfigResponse{
		AgentId:          ag.ID.String(),
		Name:             ag.Name,
		SystemPrompt:     ag.SystemPrompt,
		VoiceId:          ag.VoiceID,
		VoiceProvider:    ag.VoiceProvider,
		Language:         ag.Language,
		Model:            ag.Model,
		KnowledgeBaseIds: []string{},
		ToolsConfigJson:  toolsJSON,
	}, nil
}

// CheckCallAuthorization validates that the tenant has an active plan, positive wallet balance, and minute quota
func (s *AgentRuntimeServer) CheckCallAuthorization(
	ctx context.Context,
	req *absv1.CheckCallAuthorizationRequest,
) (*absv1.CheckCallAuthorizationResponse, error) {
	s.log.Info("gRPC CheckCallAuthorization received",
		zap.String("workspace_id", req.GetWorkspaceId()),
		zap.String("phone_number", req.GetPhoneNumber()),
		zap.String("direction", req.GetDirection()),
	)

	workspaceID, err := uuid.Parse(req.GetWorkspaceId())
	if err != nil {
		return &absv1.CheckCallAuthorizationResponse{
			IsAuthorized: false,
			DenialReason: "INVALID_WORKSPACE_ID",
		}, nil
	}

	ws, err := s.workspaceService.GetWorkspace(ctx, workspaceID)
	if err != nil {
		s.log.Warn("Workspace lookup failed during call authorization",
			zap.String("workspace_id", req.GetWorkspaceId()),
			zap.Error(err),
		)
		return &absv1.CheckCallAuthorizationResponse{
			IsAuthorized: false,
			DenialReason: "WORKSPACE_NOT_FOUND",
		}, nil
	}

	authResult, err := s.walletService.CheckCallAuthorization(ctx, ws.OrgID)
	if err != nil {
		s.log.Error("Wallet check call authorization failed",
			zap.String("org_id", ws.OrgID.String()),
			zap.Error(err),
		)
		return &absv1.CheckCallAuthorizationResponse{
			IsAuthorized: false,
			DenialReason: "BILLING_SYSTEM_ERROR",
		}, nil
	}

	denialReason := ""
	if authResult.DenialReason != nil {
		denialReason = *authResult.DenialReason
	}

	return &absv1.CheckCallAuthorizationResponse{
		IsAuthorized:            authResult.IsAuthorized,
		DenialReason:            denialReason,
		RemainingMinutes:        int64(authResult.EstimatedRemainingMinutes),
		CurrentWalletBalanceUsd: authResult.CurrentBalanceUSD,
	}, nil
}
