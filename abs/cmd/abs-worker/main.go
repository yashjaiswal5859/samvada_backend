// =============================================================================
// Samvada Platform — Standalone Async Kafka Worker Server
// =============================================================================
// Dedicated background worker process consuming all platform Kafka topics:
//   1. s2s.call.completed    -> Real-Time Billing & Wallet Deduction Worker
//   2. abs.campaign.dispatch -> Outbound Campaign Call Dispatcher (to S2S)
//   3. abs.webhook.event     -> Customer Webhook Delivery & Retries
//   4. auth.email.send       -> OTP & Transactional Email Notification Worker
// =============================================================================

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"go.uber.org/zap"

	"github.com/samvada/abs/configs"
	"github.com/samvada/abs/internal/application/billing"
	"github.com/samvada/abs/internal/application/organization"
	"github.com/samvada/abs/internal/grpc/client"
	"github.com/samvada/abs/internal/infrastructure/postgres"
	"github.com/samvada/abs/internal/infrastructure/postgres/repos"
	"github.com/samvada/abs/internal/infrastructure/redis"
	"github.com/samvada/abs/internal/workers"
	"github.com/samvada/abs/pkg/logger"
)

func loadEnvFile(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			k := strings.TrimSpace(parts[0])
			v := strings.Trim(strings.TrimSpace(parts[1]), "\"'")
			if os.Getenv(k) == "" {
				os.Setenv(k, v)
			}
		}
	}
}

func printWorkerBanner(cfg *configs.Config, log *zap.Logger) {
	banner := `
===================================================================
   ___                                 _         __      __       _             
  / __| __ _ _ __ __   ____ _  __| |__ _   \ \    / /___ _| |_____ _ _ 
  \__ \/ _' | '  \' \ / / _' |/ _' / _' |   \ \/\/ // _ \ '_/ / -_) '_|
  |___/\__,_|_|_|_|_|\_/ \__,_|\__,_\__,_|    \_/\_/ \___/_|  \_\___|_|  
  Unified Async Kafka Worker Server
===================================================================
  Consumers Initialized:
    [1] s2s.call.completed    -> Real-time Billing & Wallet Deductions
    [2] abs.campaign.dispatch -> Outbound Campaign S2S Call Dispatch
    [3] abs.webhook.event     -> Customer HMAC-Signed Webhook Delivery
    [4] auth.email.send       -> OTP Verification & Email Worker
===================================================================`
	fmt.Println(banner)
	log.Info("Starting Samvada Standalone Worker Engine",
		zap.String("environment", cfg.App.Env),
		zap.Strings("kafka_brokers", cfg.Kafka.Brokers),
		zap.String("db_name", cfg.Database.DBName),
	)
}

func main() {
	// 0. Load .env if present
	loadEnvFile(".env")
	loadEnvFile("../../.env")

	// 1. Load Configuration
	cfg, err := configs.Load("configs/config.yaml")
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	// 2. Initialize Structured Logger
	log := logger.Init(cfg.App.Env, cfg.App.LogLevel)
	defer log.Sync()

	printWorkerBanner(cfg, log)

	// 3. PostgreSQL Connection Pool
	db, err := postgres.NewConnection(&cfg.Database, log)
	if err != nil {
		log.Fatal("Failed to connect to PostgreSQL database", zap.Error(err))
	}
	sqlDB, err := db.DB()
	if err == nil {
		defer sqlDB.Close()
	}

	// 4. Redis Connection Pool
	rdb, err := redis.NewClient(&cfg.Redis, log)
	if err != nil {
		log.Warn("Failed to connect to Redis (continuing without cache)", zap.Error(err))
	} else {
		defer rdb.Close()
	}

	// 5. Initialize Repositories
	billingRepo := repos.NewBillingRepository(db)
	campaignRepo := repos.NewCampaignRepository(db)
	integrationRepo := repos.NewIntegrationRepository(db)
	orgRepo := repos.NewOrgRepository(db)
	wsRepo := repos.NewWorkspaceRepository(db)
	profileRepo := repos.NewProfileRepository(db)

	// 6. Application Services
	walletService := billing.NewWalletService(billingRepo, log)
	orgService := organization.NewOrgService(orgRepo, wsRepo, billingRepo, log)

	// 7. S2S gRPC Client for outbound call dispatch
	s2sClient, err := client.NewS2SClient(cfg.S2S.GRPCAddress, log)
	if err != nil {
		log.Warn("Failed to connect to S2S engine gRPC (dispatching in fallback mode)", zap.Error(err))
	} else {
		defer s2sClient.Close()
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 8. Start All Kafka Consumers

	// 8.1 Consumer 1: Real-time Billing Worker (s2s.call.completed)
	billingWorker, err := workers.NewBillingWorker(
		cfg.Kafka.Brokers,
		"abs-billing-worker-group",
		"s2s.call.completed",
		walletService,
		campaignRepo,
		log,
	)
	if err != nil {
		log.Error("Failed to initialize Billing Worker", zap.Error(err))
	} else {
		billingWorker.Start(ctx)
		defer billingWorker.Stop()
	}

	// 8.2 Consumer 2: Campaign Dispatch Worker (abs.campaign.dispatch)
	campaignConsumer, err := workers.NewCampaignDispatchConsumer(
		cfg.Kafka.Brokers,
		"abs-campaign-worker-group",
		"abs.campaign.dispatch",
		s2sClient,
		campaignRepo,
		log,
	)
	if err != nil {
		log.Error("Failed to initialize Campaign Dispatch Consumer", zap.Error(err))
	} else {
		campaignConsumer.Start(ctx)
		defer campaignConsumer.Stop()
	}

	// 8.3 Consumer 3: Webhook Delivery Worker (abs.webhook.event)
	webhookConsumer, err := workers.NewWebhookConsumer(
		cfg.Kafka.Brokers,
		"abs-webhook-worker-group",
		"abs.webhook.event",
		integrationRepo,
		log,
	)
	if err != nil {
		log.Error("Failed to initialize Webhook Consumer", zap.Error(err))
	} else {
		webhookConsumer.Start(ctx)
		defer webhookConsumer.Stop()
	}

	// 8.4 Consumer 4: Email & OTP Notification Worker (auth.email.send)
	emailConsumer, err := workers.NewEmailConsumer(
		cfg.Kafka.Brokers,
		"abs-email-worker-group",
		"auth.email.send",
		log,
	)
	if err != nil {
		log.Error("Failed to initialize Email Consumer", zap.Error(err))
	} else {
		emailConsumer.Start(ctx)
		defer emailConsumer.Stop()
	}

	// 8.5 Consumer 5: Tenant Provisioning Worker (auth.onboarding.completed, auth.user.registered)
	tenantConsumer, err := workers.NewTenantProvisionConsumer(
		cfg.Kafka.Brokers,
		"abs-tenant-worker-group",
		[]string{"auth.onboarding.completed", "auth.user.registered"},
		orgService,
		orgRepo,
		wsRepo,
		profileRepo,
		log,
	)
	if err != nil {
		log.Error("Failed to initialize Tenant Provision Consumer", zap.Error(err))
	} else {
		tenantConsumer.Start(ctx)
		defer tenantConsumer.Stop()
	}

	log.Info("All background Kafka workers running successfully. Waiting for events...")

	// 9. Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutdown signal received, shutting down Kafka workers gracefully...")
	cancel()
	log.Info("All Kafka workers stopped cleanly. Exit successful.")
}
