package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ridesafety/auth-service/internal/platform/cache"
	"gorm.io/gorm"
)

type HealthHandler struct {
	db    *gorm.DB
	redis cache.CacheProvider
}

func NewHealthHandler(db *gorm.DB, redis cache.CacheProvider) *HealthHandler {
	return &HealthHandler{
		db:    db,
		redis: redis,
	}
}

func (h *HealthHandler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "UP"})
}

func (h *HealthHandler) Live(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "UP"})
}

func (h *HealthHandler) Ready(c *gin.Context) {
	sqlDB, err := h.db.DB()
	if err != nil || sqlDB.Ping() != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "DOWN", "reason": "database unreachable"})
		return
	}

	_, err = h.redis.Exists(c.Request.Context(), "ping")
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "DOWN", "reason": "redis unreachable"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "UP"})
}
