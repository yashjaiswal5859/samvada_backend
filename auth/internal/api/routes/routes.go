package routes

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ridesafety/auth-service/internal/api/handlers"
	"github.com/ridesafety/auth-service/internal/api/middleware"
	"github.com/ridesafety/auth-service/internal/platform/cache"
	"github.com/ridesafety/auth-service/internal/platform/identity"
	"github.com/ridesafety/auth-service/internal/web"
	"go.uber.org/zap"
)

func SetupRouter(
	logger *zap.Logger,
	cacheProvider cache.CacheProvider,
	orchestrator identity.IdentityOrchestrator,
	loginHandler *handlers.LoginHandler,
	logoutHandler *handlers.LogoutHandler,
	refreshHandler *handlers.RefreshHandler,
	healthHandler *handlers.HealthHandler,
	userHandler *handlers.UserHandler,
	oauthHandler *handlers.OAuthHandler,
	googleHandler *handlers.GoogleHandler,
	meHandler *handlers.MeHandler,
	onboardingHandler *handlers.OnboardingHandler,
	frontendURL string,
	isProduction bool,
) *gin.Engine {
	r := gin.New()

	r.Use(func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			allowed := false
			if frontendURL != "" && origin == frontendURL {
				allowed = true
			} else if !isProduction {
				// In local development, permit standard localhost/127.0.0.1 ports
				if strings.HasPrefix(origin, "http://localhost:") || strings.HasPrefix(origin, "http://127.0.0.1:") || strings.HasPrefix(origin, "https://localhost:") {
					allowed = true
				}
			}

			if allowed {
				c.Header("Access-Control-Allow-Origin", origin)
				c.Header("Access-Control-Allow-Credentials", "true")
				c.Header("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With, X-Correlation-ID")
				c.Header("Access-Control-Allow-Methods", "POST, HEAD, PATCH, OPTIONS, GET, PUT, DELETE")
			}
		}
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})

	r.Use(middleware.CorrelationID())
	r.Use(middleware.SecurityHeaders())
	r.Use(middleware.RequestLogger(logger))
	r.Use(middleware.Recovery(logger))

	// Serve static files embedded in the binary
	r.StaticFS("/static", http.FS(web.Assets))

	r.GET("/health", healthHandler.Health)
	r.GET("/ready", healthHandler.Ready)
	r.GET("/live", healthHandler.Live)

	api := r.Group("/api/v1/auth")
	{
		// OAuth2 Entry, Callback, Consent & UI views
		api.GET("/oauth/login",
			middleware.RateLimiter(cacheProvider, 5, 1*time.Minute, "oauth_login"),
			oauthHandler.OAuthLogin,
		)
		api.GET("/callback",
			middleware.RateLimiter(cacheProvider, 10, 1*time.Minute, "callback"),
			oauthHandler.Callback,
		)
		api.GET("/consent-challenge", oauthHandler.Consent)
		api.GET("/login-challenge", oauthHandler.LoginChallenge)
		api.GET("/login-view", oauthHandler.LoginView)
		api.GET("/verify/:requestId", oauthHandler.VerifyView)

		// Google SSO
		api.GET("/google/login", googleHandler.GoogleLogin)
		api.GET("/google/callback", googleHandler.GoogleCallback)
		
		// Unified OTP Verification
		api.POST("/verify",
			middleware.RateLimiter(cacheProvider, 10, 1*time.Minute, "otp_verify"),
			oauthHandler.VerifyOTP,
		)

		// Session profile
		api.GET("/me", meHandler.Me)

		api.POST("/login",
			middleware.RateLimiter(cacheProvider, 30, 1*time.Minute, "login"),
			loginHandler.Login,
		)
		api.POST("/login/verify",
			middleware.RateLimiter(cacheProvider, 30, 1*time.Minute, "login_verify"),
			loginHandler.VerifyLogin,
		)

		api.POST("/logout", logoutHandler.Logout)

		api.POST("/refresh",
			middleware.RateLimiter(cacheProvider, 30, 1*time.Minute, "refresh"),
			refreshHandler.Refresh,
		)

		api.DELETE("/users/:id", userHandler.DeleteUser)
	}

	secHandler := handlers.NewSecurityHandler(orchestrator)
	security := r.Group("/api/v1/security")
	{
		security.POST("/password", secHandler.UpdatePassword)
		security.POST("/2fa/generate", secHandler.Generate2FA)
		security.POST("/2fa/enable", secHandler.Enable2FA)
		security.POST("/2fa/disable", secHandler.Disable2FA)
		security.GET("/sessions", secHandler.ListSessions)
		security.DELETE("/sessions/other", secHandler.RevokeOtherSessions)
		security.DELETE("/sessions/:id", secHandler.RevokeSession)
	}

	onboardingApi := r.Group("/api/v1/onboarding")
	{
		onboardingApi.POST("/profile", onboardingHandler.SaveProfile)
		onboardingApi.POST("/industries", onboardingHandler.SaveIndustries)
		onboardingApi.GET("/state", onboardingHandler.GetState)
	}

	return r
}
