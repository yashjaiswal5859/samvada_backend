package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/ridesafety/auth-service/internal/api/routes"
	"go.uber.org/zap"
)

type Server struct {
	container *Container
	logger    *zap.Logger
}

func NewServer(container *Container, logger *zap.Logger) *Server {
	return &Server{
		container: container,
		logger:    logger,
	}
}

func (s *Server) Start() error {
	cfg := s.container.Config
	router := routes.SetupRouter(
		s.logger,
		s.container.RedisProvider,
		s.container.Orchestrator,
		s.container.LoginHandler,
		s.container.LogoutHandler,
		s.container.RefreshHandler,
		s.container.HealthHandler,
		s.container.UserHandler,
		s.container.OAuthHandler,
		s.container.GoogleHandler,
		s.container.MeHandler,
		s.container.OnboardingHandler,
		cfg.FrontendURL,
		cfg.IsProduction,
	)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      router,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go s.container.CleanupWorker.Start(ctx)
	go s.container.OutboxPublisher.Start(ctx)

	go func() {
		s.logger.Info("Starting HTTP Server", zap.Int("port", cfg.Port))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Fatal("HTTP server failed to listen and serve", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	s.logger.Info("Shutting down server...")

	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		s.logger.Error("HTTP Server forced to shutdown", zap.Error(err))
	} else {
		s.logger.Info("HTTP Server gracefully stopped")
	}

	s.container.Close()
	s.logger.Info("DI resources cleaned up. Exit successful.")

	return nil
}
