// =============================================================================
// Samvada Platform — Application & Billing Service (ABS)
// =============================================================================
// Binary: abs-server
// Ports: :8082 (REST/HTTP), :9082 (gRPC)
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
	"database/sql"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/samvada/abs/configs"
	"github.com/samvada/abs/internal/api/handlers"
	"github.com/samvada/abs/internal/api/middleware"
	"github.com/samvada/abs/internal/api/routes"
	"github.com/samvada/abs/internal/application/admin"
	"github.com/samvada/abs/internal/application/agent"
	"github.com/samvada/abs/internal/application/billing"
	"github.com/samvada/abs/internal/application/campaign"
	"github.com/samvada/abs/internal/application/catalog"
	"github.com/samvada/abs/internal/application/kb"
	"github.com/samvada/abs/internal/application/organization"
	"github.com/samvada/abs/internal/application/telephony"
	"github.com/samvada/abs/internal/application/webhook"
	"github.com/samvada/abs/internal/application/workspace"
	connectorApp "github.com/samvada/abs/internal/application/integration"
	"github.com/samvada/abs/internal/application/audit"
	"github.com/samvada/abs/internal/application/notification"
	"github.com/samvada/abs/internal/application/profile"
	"github.com/samvada/abs/internal/grpc/client"
	"github.com/samvada/abs/internal/grpc/server"
	"github.com/samvada/abs/internal/infrastructure/kafka"
	"github.com/samvada/abs/internal/infrastructure/keto"
	"github.com/samvada/abs/internal/infrastructure/postgres"
	"github.com/samvada/abs/internal/infrastructure/postgres/repos"
	"github.com/samvada/abs/internal/infrastructure/redis"
	"github.com/samvada/abs/internal/infrastructure/payment"
	"github.com/samvada/abs/internal/infrastructure/storage"
	infraTelephony "github.com/samvada/abs/internal/infrastructure/telephony"
	"github.com/samvada/abs/internal/workers"
	"github.com/samvada/abs/pkg/logger"
	absv1 "github.com/samvada/proto/gen/go/abs/v1"
)



