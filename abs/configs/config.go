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

type S2SConfig struct {
	ServiceURL  string `yaml:"service_url"`
	GRPCAddress string `yaml:"grpc_address"`
}

type ReportingConfig struct {
	GRPCAddress string `yaml:"grpc_address"`
}

type StorageConfig struct {
	Provider  string `yaml:"provider"`
	LocalPath string `yaml:"local_path"`
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
		DemoCredits: DemoCreditsConfig{
			InitialUSD:     5.00,
			InitialMinutes: 100,
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

	return cfg, nil
}
