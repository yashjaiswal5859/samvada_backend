package db

import (
	"context"
	"log"
	"strings"
	"time"

	"voice-agent-go/internal/config"
	"voice-agent-go/internal/models"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var (
	DB          *gorm.DB
	RedisClient *redis.Client
	KafkaWriter *kafka.Writer
)

func InitPostgres() {
	var err error
	dsn := config.Settings.DatabaseUrl
	dsn = strings.ReplaceAll(dsn, "postgresql+asyncpg://", "postgres://")
	dsn = strings.ReplaceAll(dsn, "postgres+asyncpg://", "postgres://")

	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Printf("[DB] Warning: Failed to connect to PostgreSQL: %v. Running without DB persistence.", err)
		DB = nil
		return
	}

	// Auto-Migrate Models
	err = DB.AutoMigrate(
		&models.Tenant{},
		&models.User{},
		&models.Call{},
		&models.ConversationHistory{},
		&models.EmotionTimeline{},
		&models.ToolExecution{},
		&models.CustomerProfile{},
		&models.KnowledgeDocument{},
		&models.CustomerMemory{},
		&models.EscalationRecord{},
	)
	if err != nil {
		log.Printf("[DB] Warning: Auto-Migration failed: %v", err)
	} else {
		log.Println("[DB] PostgreSQL database initialized & migrated successfully.")

		// Seed default tenant if not exists for 100% DB foreign key integrity
		tenantID, _ := uuid.Parse("00000000-0000-0000-0000-000000000001")
		var count int64
		DB.Model(&models.Tenant{}).Where("id = ?", tenantID).Count(&count)
		if count == 0 {
			defaultTenant := &models.Tenant{
				ID:   tenantID,
				Name: "Swiggy / HDFC Enterprise",
			}
			if err := DB.Create(defaultTenant).Error; err != nil {
				log.Printf("[DB] Note seeding default tenant: %v", err)
			} else {
				log.Println("[DB] Default tenant (Swiggy / HDFC) seeded in PostgreSQL.")
			}
		}
	}
}

func InitRedis() {
	opt, err := redis.ParseURL(config.Settings.RedisUrl)
	if err != nil {
		log.Printf("[Redis] Warning: Invalid Redis URL: %v. Running without Redis.", err)
		RedisClient = nil
		return
	}

	RedisClient = redis.NewClient(opt)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if _, err := RedisClient.Ping(ctx).Result(); err != nil {
		log.Printf("[Redis] Warning: Failed to connect to Redis: %v. Running without Redis.", err)
		RedisClient = nil
		return
	}

	log.Println("[Redis] Redis client initialized successfully.")
}

func InitKafka() {
	KafkaWriter = &kafka.Writer{
		Addr:     kafka.TCP(config.Settings.KafkaBootstrapServers),
		Topic:    "call.lifecycle",
		Balancer: &kafka.LeastBytes{},
	}
	log.Println("[Kafka] Kafka producer initialized successfully.")
}

func CloseDBs() {
	if RedisClient != nil {
		RedisClient.Close()
	}
	if KafkaWriter != nil {
		KafkaWriter.Close()
	}
}