func main() {
	// 1. Load Configuration
	cfg, err := configs.Load("configs/config.yaml")
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	// 2. Initialize Enterprise Structured Logger
	log := logger.Init(cfg.App.Env, cfg.App.LogLevel)
	defer log.Sync()

	printBanner(cfg, log)

	// 3. Initialize Infrastructure Adapters
	// 3.1 PostgreSQL (GORM)
	db, err := postgres.NewConnection(&cfg.Database, log)
	if err != nil {
		log.Fatal("Failed to initialize PostgreSQL connection", zap.Error(err))
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()

	// 3.2 Redis
	rdb, err := redis.NewClient(&cfg.Redis, log)
	if err != nil {
		log.Fatal("Failed to initialize Redis client", zap.Error(err))
	}
	defer rdb.Close()

	// 3.3 Kafka Producer
	kafkaProducer, err := kafka.NewProducer(cfg.Kafka.Brokers, log)
	if err != nil {
		log.Warn("Kafka broker unavailable at startup (running in degraded mode)", zap.Error(err))
	} else {
		defer kafkaProducer.Close()
	}

	// 3.4 Storage
	localStorage, err := storage.NewLocalStorage(cfg.Storage.LocalPath, fmt.Sprintf("http://localhost:%d", cfg.App.HTTPPort), log)
	if err != nil {
		log.Fatal("Failed to initialize local storage", zap.Error(err))
	}

	var appStorage storage.Storage = localStorage
	if cfg.Storage.Provider == "s3" {
		s3Store, s3Err := storage.NewS3Storage(
			cfg.Storage.S3Endpoint,
			cfg.Storage.S3Bucket,
			cfg.Storage.S3AccessKey,
			cfg.Storage.S3SecretKey,
			cfg.Storage.S3PublicURL,
			cfg.Storage.S3UseSSL,
			log,
		)
		if s3Err != nil {
			log.Warn("Failed to initialize S3 storage, falling back to local storage", zap.Error(s3Err))
		} else {
			appStorage = s3Store
			log.Info("Initialized S3/MinIO storage for Knowledge Base",
				zap.String("endpoint", cfg.Storage.S3Endpoint),
				zap.String("bucket", cfg.Storage.S3Bucket),
			)
		}
	}

	// 4. Setup Gin HTTP Server
	if cfg.App.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.CorrelationAndLoggingMiddleware(log))
	router.Use(corsMiddleware())

	authMiddleware := middleware.NewAuthMiddleware(cfg, log)
	workspaceMiddleware := middleware.NewWorkspaceMiddleware(db, rdb, cfg.App.Env, log)

	registerBaseRoutes(router, cfg, sqlDB, rdb, authMiddleware, workspaceMiddleware)

	// 5. Initialize Repositories
	orgRepo := repos.NewOrgRepository(db)
	wsRepo := repos.NewWorkspaceRepository(db)
	agentRepo := repos.NewAgentRepository(db)
	billingRepo := repos.NewBillingRepository(db)
	kbRepo := repos.NewKBRepository(db)
	campaignRepo := repos.NewCampaignRepository(db)
	telephonyRepo := repos.NewTelephonyRepository(db)
	integrationRepo := repos.NewIntegrationRepository(db)
	profileRepo := repos.NewProfileRepository(db)
	notificationRepo := repos.NewNotificationRepository(db)
	connectorRepo := repos.NewConnectorRepository(db)
	auditRepo := repos.NewAuditRepository(db)
	catalogRepo := repos.NewCatalogRepository(db)

	// 6. Initialize Storage & Async Workers
	docProcessor := kb.NewDocumentProcessor(kbRepo, appStorage, log, 4)
	docProcessor.Start(context.Background())

	// 6.1 Initialize Telephony Carrier Adapters
	twilioAdapter := infraTelephony.NewTwilioAdapter(os.Getenv("TWILIO_ACCOUNT_SID"), os.Getenv("TWILIO_AUTH_TOKEN"), log)
	plivoAdapter := infraTelephony.NewPlivoAdapter(os.Getenv("PLIVO_AUTH_ID"), os.Getenv("PLIVO_AUTH_TOKEN"), log)
	carriers := map[string]infraTelephony.CarrierAdapter{
		"twilio": twilioAdapter,
		"plivo":  plivoAdapter,
	}

	// 6.2 Initialize Payment Gateways (Stripe + Razorpay)
	stripeGateway := payment.NewStripeGateway(os.Getenv("STRIPE_SECRET_KEY"), os.Getenv("STRIPE_WEBHOOK_SECRET"), log)
	razorpayGateway := payment.NewRazorpayGateway(os.Getenv("RAZORPAY_KEY_ID"), os.Getenv("RAZORPAY_KEY_SECRET"), os.Getenv("RAZORPAY_WEBHOOK_SECRET"), log)
	paymentGateways := map[string]payment.PaymentGateway{
		"stripe":   stripeGateway,
		"razorpay": razorpayGateway,
	}

	// 6.3 Optional Reporting Client for ClickHouse Analytics & Usage Metering
	reportingClient, err := client.NewReportingClient("localhost:9083", log)
	if err != nil {
		log.Warn("Reporting Client not available at localhost:9083 (metering running in fallback mode)", zap.Error(err))
	} else {
		defer reportingClient.Close()
	}

	// 6.4 Initialize Application Services
	orgService := organization.NewOrgService(orgRepo, wsRepo, billingRepo, log)
	wsService := workspace.NewWorkspaceService(wsRepo, log)
	agentService := agent.NewAgentService(agentRepo, wsRepo, log)
	walletService := billing.NewWalletService(billingRepo, log)
	subService := billing.NewSubscriptionService(billingRepo, orgRepo, paymentGateways, log)
	meteringService := billing.NewUsageMeteringService(billingRepo, reportingClient, log)
	invoiceService := billing.NewInvoiceService(billingRepo, orgRepo, reportingClient, paymentGateways, localStorage, log)
	kbService := kb.NewKBService(kbRepo, appStorage, docProcessor, log)
	if kafkaProducer != nil {
		kbService.SetKafkaProducer(kafkaProducer)
	}
	campaignService := campaign.NewCampaignService(campaignRepo, log)
	telephonyService := telephony.NewTelephonyService(telephonyRepo, carriers, log)
	integrationService := webhook.NewIntegrationService(integrationRepo, log)
	profileService := profile.NewProfileService(profileRepo, notificationRepo, orgRepo, wsRepo, localStorage, log)
	notificationService := notification.NewNotificationService(notificationRepo, log)
	connectorService := connectorApp.NewConnectorService(connectorRepo, log)
	auditService := audit.NewAuditService(auditRepo, log)
	catalogService := catalog.NewCatalogService(catalogRepo, log)

	// 6.2 Initialize Keto ReBAC & Admin Services
	ketoClient := keto.NewKetoClient(cfg.Keto.ReadURL, cfg.Keto.WriteURL, log)
	adminService := admin.NewAdminService(cfg, ketoClient, rdb, log)
	adminHandler := handlers.NewAdminHandler(adminService)
	adminAuthMW := middleware.NewAdminAuthMiddleware(adminService, ketoClient, cfg.Auth.JWTSecret, authMiddleware, log)

	// 7. Initialize HTTP Handlers
	orgHandler := handlers.NewOrgHandler(orgService, walletService, profileRepo)
	wsHandler := handlers.NewWorkspaceHandler(wsService)
	agentHandler := handlers.NewAgentHandler(agentService, wsService, walletService, rdb, cfg.Auth.JWTSecret)
	agentHandler.SetBuilderDependencies(catalogService, kbService, connectorService, telephonyService)
	billingHandler := handlers.NewBillingHandler(walletService, wsService, subService, meteringService, invoiceService)
	kbHandler := handlers.NewKBHandler(kbService)
	campaignHandler := handlers.NewCampaignHandler(campaignService)
	telephonyHandler := handlers.NewTelephonyHandler(telephonyService)
	webhookHandler := handlers.NewWebhookHandler(integrationService)
	profileHandler := handlers.NewProfileHandler(profileService, log)
	notificationHandler := handlers.NewNotificationHandler(notificationService, log)
	connectorHandler := handlers.NewConnectorHandler(connectorService, log)
	auditHandler := handlers.NewAuditHandler(auditService, log)
	catalogHandler := handlers.NewCatalogHandler(catalogService)

	// 8. Register All REST API Routes
	routes.RegisterAPIRoutes(router, routes.RouterDependencies{
		AuthMW:              authMiddleware,
		WorkspaceMW:         workspaceMiddleware,
		OrgHandler:          orgHandler,
		WorkspaceHandler:    wsHandler,
		AgentHandler:        agentHandler,
		BillingHandler:      billingHandler,
		KBHandler:           kbHandler,
		CampaignHandler:     campaignHandler,
		TelephonyHandler:    telephonyHandler,
		WebhookHandler:      webhookHandler,
		ProfileHandler:      profileHandler,
		NotificationHandler: notificationHandler,
		ConnectorHandler:    connectorHandler,
		AuditHandler:        auditHandler,
		CatalogHandler:      catalogHandler,
		AdminHandler:        adminHandler,
		AdminAuthMW:         adminAuthMW,
		RDB:                 rdb,
	})

	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.App.HTTPPort),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// 5. Start HTTP Server in background
	go func() {
		log.Info("HTTP server listening", zap.String("addr", httpServer.Addr))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("HTTP server fatal error", zap.Error(err))
		}
	}()

	// 6. Setup and Start gRPC Server in background
	grpcListener, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.App.GRPCPort))
	if err != nil {
		log.Fatal("Failed to bind gRPC listener", zap.Error(err))
	}

	grpcServer := grpc.NewServer()
	runtimeServer := server.NewAgentRuntimeServer(agentService, wsService, walletService, log)
	absv1.RegisterAgentRuntimeServiceServer(grpcServer, runtimeServer)
	reflection.Register(grpcServer)

	go func() {
		log.Info("gRPC server listening", zap.String("addr", grpcListener.Addr().String()))
		if err := grpcServer.Serve(grpcListener); err != nil {
			log.Fatal("gRPC server fatal error", zap.Error(err))
		}
	}()
	// 6.2 Setup and Start Outbound Campaign Runner (Kafka Producer only)
	campaignRunner := workers.NewCampaignRunner(campaignRepo, kafkaProducer, log)
	campaignRunner.Start(context.Background())
	defer campaignRunner.Stop()

	// 6.3 Setup and Start Tenant Provision Consumer (auth.onboarding.completed, auth.user.registered)
	tenantConsumer, err := workers.NewTenantProvisionConsumer(
		cfg.Kafka.Brokers,
		"abs-server-tenant-worker-group",
		[]string{"auth.onboarding.completed", "auth.user.registered"},
		orgService,
		orgRepo,
		wsRepo,
		profileRepo,
		log,
	)
	if err != nil {
		log.Warn("Failed to initialize Tenant Provision Consumer in abs-server", zap.Error(err))
	} else {
		tenantConsumer.Start(context.Background())
		defer tenantConsumer.Stop()
	}

	// 7. Graceful Shutdown Handler
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutdown signal received, shutting down gracefully...")

	// Gracefully stop gRPC
	grpcServer.GracefulStop()
	log.Info("gRPC server stopped")

	// Gracefully stop HTTP
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Error("HTTP server forced shutdown", zap.Error(err))
	} else {
		log.Info("HTTP server stopped gracefully")
	}

	log.Info("Samvada ABS service stopped")
}

