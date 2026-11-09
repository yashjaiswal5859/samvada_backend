package main

import (
	"log"

	"github.com/ridesafety/auth-service/internal/bootstrap"
	"go.uber.org/zap"
)

func main() {
	// 1. Initialize Logger
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("Failed to initialize Zap logger: %v", err)
	}
	defer logger.Sync()

	logger.Info("Starting Auth Service...")

	// 2. Load Config
	cfg, err := bootstrap.LoadConfig()
	if err != nil {
		logger.Fatal("Failed to load configuration", zap.Error(err))
	}

	// 3. Build DI Container
	container, err := bootstrap.NewContainer(cfg, logger)
	if err != nil {
		logger.Fatal("Failed to build dependency container", zap.Error(err))
	}

	// 4. Start Server (gracefully handles shutdown)
	server := bootstrap.NewServer(container, logger)
	if err := server.Start(); err != nil {
		logger.Fatal("Server startup failed", zap.Error(err))
	}
}
