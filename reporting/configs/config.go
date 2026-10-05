// =============================================================================
// Samvada Platform — Reporting Engine Service Configuration
// =============================================================================

package configs

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	App        AppConfig        `yaml:"app"`
	Database   DatabaseConfig   `yaml:"database"`
	ClickHouse ClickHouseConfig `yaml:"clickhouse"`
	Redis      RedisConfig      `yaml:"redis"`
	Kafka      KafkaConfig      `yaml:"kafka"`
	Auth       AuthConfig       `yaml:"auth"`
	Batch      BatchConfig      `yaml:"batch"`
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
	MaxOpenConns       int    `yaml:"max_open_conns"`
	MaxIdleConns       int    `yaml:"max_idle_conns"`
	ConnMaxLifetimeMin int    `yaml:"conn_max_lifetime_min"`
}

func (d *DatabaseConfig) DSN() string {
	sslMode := d.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s timezone=UTC",
		d.Host, d.Port, d.User, d.Password, d.DBName, sslMode)
}

type ClickHouseConfig struct {
	Addr           string `yaml:"addr"`
	Database       string `yaml:"database"`
	User           string `yaml:"user"`
	Password       string `yaml:"password"`
	MaxOpenConns   int    `yaml:"max_open_conns"`
	MaxIdleConns   int    `yaml:"max_idle_conns"`
	DialTimeoutSec int    `yaml:"dial_timeout_sec"`
}

type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
	PoolSize int    `yaml:"pool_size"`
}

type KafkaTopicsConfig struct {
	CallCompleted string `yaml:"call_completed"`
	CallTurns     string `yaml:"call_turns"`
	CallStarted   string `yaml:"call_started"`
}

type KafkaConfig struct {
	Brokers       []string          `yaml:"brokers"`
	ConsumerGroup string            `yaml:"consumer_group"`
	Topics        KafkaTopicsConfig `yaml:"topics"`
}

type AuthConfig struct {
	JWTSecret string `yaml:"jwt_secret"`
}

type BatchConfig struct {
	RingBufferSize  int `yaml:"ring_buffer_size"`
	FlushIntervalMS int `yaml:"flush_interval_ms"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing yaml config: %w", err)
	}

	// Environment variable overrides
	if envPort := os.Getenv("PORT"); envPort != "" {
		fmt.Sscanf(envPort, "%d", &cfg.App.HTTPPort)
	}
	if envCHAddr := os.Getenv("CLICKHOUSE_ADDR"); envCHAddr != "" {
		cfg.ClickHouse.Addr = envCHAddr
	}
	if envCHDB := os.Getenv("CLICKHOUSE_DB"); envCHDB != "" {
		cfg.ClickHouse.Database = envCHDB
	}
	if envSecret := os.Getenv("JWT_SECRET"); envSecret != "" {
		cfg.Auth.JWTSecret = envSecret
	}
	if redisAddr := os.Getenv("REDIS_ADDR"); redisAddr != "" {
		cfg.Redis.Addr = redisAddr
	}
	if kafkaBrokers := os.Getenv("KAFKA_BROKERS"); kafkaBrokers != "" {
		cfg.Kafka.Brokers = strings.Split(kafkaBrokers, ",")
	}

	// PostgreSQL environment overrides
	if host := os.Getenv("DB_HOST"); host != "" {
		cfg.Database.Host = host
	}
	if port := os.Getenv("DB_PORT"); port != "" {
		fmt.Sscanf(port, "%d", &cfg.Database.Port)
	}
	if user := os.Getenv("DB_USER"); user != "" {
		cfg.Database.User = user
	}
	if pass := os.Getenv("DB_PASSWORD"); pass != "" {
		cfg.Database.Password = pass
	}
	if dbName := os.Getenv("DB_NAME"); dbName != "" {
		cfg.Database.DBName = dbName
	}
	if ssl := os.Getenv("DB_SSLMODE"); ssl != "" {
		cfg.Database.SSLMode = ssl
	}

	return &cfg, nil
}
