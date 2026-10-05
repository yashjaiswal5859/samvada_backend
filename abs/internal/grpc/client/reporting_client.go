// =============================================================================
// Samvada Platform — ABS Reporting Engine gRPC Client
// =============================================================================

package client

import (
	"context"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/samvada/abs/internal/pkg/circuitbreaker"
	reportingv1 "github.com/samvada/proto/gen/go/reporting/v1"
)

type ReportingClient interface {
	GetUsageSummary(ctx context.Context, orgID, periodStart, periodEnd string) (*reportingv1.GetUsageSummaryResponse, error)
	GetCurrentMonthUsage(ctx context.Context, orgID, workspaceID string) (*reportingv1.GetCurrentMonthUsageResponse, error)
	Close() error
}

type reportingClient struct {
	conn   *grpc.ClientConn
	client reportingv1.UsageReportingServiceClient
	cb     *circuitbreaker.CircuitBreaker
	log    *zap.Logger
}

func NewReportingClient(target string, log *zap.Logger) (ReportingClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(
		ctx,
		target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		log.Warn("Could not establish immediate blocking connection to Reporting Engine (lazy connection fallback)",
			zap.String("target", target),
			zap.Error(err),
		)
		// Non-blocking fallback
		conn, err = grpc.Dial(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return nil, err
		}
	}

	cb := circuitbreaker.NewCircuitBreaker(circuitbreaker.Config{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		Timeout:          5 * time.Second,
	})

	return &reportingClient{
		conn:   conn,
		client: reportingv1.NewUsageReportingServiceClient(conn),
		cb:     cb,
		log:    log.Named("reporting_client"),
	}, nil
}

func (c *reportingClient) GetUsageSummary(
	ctx context.Context,
	orgID, periodStart, periodEnd string,
) (*reportingv1.GetUsageSummaryResponse, error) {
	var resp *reportingv1.GetUsageSummaryResponse
	err := c.cb.Execute(func() error {
		var callErr error
		resp, callErr = c.client.GetUsageSummary(ctx, &reportingv1.GetUsageSummaryRequest{
			OrgId:       orgID,
			PeriodStart: periodStart,
			PeriodEnd:   periodEnd,
		})
		return callErr
	})
	if err != nil {
		c.log.Warn("Reporting client GetUsageSummary failed via circuit breaker", zap.Error(err))
		return nil, err
	}
	return resp, nil
}

func (c *reportingClient) GetCurrentMonthUsage(
	ctx context.Context,
	orgID, workspaceID string,
) (*reportingv1.GetCurrentMonthUsageResponse, error) {
	var resp *reportingv1.GetCurrentMonthUsageResponse
	err := c.cb.Execute(func() error {
		var callErr error
		resp, callErr = c.client.GetCurrentMonthUsage(ctx, &reportingv1.GetCurrentMonthUsageRequest{
			OrgId:       orgID,
			WorkspaceId: workspaceID,
		})
		return callErr
	})
	if err != nil {
		c.log.Warn("Reporting client GetCurrentMonthUsage failed via circuit breaker", zap.Error(err))
		return nil, err
	}
	return resp, nil
}

func (c *reportingClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
