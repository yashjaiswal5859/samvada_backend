// =============================================================================
// Samvada Platform — ABS S2S (Speech-to-Speech) gRPC Client
// =============================================================================

package client

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type OutboundCallRequest struct {
	CampaignID  uuid.UUID         `json:"campaign_id"`
	AgentID     uuid.UUID         `json:"agent_id"`
	WorkspaceID uuid.UUID         `json:"workspace_id"`
	ContactID   uuid.UUID         `json:"contact_id"`
	PhoneNumber string            `json:"phone_number"`
	Variables   map[string]string `json:"variables,omitempty"`
}

type OutboundCallResponse struct {
	CallID    string `json:"call_id"`
	Status    string `json:"status"` // queued, dialing, connected, failed
	ErrorMsg  string `json:"error_msg,omitempty"`
}

type S2SClient interface {
	InitiateOutboundCall(ctx context.Context, req OutboundCallRequest) (*OutboundCallResponse, error)
	Close() error
}

type s2sClient struct {
	conn *grpc.ClientConn
	log  *zap.Logger
}

func NewS2SClient(target string, log *zap.Logger) (S2SClient, error) {
	conn, err := grpc.Dial(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Warn("Could not establish immediate connection to S2S Engine (will retry lazily)",
			zap.String("target", target),
			zap.Error(err),
		)
	}

	return &s2sClient{
		conn: conn,
		log:  log.Named("s2s_client"),
	}, nil
}

func (c *s2sClient) InitiateOutboundCall(
	ctx context.Context,
	req OutboundCallRequest,
) (*OutboundCallResponse, error) {
	c.log.Info("Initiating outbound call dispatch to S2S engine",
		zap.String("campaign_id", req.CampaignID.String()),
		zap.String("agent_id", req.AgentID.String()),
		zap.String("phone_number", req.PhoneNumber),
	)

	// Simulated / stubbed response until S2S engine outbound proto is registered
	return &OutboundCallResponse{
		CallID: "call_" + uuid.New().String(),
		Status: "queued",
	}, nil
}

func (c *s2sClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
