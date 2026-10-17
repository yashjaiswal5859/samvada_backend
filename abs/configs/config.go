// =============================================================================
// Samvada Platform — ABS Configuration Management
// =============================================================================

package configs

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	App         AppConfig         `yaml:"app"`
	Database    DatabaseConfig    `yaml:"database"`
	Redis       RedisConfig       `yaml:"redis"`
	Kafka       KafkaConfig       `yaml:"kafka"`
	Auth        AuthConfig        `yaml:"auth"`
	S2S         S2SConfig         `yaml:"s2s"`
	Reporting   ReportingConfig   `yaml:"reporting"`
	Storage     StorageConfig     `yaml:"storage"`
	DemoCredits DemoCreditsConfig `yaml:"demo_credits"`
	Keto        KetoConfig        `yaml:"keto"`
	Admin       AdminConfig       `yaml:"admin"`
}

type AppConfig struct {
	Name     string `yaml:"name"`
	Env      string `yaml:"env"`
	HTTPPort int    `yaml:"http_port"`
	GRPCPort int    `yaml:"grpc_port"`
	LogLevel string `yaml:"log_level"`
}

type DatabaseConfig struct {
	Host               string `yaml:"host"`
	Port               int    `yaml:"port"`
	User               string `yaml:"user"`
	Password           string `yaml:"password"`
	DBName             string `yaml:"dbname"`
	SSLMode            string `yaml:"sslmode"`
	MaxOpenConns       int      `yaml:"max_open_conns"`
	MaxIdleConns       int      `yaml:"max_idle_conns"`
	ConnMaxLifetimeMin int      `yaml:"conn_max_lifetime_min"`
	ReplicaHosts       []string `yaml:"replica_hosts"`
}

func (d *DatabaseConfig) DSN() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s timezone=UTC",
		d.Host, d.Port, d.User, d.Password, d.DBName, d.SSLMode)
}

type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
	PoolSize int    `yaml:"pool_size"`
}

type KafkaConfig struct {
	Brokers       []string `yaml:"brokers"`
	ConsumerGroup string   `yaml:"consumer_group"`
}

type AuthConfig struct {
	JWTSecret   string `yaml:"jwt_secret"`
	ServiceURL  string `yaml:"service_url"`
	GRPCAddress string `yaml:"grpc_address"`
}

type KetoConfig struct {
	ReadURL  string `yaml:"read_url"`
	WriteURL string `yaml:"write_url"`
}

type AdminConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	UserID   string `yaml:"user_id"`
}

type S2SConfig struct {
	ServiceURL  string `yaml:"service_url"`
	GRPCAddress string `yaml:"grpc_address"`
}

type ReportingConfig struct {
	GRPCAddress string `yaml:"grpc_address"`
}

type StorageConfig struct {
	Provider    string `yaml:"provider"`
	LocalPath   string `yaml:"local_path"`
	S3Endpoint  string `yaml:"s3_endpoint"`
	S3Bucket    string `yaml:"s3_bucket"`
	S3AccessKey string `yaml:"s3_access_key"`
	S3SecretKey string `yaml:"s3_secret_key"`
	S3PublicURL string `yaml:"s3_public_url"`
	S3UseSSL    bool   `yaml:"s3_use_ssl"`
}

type DemoCreditsConfig struct {
	InitialUSD     float64 `yaml:"initial_usd"`
	InitialMinutes int     `yaml:"initial_minutes"`
}

