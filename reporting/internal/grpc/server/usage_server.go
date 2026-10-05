// =============================================================================
// Samvada Platform — Reporting Engine gRPC Usage Reporting Server
// =============================================================================

package server

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/samvada/reporting/internal/application/usage"
	reportingv1 "github.com/samvada/proto/gen/go/reporting/v1"
)

type UsageServer struct {
	reportingv1.UnimplementedUsageReportingServiceServer
	meteringService usage.MeteringService
	log             *zap.Logger
}

func NewUsageServer(meteringService usage.MeteringService, log *zap.Logger) *UsageServer {
	return &UsageServer{
		meteringService: meteringService,
		log:             log.Named("grpc_usage_server"),
	}
}

func (s *UsageServer) GetUsageSummary(
	ctx context.Context,
	req *reportingv1.GetUsageSummaryRequest,
) (*reportingv1.GetUsageSummaryResponse, error) {
	s.log.Info("gRPC GetUsageSummary received",
		zap.String("org_id", req.GetOrgId()),
		zap.String("period_start", req.GetPeriodStart()),
		zap.String("period_end", req.GetPeriodEnd()),
	)

	orgID, err := uuid.Parse(req.GetOrgId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid org_id UUID: %v", err)
	}

	start, err := time.Parse(time.RFC3339, req.GetPeriodStart())
	if err != nil {
		start = time.Now().UTC().AddDate(0, 0, -30)
	}

	end, err := time.Parse(time.RFC3339, req.GetPeriodEnd())
	if err != nil {
		end = time.Now().UTC()
	}

	summary, err := s.meteringService.GetUsageSummary(ctx, orgID, start, end)
	if err != nil {
		s.log.Error("Failed to fetch usage summary from ClickHouse", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "usage summary query failed: %v", err)
	}

	return &reportingv1.GetUsageSummaryResponse{
		TotalCalls:            summary.TotalCalls,
		TotalDurationSeconds:  summary.TotalDurationSeconds,
		TotalBillableSeconds:  summary.TotalBillableSeconds,
		TotalCostTelephonyUsd: summary.TotalCostTelephonyUSD,
		TotalCostSttUsd:       summary.TotalCostSTTUSD,
		TotalCostLlmUsd:       summary.TotalCostLLMUSD,
		TotalCostTtsUsd:       summary.TotalCostTTSUSD,
		TotalSpendUsd:         summary.TotalSpendUSD,
		PerWorkspaceMinutes:   summary.PerWorkspaceMinutes,
	}, nil
}

func (s *UsageServer) GetCurrentMonthUsage(
	ctx context.Context,
	req *reportingv1.GetCurrentMonthUsageRequest,
) (*reportingv1.GetCurrentMonthUsageResponse, error) {
	s.log.Info("gRPC GetCurrentMonthUsage received",
		zap.String("org_id", req.GetOrgId()),
		zap.String("workspace_id", req.GetWorkspaceId()),
	)

	orgID, err := uuid.Parse(req.GetOrgId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid org_id UUID: %v", err)
	}

	var wsID uuid.UUID
	if req.GetWorkspaceId() != "" {
		wsID, _ = uuid.Parse(req.GetWorkspaceId())
	}

	cur, err := s.meteringService.GetCurrentMonthUsage(ctx, orgID, wsID)
	if err != nil {
		s.log.Error("Failed to fetch current month usage from ClickHouse", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "current month usage query failed: %v", err)
	}

	return &reportingv1.GetCurrentMonthUsageResponse{
		MinutesUsed:       cur.MinutesUsed,
		MinutesLimit:      cur.MinutesLimit,
		EstimatedSpendUsd: cur.EstimatedSpendUSD,
		PercentUsed:       cur.PercentUsed,
	}, nil
}
