package bootstrap

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Port            int
	Env             string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration

	DBHost            string
	DBPort            int
	DBUser            string
	DBPassword        string
	DBName            string
	DBSSLMode         string
	DBMaxOpenConns    int
	DBMaxIdleConns    int
	DBConnMaxLifetime time.Duration

	RedisHost     string
	RedisPort     int
	RedisPassword string
	RedisDB       int
	RedisPoolSize int

	KafkaBrokers             []string
	KafkaClientID            string
	KafkaTopicOTPSend        string
	KafkaTopicUserRegistered string
	KafkaTopicUserLoggedIn    string
	KafkaTopicUserLoggedOut   string
	KafkaTopicTenantProvision string

	KratosAdminURL  string
	KratosPublicURL string

	HydraAdminURL     string
	HydraPublicURL    string
	HydraBrowserURL   string
	HydraClientID     string
	HydraClientSecret string
	HydraRedirectURI  string

	KetoWriteURL string
	KetoReadURL  string

	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURI  string

	FrontendURL            string
	IsProduction           bool
	AllowedRedirectDomains []string
	RoleEncryptionKey      string
	JWTSecret              string

	OTPExpiry   time.Duration
	MaxAttempts int
}

func LoadConfig() (*Config, error) {
	_ = godotenv.Load(".env.local")
	_ = godotenv.Load()

	port, _ := strconv.Atoi(getEnv("PORT", "8081"))
	readTimeout, _ := time.ParseDuration(getEnv("READ_TIMEOUT", "15s"))
	writeTimeout, _ := time.ParseDuration(getEnv("WRITE_TIMEOUT", "15s"))
	shutdownTimeout, _ := time.ParseDuration(getEnv("SHUTDOWN_TIMEOUT", "10s"))

	dbPort, _ := strconv.Atoi(getEnv("DB_PORT", "5432"))
	dbMaxOpen, _ := strconv.Atoi(getEnv("DB_MAX_OPEN_CONNS", "25"))
	dbMaxIdle, _ := strconv.Atoi(getEnv("DB_MAX_IDLE_CONNS", "25"))
	dbMaxLifetime, _ := time.ParseDuration(getEnv("DB_CONN_MAX_LIFETIME", "5m"))

	redisPort, _ := strconv.Atoi(getEnv("REDIS_PORT", "6379"))
	redisDB, _ := strconv.Atoi(getEnv("REDIS_DB", "0"))
	redisPool, _ := strconv.Atoi(getEnv("REDIS_POOL_SIZE", "10"))

	otpExpiry, _ := time.ParseDuration(fmt.Sprintf("%sm", getEnv("OTP_EXPIRY_MINUTES", "5")))
	maxAttempts, _ := strconv.Atoi(getEnv("OTP_MAX_ATTEMPTS", "5"))

	brokersStr := getEnv("KAFKA_BROKERS", "localhost:9092")
	brokers := splitAndTrim(brokersStr, ",")

	return &Config{
		Port:            port,
		Env:             getEnv("ENV", "local"),
		ReadTimeout:     readTimeout,
		WriteTimeout:    writeTimeout,
		ShutdownTimeout: shutdownTimeout,

		DBHost:            getEnv("DB_HOST", "localhost"),
		DBPort:            dbPort,
		DBUser:            getEnv("DB_USER", "samvada"),
		DBPassword:        getEnv("DB_PASSWORD", "samvada_dev_2026"),
		DBName:            getEnv("DB_NAME", "samvada_auth"),
		DBSSLMode:         getEnv("DB_SSLMODE", "disable"),
		DBMaxOpenConns:    dbMaxOpen,
		DBMaxIdleConns:    dbMaxIdle,
		DBConnMaxLifetime: dbMaxLifetime,

		RedisHost:     getEnv("REDIS_HOST", "localhost"),
		RedisPort:     redisPort,
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		RedisDB:       redisDB,
		RedisPoolSize: redisPool,

		KafkaBrokers:             brokers,
		KafkaClientID:            getEnv("KAFKA_CLIENT_ID", "auth-service"),
		KafkaTopicOTPSend:        getEnv("KAFKA_TOPIC_OTP_SEND", "auth.email.send"),
		KafkaTopicUserRegistered: getEnv("KAFKA_TOPIC_USER_REGISTERED", "auth.user.registered"),
		KafkaTopicUserLoggedIn:    getEnv("KAFKA_TOPIC_USER_LOGGED_IN", "auth.user.logged_in"),
		KafkaTopicUserLoggedOut:   getEnv("KAFKA_TOPIC_USER_LOGGED_OUT", "auth.user.logged_out"),
		KafkaTopicTenantProvision: getEnv("KAFKA_TOPIC_TENANT_PROVISION", "auth.onboarding.completed"),

		KratosAdminURL:  getEnv("KRATOS_ADMIN_URL", "http://localhost:4434"),
		KratosPublicURL: getEnv("KRATOS_PUBLIC_URL", "http://localhost:4433"),

		HydraAdminURL:     getEnv("HYDRA_ADMIN_URL", "http://localhost:4445"),
		HydraPublicURL:    getEnv("HYDRA_PUBLIC_URL", "http://localhost:4444"),
		HydraBrowserURL:   getEnv("HYDRA_BROWSER_URL", "http://localhost:4444"),
		HydraClientID:     getEnv("HYDRA_CLIENT_ID", "auth-service-client"),
		HydraClientSecret: getEnv("HYDRA_CLIENT_SECRET", "auth-service-secret"),
		HydraRedirectURI:  getEnv("HYDRA_REDIRECT_URI", "http://localhost:8080/api/v1/auth/callback"),

		KetoWriteURL: getEnv("KETO_WRITE_URL", "http://localhost:4467"),
		KetoReadURL:  getEnv("KETO_READ_URL", "http://localhost:4466"),

		GoogleClientID:     getEnv("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret: getEnv("GOOGLE_CLIENT_SECRET", ""),
		GoogleRedirectURI:  getEnv("GOOGLE_REDIRECT_URL", "http://localhost:8080/api/v1/auth/google/callback"),

		FrontendURL:            getEnv("FRONTEND_URL", "http://localhost:5173"),
		IsProduction:           getEnv("ENV", "local") == "production",
		AllowedRedirectDomains: splitAndTrim(getEnv("ALLOWED_REDIRECT_DOMAINS", "localhost"), ","),
		RoleEncryptionKey:      getEnv("ROLE_ENCRYPTION_KEY", "this_is_a_very_secret_role_key_32_chars"),
		JWTSecret:              getEnv("JWT_SECRET", "samvada_jwt_super_secret_dev_2026_key"),

		OTPExpiry:   otpExpiry,
		MaxAttempts: maxAttempts,
	}, nil
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return defaultVal
}

func splitAndTrim(s, sep string) []string {
	if s == "" {
		return []string{}
	}
	parts := strings.Split(s, sep)
	var res []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			res = append(res, trimmed)
		}
	}
	return res
}
