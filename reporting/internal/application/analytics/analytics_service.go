// =============================================================================
// Samvada Platform — Reporting Engine Analytics Application Service
// =============================================================================

package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/samvada/reporting/internal/domain"
	"github.com/samvada/reporting/internal/infrastructure/redis"
)

type AnalyticsService interface {
	GetKPISummary(ctx context.Context, orgID uuid.UUID, workspaceID *uuid.UUID, start, end time.Time) (*domain.KPISummary, error)
	GetDailyVolumeTrend(ctx context.Context, orgID uuid.UUID, workspaceID *uuid.UUID, days int) ([]*domain.DailyVolumeTrend, error)
	GetAgentLeaderboard(ctx context.Context, orgID uuid.UUID, workspaceID *uuid.UUID, start, end time.Time, limit int) ([]*domain.AgentLeaderboardItem, error)
	GetConversionFunnels(ctx context.Context, orgID uuid.UUID, workspaceID *uuid.UUID, start, end time.Time) ([]*domain.ConversionFunnelStage, error)
	GetLiveCallCount(ctx context.Context, orgID uuid.UUID, workspaceID *uuid.UUID) (int64, error)
}

type analyticsService struct {
	analyticsRepo domain.AnalyticsRepository
	redisClient   *redis.Client
	log           *zap.Logger
}

func NewAnalyticsService(
	analyticsRepo domain.AnalyticsRepository,
	redisClient *redis.Client,
	log *zap.Logger,
) AnalyticsService {
	return &analyticsService{
		analyticsRepo: analyticsRepo,
		redisClient:   redisClient,
		log:           log.Named("analytics_service"),
	}
}

func (s *analyticsService) GetKPISummary(
	ctx context.Context,
	orgID uuid.UUID,
	workspaceID *uuid.UUID,
	start, end time.Time,
) (*domain.KPISummary, error) {
	wsKey := "all"
	if workspaceID != nil {
		wsKey = workspaceID.String()
	}
	cacheKey := fmt.Sprintf("samvada:analytics:kpi:%s:%s:%d:%d", orgID, wsKey, start.Unix(), end.Unix())

	if s.redisClient != nil {
		if val, err := s.redisClient.Get(ctx, cacheKey); err == nil && val != "" {
			var cached domain.KPISummary
			if err := json.Unmarshal([]byte(val), &cached); err == nil {
				return &cached, nil
			}
		}
	}

	kpi, err := s.analyticsRepo.GetKPISummary(ctx, orgID, workspaceID, start, end)
	if err != nil {
		return nil, err
	}

	if s.redisClient != nil {
		if raw, err := json.Marshal(kpi); err == nil {
			_ = s.redisClient.Set(ctx, cacheKey, string(raw), 60*time.Second)
		}
	}

	return kpi, nil
}

func (s *analyticsService) GetDailyVolumeTrend(
	ctx context.Context,
	orgID uuid.UUID,
	workspaceID *uuid.UUID,
	days int,
) ([]*domain.DailyVolumeTrend, error) {
	wsKey := "all"
	if workspaceID != nil {
		wsKey = workspaceID.String()
	}
	cacheKey := fmt.Sprintf("samvada:analytics:trend:%s:%s:%d", orgID, wsKey, days)

	if s.redisClient != nil {
		if val, err := s.redisClient.Get(ctx, cacheKey); err == nil && val != "" {
			var cached []*domain.DailyVolumeTrend
			if err := json.Unmarshal([]byte(val), &cached); err == nil {
				return cached, nil
			}
		}
	}

	trends, err := s.analyticsRepo.GetDailyVolumeTrend(ctx, orgID, workspaceID, days)
	if err != nil {
		return nil, err
	}

	if s.redisClient != nil {
		if raw, err := json.Marshal(trends); err == nil {
			_ = s.redisClient.Set(ctx, cacheKey, string(raw), 60*time.Second)
		}
	}

	return trends, nil
}

func (s *analyticsService) GetAgentLeaderboard(
	ctx context.Context,
	orgID uuid.UUID,
	workspaceID *uuid.UUID,
	start, end time.Time,
	limit int,
) ([]*domain.AgentLeaderboardItem, error) {
	return s.analyticsRepo.GetAgentLeaderboard(ctx, orgID, workspaceID, start, end, limit)
}

func (s *analyticsService) GetConversionFunnels(
	ctx context.Context,
	orgID uuid.UUID,
	workspaceID *uuid.UUID,
	start, end time.Time,
) ([]*domain.ConversionFunnelStage, error) {
	return s.analyticsRepo.GetConversionFunnels(ctx, orgID, workspaceID, start, end)
}

func (s *analyticsService) GetLiveCallCount(
	ctx context.Context,
	orgID uuid.UUID,
	workspaceID *uuid.UUID,
) (int64, error) {
	if s.redisClient == nil {
		return 0, nil
	}

	key := fmt.Sprintf("samvada:live_calls:org:%s", orgID.String())
	if workspaceID != nil {
		key = fmt.Sprintf("samvada:live_calls:ws:%s", workspaceID.String())
	}

	val, err := s.redisClient.Get(ctx, key)
	if err != nil || val == "" {
		return 0, nil
	}

	count, _ := strconv.ParseInt(val, 10, 64)
	if count < 0 {
		count = 0
	}
	return count, nil
}