// Load reads config from YAML and applies environment variable overrides.
func Load(path string) (*Config, error) {
	cfg := &Config{
		App: AppConfig{
			Name:     "samvada-abs",
			Env:      "development",
			HTTPPort: 8082,
			GRPCPort: 9082,
			LogLevel: "debug",
		},
		Database: DatabaseConfig{
			Host:               "localhost",
			Port:               5432,
			User:               "samvada",
			Password:           "samvada_dev_2026",
			DBName:             "samvada_abs",
			SSLMode:            "disable",
			MaxOpenConns:       100,
			MaxIdleConns:       20,
			ConnMaxLifetimeMin: 60,
		},
		Redis: RedisConfig{
			Addr:     "localhost:6379",
			Password: "",
			DB:       1,
			PoolSize: 50,
		},
		Kafka: KafkaConfig{
			Brokers:       []string{"localhost:9092"},
			ConsumerGroup: "samvada-abs-consumer",
		},
		Auth: AuthConfig{
			JWTSecret: "samvada_jwt_super_secret_dev_2026_key",
		},
		Storage: StorageConfig{
			Provider:    "s3",
			LocalPath:   "./storage_data",
			S3Endpoint:  "localhost:9002",
			S3Bucket:    "samvada-kb",
			S3AccessKey: "samvada_admin",
			S3SecretKey: "samvada_storage_2026",
			S3PublicURL: "http://localhost:9002/samvada-kb",
			S3UseSSL:    false,
		},
		DemoCredits: DemoCreditsConfig{
			InitialUSD:     5.00,
			InitialMinutes: 100,
		},
		Keto: KetoConfig{
			ReadURL:  "http://keto:4466",
			WriteURL: "http://keto:4467",
		},
		Admin: AdminConfig{
			Username: "admin",
			Password: "SamvadaAdmin2026!",
			UserID:   "00000000-0000-0000-0000-000000000001",
		},
	}

	if data, err := os.ReadFile(path); err == nil {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("failed to parse config file: %w", err)
		}
	}

	// Environment variable overrides
	if v := os.Getenv("APP_ENV"); v != "" {
		cfg.App.Env = v
	}
	if v := os.Getenv("HTTP_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.App.HTTPPort = port
		}
	}
	if v := os.Getenv("GRPC_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.App.GRPCPort = port
		}
	}
	if v := os.Getenv("DB_HOST"); v != "" {
		cfg.Database.Host = v
	}
	if v := os.Getenv("DB_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Database.Port = port
		}
	}
	if v := os.Getenv("DB_USER"); v != "" {
		cfg.Database.User = v
	}
	if v := os.Getenv("DB_PASSWORD"); v != "" {
		cfg.Database.Password = v
	}
	if v := os.Getenv("DB_NAME"); v != "" {
		cfg.Database.DBName = v
	}
	if v := os.Getenv("DB_REPLICA_HOSTS"); v != "" {
		cfg.Database.ReplicaHosts = strings.Split(v, ",")
	}
	if v := os.Getenv("REDIS_ADDR"); v != "" {
		cfg.Redis.Addr = v
	}
	if v := os.Getenv("KAFKA_BROKERS"); v != "" {
		cfg.Kafka.Brokers = strings.Split(v, ",")
	}
	if v := os.Getenv("JWT_SECRET"); v != "" {
		cfg.Auth.JWTSecret = v
	}
	if v := os.Getenv("STORAGE_PROVIDER"); v != "" {
		cfg.Storage.Provider = v
	}
	if v := os.Getenv("S3_ENDPOINT"); v != "" {
		cfg.Storage.S3Endpoint = v
	}
	if v := os.Getenv("S3_BUCKET"); v != "" {
		cfg.Storage.S3Bucket = v
	}
	if v := os.Getenv("S3_ACCESS_KEY"); v != "" {
		cfg.Storage.S3AccessKey = v
	}
	if v := os.Getenv("S3_SECRET_KEY"); v != "" {
		cfg.Storage.S3SecretKey = v
	}
	if v := os.Getenv("S3_PUBLIC_URL"); v != "" {
		cfg.Storage.S3PublicURL = v
	}
	if v := os.Getenv("S3_USE_SSL"); v != "" {
		cfg.Storage.S3UseSSL = v == "true" || v == "1"
	}
	if v := os.Getenv("KETO_READ_URL"); v != "" {
		cfg.Keto.ReadURL = v
	}
	if v := os.Getenv("KETO_WRITE_URL"); v != "" {
		cfg.Keto.WriteURL = v
	}
	if v := os.Getenv("ADMIN_USERNAME"); v != "" {
		cfg.Admin.Username = v
	}
	if v := os.Getenv("ADMIN_PASSWORD"); v != "" {
		cfg.Admin.Password = v
	}
	if v := os.Getenv("ADMIN_USER_ID"); v != "" {
		cfg.Admin.UserID = v
	}

	// Production readiness validation
	if cfg.App.Env == "production" {
		if cfg.Auth.JWTSecret == "samvada_jwt_super_secret_dev_2026_key" || cfg.Auth.JWTSecret == "" {
			return nil, fmt.Errorf("CRITICAL SECURITY ERROR: Cannot start in production with default or empty JWT_SECRET. Set JWT_SECRET environment variable")
		}
	}

	return cfg, nil
}
