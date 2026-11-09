package bootstrap

import (
	"time"

	"github.com/ridesafety/auth-service/internal/api/handlers"
	applogin "github.com/ridesafety/auth-service/internal/application/login"
	applogout "github.com/ridesafety/auth-service/internal/application/logout"
	apprefresh "github.com/ridesafety/auth-service/internal/application/refresh"
	appuser "github.com/ridesafety/auth-service/internal/application/user"
	apponboarding "github.com/ridesafety/auth-service/internal/application/onboarding"
	"github.com/ridesafety/auth-service/internal/infrastructure/hydra"
	"github.com/ridesafety/auth-service/internal/infrastructure/kafka"
	"github.com/ridesafety/auth-service/internal/infrastructure/keto"
	"github.com/ridesafety/auth-service/internal/infrastructure/kratos"
	"github.com/ridesafety/auth-service/internal/infrastructure/postgres"
	infrarepo "github.com/ridesafety/auth-service/internal/infrastructure/postgres/repository"
	infraredis "github.com/ridesafety/auth-service/internal/infrastructure/redis"
	"github.com/ridesafety/auth-service/internal/platform/cache"
	"github.com/ridesafety/auth-service/internal/platform/eventbus"
	"github.com/ridesafety/auth-service/internal/platform/identity"
	"github.com/ridesafety/auth-service/internal/workers"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Container struct {
	Config            *Config
	Logger            *zap.Logger
	DB                *gorm.DB
	RedisProvider     cache.CacheProvider
	KafkaProducer     kafka.Producer
	Orchestrator      identity.IdentityOrchestrator
	EventBus          eventbus.EventBus
	LoginHandler      *handlers.LoginHandler
	LogoutHandler     *handlers.LogoutHandler
	RefreshHandler    *handlers.RefreshHandler
	HealthHandler     *handlers.HealthHandler
	UserHandler       *handlers.UserHandler
	OAuthHandler      *handlers.OAuthHandler
	GoogleHandler     *handlers.GoogleHandler
	MeHandler         *handlers.MeHandler
	OnboardingHandler *handlers.OnboardingHandler
	OutboxPublisher   *workers.OutboxPublisher
	CleanupWorker     *workers.CleanupWorker
}

func NewContainer(cfg *Config, logger *zap.Logger) (*Container, error) {
	dbCfg := postgres.Config{
		Host:            cfg.DBHost,
		Port:            cfg.DBPort,
		User:            cfg.DBUser,
		Password:        cfg.DBPassword,
		DBName:          cfg.DBName,
		SSLMode:         cfg.DBSSLMode,
		MaxOpenConns:    cfg.DBMaxOpenConns,
		MaxIdleConns:    cfg.DBMaxIdleConns,
		ConnMaxLifetime: cfg.DBConnMaxLifetime,
	}
	db, err := postgres.Connect(dbCfg)
	if err != nil {
		return nil, err
	}

	redisProvider := infraredis.NewRedisProvider(
		cfg.RedisHost,
		cfg.RedisPort,
		cfg.RedisPassword,
		cfg.RedisDB,
		cfg.RedisPoolSize,
	)

	kafkaProducer, err := kafka.NewKafkaProducer(cfg.KafkaBrokers, cfg.KafkaClientID)
	if err != nil {
		return nil, err
	}

	kratosProvider := kratos.NewKratosProvider(cfg.KratosAdminURL)
	hydraProvider := hydra.NewHydraProvider(
		cfg.HydraAdminURL,
		cfg.HydraPublicURL,
		cfg.HydraBrowserURL,
		cfg.HydraClientID,
		cfg.HydraClientSecret,
		cfg.HydraRedirectURI,
	)
	ketoProvider := keto.NewKetoProvider(cfg.KetoWriteURL, cfg.KetoReadURL)

	orchestrator := identity.NewIdentityOrchestrator(
		kratosProvider,
		hydraProvider,
		ketoProvider,
		db,
		cfg.JWTSecret,
		cfg.FrontendURL,
	)

	userRepo := infrarepo.NewUserRepository(db)
	pendingAuthRepo := infrarepo.NewPendingAuthRepository(db)
	oauthSessionRepo := infrarepo.NewOAuthSessionRepository(db)
	outboxRepo := infrarepo.NewOutboxRepository(db)
	onboardingRepo := infrarepo.NewOnboardingRepository(db)

	eventBus := eventbus.NewOutboxEventBus(db)

	onboardingService := apponboarding.NewService(onboardingRepo, eventBus, kafkaProducer, cfg.KafkaTopicTenantProvision)

	loginService := applogin.NewService(
		db,
		userRepo,
		pendingAuthRepo,
		eventBus,
		orchestrator,
		cfg.OTPExpiry,
		cfg.MaxAttempts,
	)

	logoutService := applogout.NewService(db, eventBus, orchestrator)
	refreshService := apprefresh.NewService(orchestrator)
	userService := appuser.NewService(userRepo, orchestrator)

	loginHandler := handlers.NewLoginHandler(loginService, orchestrator, userRepo, oauthSessionRepo, redisProvider, cfg.IsProduction)
	logoutHandler := handlers.NewLogoutHandler(logoutService, cfg.IsProduction)
	refreshHandler := handlers.NewRefreshHandler(refreshService, redisProvider, cfg.IsProduction)
	healthHandler := handlers.NewHealthHandler(db, redisProvider)
	userHandler := handlers.NewUserHandler(userService)

	oauthHandler := handlers.NewOAuthHandler(
		oauthSessionRepo,
		pendingAuthRepo,
		orchestrator,
		loginService,
		userRepo,
		redisProvider,
		cfg.HydraBrowserURL,
		cfg.HydraClientID,
		cfg.HydraRedirectURI,
		cfg.FrontendURL,
		cfg.IsProduction,
		cfg.AllowedRedirectDomains,
	)

	googleHandler := handlers.NewGoogleHandler(
		cfg.GoogleClientID,
		cfg.GoogleClientSecret,
		cfg.GoogleRedirectURI,
		orchestrator,
		userRepo,
		db,
		eventBus,
		cfg.IsProduction,
	)

	meHandler := handlers.NewMeHandler(orchestrator, userRepo, onboardingService, cfg.IsProduction)
	onboardingHandler := handlers.NewOnboardingHandler(onboardingService, orchestrator, userRepo, cfg.IsProduction)

	outboxPublisher := workers.NewOutboxPublisher(
		db,
		outboxRepo,
		kafkaProducer,
		logger,
		cfg.KafkaTopicOTPSend,
		cfg.KafkaTopicUserRegistered,
		cfg.KafkaTopicUserLoggedIn,
		cfg.KafkaTopicUserLoggedOut,
		cfg.KafkaTopicTenantProvision,
	)

	cleanupWorker := workers.NewCleanupWorker(
		db,
		pendingAuthRepo,
		outboxRepo,
		logger,
		15*time.Minute,
		24*time.Hour,
	)

	return &Container{
		Config:            cfg,
		Logger:            logger,
		DB:                db,
		RedisProvider:     redisProvider,
		KafkaProducer:     kafkaProducer,
		Orchestrator:      orchestrator,
		EventBus:          eventBus,
		LoginHandler:      loginHandler,
		LogoutHandler:     logoutHandler,
		RefreshHandler:    refreshHandler,
		HealthHandler:     healthHandler,
		UserHandler:       userHandler,
		OAuthHandler:      oauthHandler,
		GoogleHandler:     googleHandler,
		MeHandler:         meHandler,
		OnboardingHandler: onboardingHandler,
		OutboxPublisher:   outboxPublisher,
		CleanupWorker:     cleanupWorker,
	}, nil
}

func (c *Container) Close() {
	if c.KafkaProducer != nil {
		_ = c.KafkaProducer.Close()
	}
	if sqlDB, err := c.DB.DB(); err == nil {
		_ = sqlDB.Close()
	}
}
