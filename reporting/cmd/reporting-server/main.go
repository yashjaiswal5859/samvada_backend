// =============================================================================
// Samvada Platform — Reporting Engine Server Entrypoint
// =============================================================================

package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/samvada/reporting/configs"
	"github.com/samvada/reporting/internal/api/handlers"
	"github.com/samvada/reporting/internal/api/middleware"
	"github.com/samvada/reporting/internal/api/routes"
	"github.com/samvada/reporting/internal/application/analytics"
	"github.com/samvada/reporting/internal/application/calls"
	"github.com/samvada/reporting/internal/application/usage"
	grpcserver "github.com/samvada/reporting/internal/grpc/server"
	"github.com/samvada/reporting/internal/infrastructure/clickhouse"
	"github.com/samvada/reporting/internal/infrastructure/clickhouse/repos"
	"github.com/samvada/reporting/internal/infrastructure/postgres"
	"github.com/samvada/reporting/internal/infrastructure/redis"
	"github.com/samvada/reporting/internal/workers"
	"github.com/samvada/reporting/pkg/logger"
	reportingv1 "github.com/samvada/proto/gen/go/reporting/v1"
	"gorm.io/gorm"
)

func printBanner(cfg *configs.Config, log *zap.Logger) {
	fmt.Println(`
   _____                                 _         _____                       _   _             
  / ____|                               | |       |  __ \                     | | (_)            
 | (___   __ _ _ __ _____   ____ _  __| | __ _  | |__) |___ _ __   ___  _ __| |_ _ _ __   __ _ 
  \___ \ / _` + "`" + ` | '_ ` + "`" + ` _ \ \ / / _` + "`" + ` |/ _` + "`" + ` |/ _` + "`" + ` | |  _  // _ \ '_ \ / _ \| '__| __| | '_ \ / _` + "`" + ` |
  ____) | (_| | | | | | \ V / (_| | (_| | (_| | | | \ \  __/ |_) | (_) | |  | |_| | | | | (_| |
 |_____/ \__,_|_| |_| |_|\_/ \__,_|\__,_|\__,_| |_|  \_\___| .__/ \___/|_|   \__|_|_| |_|\__, |
                                                            | |                            __/ |
                                                            |_|                           |___/ 
	`)
	log.Info("Samvada Reporting Engine bootstrapping",
		zap.String("service", cfg.App.Name),
		zap.String("env", cfg.App.Env),
		zap.Int("http_port", cfg.App.HTTPPort),
		zap.Int("grpc_port", cfg.App.GRPCPort),
	)
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With, X-Correlation-ID, X-Request-ID, X-Workspace-ID, X-Org-ID")
			c.Header("Access-Control-Allow-Methods", "POST, HEAD, PATCH, OPTIONS, GET, PUT, DELETE")
		}
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func main() {
	// 1. Load Configuration
	cfg, err := configs.Load("configs/config.yaml")
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	// 2. Structured Logger
	log := logger.Init(cfg.App.Env, cfg.App.LogLevel)
	defer log.Sync()

	printBanner(cfg, log)

	// 3. Infrastructure Adapters
	// 3.1 ClickHouse Native Connection
	chConn, err := clickhouse.NewConnection(&cfg.ClickHouse, log)
	if err != nil {
		log.Fatal("Failed to connect to ClickHouse cluster", zap.Error(err))
	}
	defer chConn.Close()

	// 3.2 Redis
	rdb, err := redis.NewClient(&cfg.Redis, log)
	if err != nil {
		log.Warn("Redis connection unavailable (running without query cache)", zap.Error(err))
	} else {
		defer rdb.Close()
	}

	// 3.3 PostgreSQL (Relational Metadata & Cross-Engine Queries)
	var pgDB *gorm.DB
	if cfg.Database.Host != "" {
		pgDB, err = postgres.NewConnection(&cfg.Database, log)
		if err != nil {
			log.Warn("PostgreSQL connection unavailable (running in ClickHouse-only mode)", zap.Error(err))
		} else {
			sqlDB, err := pgDB.DB()
			if err == nil {
				defer sqlDB.Close()
			}
		}
	}
	_ = pgDB

	// 4. Repositories
	callRepo := repos.NewCallRepository(chConn, log)
	analyticsRepo := repos.NewAnalyticsRepository(chConn, log)
	usageRepo := repos.NewUsageRepository(chConn, log)

	// 5. Application Services
	analyticsService := analytics.NewAnalyticsService(analyticsRepo, rdb, log)
	meteringService := usage.NewMeteringService(usageRepo, log)
	callsService := calls.NewCallsService(callRepo, log)

	// 6. Kafka Ingestion Worker
	worker, err := workers.NewIngestionWorker(
		cfg.Kafka.Brokers,
		cfg.Kafka.ConsumerGroup,
		callRepo,
		rdb,
		&cfg.Kafka,
		&cfg.Batch,
		log,
	)
	if err != nil {
		log.Warn("Kafka broker unavailable at startup (running in degraded ingestion mode)", zap.Error(err))
	} else {
		workerCtx, workerCancel := context.WithCancel(context.Background())
		defer workerCancel()
		worker.Start(workerCtx)
		defer worker.Close()
	}

	// 7. Setup Gin HTTP Server
	if cfg.App.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.CorrelationAndLoggingMiddleware(log))
	router.Use(corsMiddleware())

	// Health Probes
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "UP", "service": "reporting-engine"})
	})
	router.GET("/ready", func(c *gin.Context) {
		if err := chConn.Ping(c.Request.Context()); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "DOWN", "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "READY", "clickhouse": "connected"})
	})

	authMiddleware := middleware.NewAuthMiddleware(cfg, log)
	analyticsHandler := handlers.NewAnalyticsHandler(analyticsService)
	callsHandler := handlers.NewCallsHandler(callsService)

	routes.RegisterAPIRoutes(router, routes.RouterDependencies{
		AuthMW:           authMiddleware,
		AnalyticsHandler: analyticsHandler,
		CallsHandler:     callsHandler,
	})

	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.App.HTTPPort),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Info("Reporting HTTP server listening", zap.String("addr", httpServer.Addr))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("HTTP server fatal error", zap.Error(err))
		}
	}()

	// 8. Setup gRPC Server
	grpcListener, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.App.GRPCPort))
	if err != nil {
		log.Fatal("Failed to bind gRPC listener", zap.Error(err))
	}

	grpcServer := grpc.NewServer()
	usageServer := grpcserver.NewUsageServer(meteringService, log)
	reportingv1.RegisterUsageReportingServiceServer(grpcServer, usageServer)
	reflection.Register(grpcServer)

	go func() {
		log.Info("Reporting gRPC server listening", zap.String("addr", grpcListener.Addr().String()))
		if err := grpcServer.Serve(grpcListener); err != nil {
			log.Fatal("gRPC server fatal error", zap.Error(err))
		}
	}()

	// 9. Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Reporting Engine shutting down gracefully...")
	grpcServer.GracefulStop()
	log.Info("gRPC server stopped")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Error("HTTP server forced shutdown", zap.Error(err))
	} else {
		log.Info("HTTP server stopped gracefully")
	}
}