func printBanner(cfg *configs.Config, log *zap.Logger) {
	banner := `
===================================================================
   ___                                 _         ___  ____  ____  
  / __| __ _ _ __ __   ____ _  __| |__ _   /   \| __|/ ___| 
  \__ \/ _' | '  \' \ / / _' |/ _' / _' | |  _  |__ \ \___ \ 
  |___/\__,_|_|_|_|_|\_/ \__,_|\__,_\__,_| |_| |_|___/|____/ 
  Application & Billing Service (ABS) — Core Microservice
===================================================================`
	fmt.Println(banner)
	log.Info("Starting Samvada ABS service",
		zap.String("environment", cfg.App.Env),
		zap.Int("http_port", cfg.App.HTTPPort),
		zap.Int("grpc_port", cfg.App.GRPCPort),
		zap.String("db_name", cfg.Database.DBName),
	)
}


func corsMiddleware() gin.HandlerFunc {
	allowedOriginsEnv := os.Getenv("ALLOWED_ORIGINS")
	var allowedOrigins []string
	if allowedOriginsEnv != "" {
		for _, o := range strings.Split(allowedOriginsEnv, ",") {
			if trimmed := strings.TrimSpace(o); trimmed != "" {
				allowedOrigins = append(allowedOrigins, trimmed)
			}
		}
	} else {
		allowedOrigins = []string{
			"http://localhost:5173",
			"http://localhost:3000",
			"http://127.0.0.1:5173",
			"http://localhost:8080",
		}
	}

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		allowOrigin := ""

		if origin != "" {
			for _, allowed := range allowedOrigins {
				if allowed == "*" || strings.EqualFold(allowed, origin) {
					allowOrigin = origin
					break
				}
			}
			// In non-production, allow localhost and 127.0.0.1 on any port for flexibility
			if allowOrigin == "" && os.Getenv("APP_ENV") != "production" {
				if strings.HasPrefix(origin, "http://localhost:") || strings.HasPrefix(origin, "http://127.0.0.1:") {
					allowOrigin = origin
				}
			}
		}

		if allowOrigin != "" {
			c.Writer.Header().Set("Access-Control-Allow-Origin", allowOrigin)
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
			c.Writer.Header().Set("Vary", "Origin")
		} else if origin == "" {
			c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		}

		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With, X-Correlation-ID, X-Workspace-ID, X-Org-ID")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, PATCH, DELETE")
		c.Writer.Header().Set("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

func registerBaseRoutes(r *gin.Engine, cfg *configs.Config, db *sql.DB, rdb *redis.Client, authMW *middleware.AuthMiddleware, wsMW *middleware.WorkspaceMiddleware) {
	// Liveness probe (Kubernetes / Docker)
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "ok",
			"service":   cfg.App.Name,
			"env":       cfg.App.Env,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	})

	// Readiness probe — checks live PostgreSQL & Redis connections
	r.GET("/ready", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		dbStatus := "ok"
		if err := db.PingContext(ctx); err != nil {
			dbStatus = fmt.Sprintf("unhealthy: %v", err)
		}

		redisStatus := "ok"
		if err := rdb.Ping(ctx); err != nil {
			redisStatus = fmt.Sprintf("unhealthy: %v", err)
		}

		status := "ready"
		httpCode := http.StatusOK
		if dbStatus != "ok" || redisStatus != "ok" {
			status = "degraded"
			httpCode = http.StatusServiceUnavailable
		}

		c.JSON(httpCode, gin.H{
			"status":    status,
			"service":   cfg.App.Name,
			"database":  dbStatus,
			"redis":     redisStatus,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	})

	// API v1 root health
	v1 := r.Group("/api/v1")
	{
		v1.GET("/ping", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"message":   "pong",
				"service":   "samvada-abs",
				"timestamp": time.Now().UTC(),
			})
		})

		// Protected probe routes to verify auth & tenant context middleware
		protected := v1.Group("")
		protected.Use(authMW.RequireAuth())
		{
			protected.GET("/auth/me", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{
					"success": true,
					"user_id": c.GetString(middleware.ContextUserIDKey),
					"email":   c.GetString(middleware.ContextUserEmailKey),
				})
			})

			tenant := protected.Group("")
			tenant.Use(wsMW.RequireWorkspace())
			{
				tenant.GET("/tenant/context", func(c *gin.Context) {
					c.JSON(http.StatusOK, gin.H{
						"success":        true,
						"workspace_id":   middleware.GetWorkspaceID(c),
						"workspace_role": middleware.GetWorkspaceRole(c),
						"user_id":        c.GetString(middleware.ContextUserIDKey),
					})
				})
			}
		}
	}
}
