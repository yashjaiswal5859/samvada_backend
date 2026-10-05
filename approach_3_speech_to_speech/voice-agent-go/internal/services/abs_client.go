// =============================================================================
// Samvada Platform — S2S ↔ ABS gRPC Client & Redis Cache (Phase 3.1)
// =============================================================================
// Fetches dynamic agent runtime configuration and performs call authorization
// checks via ABS gRPC on :9082 with sub-millisecond Redis caching.
// =============================================================================

package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	absv1 "github.com/samvada/proto/gen/go/abs/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type ABSClient struct {
	client      absv1.AgentRuntimeServiceClient
	redisClient *redis.Client
	conn        *grpc.ClientConn
}

func NewABSClient(grpcAddr string, redisClient *redis.Client) (*ABSClient, error) {
	if grpcAddr == "" {
		grpcAddr = "localhost:9082"
	}

	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to ABS gRPC server at %s: %w", grpcAddr, err)
	}

	client := absv1.NewAgentRuntimeServiceClient(conn)
	return &ABSClient{
		client:      client,
		redisClient: redisClient,
		conn:        conn,
	}, nil
}

func (c *ABSClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *ABSClient) GetAgentRuntimeConfig(ctx context.Context, workspaceID, agentID string) (*absv1.GetAgentRuntimeConfigResponse, error) {
	cacheKey := fmt.Sprintf("agent:config:%s", agentID)

	// 1. Check Redis cache first (60s TTL)
	if c.redisClient != nil {
		if cached, err := c.redisClient.Get(ctx, cacheKey).Result(); err == nil && cached != "" {
			var resp absv1.GetAgentRuntimeConfigResponse
			if err := json.Unmarshal([]byte(cached), &resp); err == nil {
				return &resp, nil
			}
		}
	}

	// 2. Cache miss: Fetch from ABS via gRPC with retry & backoff
	req := &absv1.GetAgentRuntimeConfigRequest{
		AgentId:     agentID,
		WorkspaceId: workspaceID,
	}

	var resp *absv1.GetAgentRuntimeConfigResponse
	var err error
	maxRetries := 2
	for attempt := 0; attempt <= maxRetries; attempt++ {
		callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		resp, err = c.client.GetAgentRuntimeConfig(callCtx, req)
		cancel()
		if err == nil {
			break
		}
		if attempt < maxRetries {
			time.Sleep(time.Duration(100*(1<<attempt)) * time.Millisecond)
		}
	}

	if err != nil {
		// Fallback to stale backup cache if available
		if c.redisClient != nil {
			backupKey := fmt.Sprintf("agent:config:backup:%s", agentID)
			if cached, bErr := c.redisClient.Get(ctx, backupKey).Result(); bErr == nil && cached != "" {
				var backupResp absv1.GetAgentRuntimeConfigResponse
				if json.Unmarshal([]byte(cached), &backupResp) == nil {
					return &backupResp, nil
				}
			}
		}
		return nil, fmt.Errorf("grpc GetAgentRuntimeConfig failed after retries: %w", err)
	}

	// 3. Cache in Redis: 60s hot cache + 24h resilient backup cache
	if c.redisClient != nil && resp != nil {
		if data, err := json.Marshal(resp); err == nil {
			_ = c.redisClient.Set(ctx, cacheKey, string(data), 60*time.Second).Err()
			backupKey := fmt.Sprintf("agent:config:backup:%s", agentID)
			_ = c.redisClient.Set(ctx, backupKey, string(data), 24*time.Hour).Err()
		}
	}

	return resp, nil
}

func (c *ABSClient) CheckCallAuthorization(ctx context.Context, workspaceID, phoneNumber, direction string) (*absv1.CheckCallAuthorizationResponse, error) {
	req := &absv1.CheckCallAuthorizationRequest{
		WorkspaceId: workspaceID,
		PhoneNumber: phoneNumber,
		Direction:   direction,
	}

	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	return c.client.CheckCallAuthorization(callCtx, req)
}
