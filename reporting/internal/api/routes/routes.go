// =============================================================================
// Samvada Platform — Reporting Engine REST Route Registration
// =============================================================================

package routes

import (
	"github.com/gin-gonic/gin"

	"github.com/samvada/reporting/internal/api/handlers"
	"github.com/samvada/reporting/internal/api/middleware"
)

type RouterDependencies struct {
	AuthMW           *middleware.AuthMiddleware
	AnalyticsHandler *handlers.AnalyticsHandler
	CallsHandler     *handlers.CallsHandler
}

func RegisterAPIRoutes(r *gin.Engine, deps RouterDependencies) {
	r.Use(middleware.PrometheusMetrics())
	r.GET("/metrics", middleware.MetricsHandler())
	r.Use(middleware.RequestBodyLimit(middleware.DefaultBodyLimit))

	v1 := r.Group("/api/v1")
	v1.Use(deps.AuthMW.RequireAuth())
	{
		// 1. Analytics & Business Intelligence
		analytics := v1.Group("/analytics")
		{
			analytics.GET("/summary", deps.AnalyticsHandler.GetSummary)
			analytics.GET("/volume-trend", deps.AnalyticsHandler.GetVolumeTrend)
			analytics.GET("/agents/leaderboard", deps.AnalyticsHandler.GetLeaderboard)
			analytics.GET("/funnels", deps.AnalyticsHandler.GetFunnels)
			analytics.GET("/live-calls", deps.AnalyticsHandler.GetLiveCalls)
		}

		// 2. Call Detail Records (CDR)
		calls := v1.Group("/calls")
		{
			calls.GET("", deps.CallsHandler.ListCalls)
			calls.GET("/:call_id", deps.CallsHandler.GetCall)
		}
	}
}
