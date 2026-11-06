package config

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	AppName                 string `mapstructure:"APP_NAME"`
	ApiV1Prefix             string `mapstructure:"API_V1_PREFIX"`
	DatabaseUrl             string `mapstructure:"DATABASE_URL"`
	RedisUrl                string `mapstructure:"REDIS_URL"`
	KafkaBootstrapServers   string `mapstructure:"KAFKA_BOOTSTRAP_SERVERS"`
	DefaultLlmModel         string `mapstructure:"DEFAULT_LLM_MODEL"`
	RagProvider             string `mapstructure:"RAG_PROVIDER"`
	OpenaiApiKey            string `mapstructure:"OPENAI_API_KEY"`
	AnthropicApiKey         string `mapstructure:"ANTHROPIC_API_KEY"`
	GeminiApiKey            string `mapstructure:"GEMINI_API_KEY"`
	DeepgramApiKey          string `mapstructure:"DEEPGRAM_API_KEY"`
	SarvamApiKey            string `mapstructure:"SARVAM_API_KEY"`
	SttProvider             string `mapstructure:"STT_PROVIDER"`
	SttMode                 string `mapstructure:"STT_MODE"` // "streaming" (WS real-time) or "batch" (REST upload)
	BargeInReactionDelayMs  int    `mapstructure:"BARGEIN_REACTION_DELAY_MS"` // Human natural reaction delay (e.g. 80ms)
	SttLanguage             string `mapstructure:"STT_LANGUAGE"`
	TtsProvider             string `mapstructure:"TTS_PROVIDER"`
	TtsVoice                string `mapstructure:"TTS_VOICE"`
	TtsLanguage             string `mapstructure:"TTS_LANGUAGE"`
	CartesiaVoice           string `mapstructure:"CARTESIA_VOICE"`
	FishAudioVoice          string `mapstructure:"FISHAUDIO_VOICE"`
	OpenaiVoice             string `mapstructure:"OPENAI_VOICE"`
	ElevenlabsApiKey        string `mapstructure:"ELEVENLABS_API_KEY"`
	CartesiaApiKey          string `mapstructure:"CARTESIA_API_KEY"`
	FishAudioApiKey         string `mapstructure:"FISH_AUDIO_API_KEY"`
	FishAudioLocalUrl       string `mapstructure:"FISH_AUDIO_LOCAL_URL"`
	MurfApiKey              string `mapstructure:"MURF_API_KEY"`
	MurfVoice               string `mapstructure:"MURF_VOICE"`
	MurfModel               string `mapstructure:"MURF_MODEL"`
	HumeApiKey              string `mapstructure:"HUME_API_KEY"`
	CppServerUrl            string `mapstructure:"CPP_SERVER_URL"` // To talk to C++ local ML server
	DocsBasePath            string `mapstructure:"DOCS_BASE_PATH"` // Shared docs folder for RAG
	TenantDocsPath          string `mapstructure:"TENANT_DOCS_PATH"` // Plug & play path to active tenant folder
	ActiveProductPath       string `mapstructure:"ACTIVE_PRODUCT_PATH"` // Full path to active product doc folder

	// Call & Audio Defaults (No Hardcoding)
	DefaultCompanyName      string  `mapstructure:"DEFAULT_COMPANY_NAME"`
	DefaultCustomerName     string  `mapstructure:"DEFAULT_CUSTOMER_NAME"`
	InitialLanguage         string  `mapstructure:"INITIAL_LANGUAGE"`
	TtsSpeed                float64 `mapstructure:"TTS_SPEED"`
	InboundGreetingDelayMs  int     `mapstructure:"INBOUND_GREETING_DELAY_MS"`
	FillerAudioEnabled      bool    `mapstructure:"FILLER_AUDIO_ENABLED"`
	FillerAudioSpeed        float64 `mapstructure:"FILLER_AUDIO_SPEED"`

	// Ambience Background Audio Configuration
	AmbienceAudioEnabled    bool    `mapstructure:"AMBIENCE_AUDIO_ENABLED"`
	AmbienceAudioPath       string  `mapstructure:"AMBIENCE_AUDIO_PATH"`
	AmbienceVolumeIdle      float64 `mapstructure:"AMBIENCE_VOLUME_IDLE"`
	AmbienceVolumeSpeaking  float64 `mapstructure:"AMBIENCE_VOLUME_SPEAKING"`

	// Server Host for Callbacks
	ServerPublicHost        string `mapstructure:"SERVER_PUBLIC_HOST"`

	// Telephony Providers Configuration
	DefaultTelephonyProvider string `mapstructure:"DEFAULT_TELEPHONY_PROVIDER"`
	TwilioAccountSid         string `mapstructure:"TWILIO_ACCOUNT_SID"`
	TwilioAuthToken          string `mapstructure:"TWILIO_AUTH_TOKEN"`
	TwilioPhoneNumber        string `mapstructure:"TWILIO_PHONE_NUMBER"`
	ExotelAccountSid         string `mapstructure:"EXOTEL_ACCOUNT_SID"`
	ExotelApiKey             string `mapstructure:"EXOTEL_API_KEY"`
	ExotelApiToken           string `mapstructure:"EXOTEL_API_TOKEN"`
	ExotelSubdomain          string `mapstructure:"EXOTEL_SUBDOMAIN"`
	ExotelPhoneNumber        string `mapstructure:"EXOTEL_PHONE_NUMBER"`
	PlivoAuthId              string `mapstructure:"PLIVO_AUTH_ID"`
	PlivoAuthToken           string `mapstructure:"PLIVO_AUTH_TOKEN"`
	PlivoPhoneNumber         string `mapstructure:"PLIVO_PHONE_NUMBER"`
	TelnyxApiKey             string `mapstructure:"TELNYX_API_KEY"`
	TelnyxConnectionId       string `mapstructure:"TELNYX_CONNECTION_ID"`
	TelnyxPhoneNumber        string `mapstructure:"TELNYX_PHONE_NUMBER"`

	// ABS S2S Integration & Ephemeral Ticket Security (Phase 3.1 & 3.2)
	AbsGrpcUrl              string `mapstructure:"ABS_GRPC_URL"`
	TicketSecret            string `mapstructure:"TICKET_SECRET"`
}

var Settings Config

func LoadConfig() {
	envSearchPaths := []string{
		".env",
		"../.env",
		"../../.env",
		"../../../.env",
	}

	loadedPath := ""
	for _, candidate := range envSearchPaths {
		if abs, err := filepath.Abs(candidate); err == nil {
			if _, err := os.Stat(abs); err == nil {
				viper.SetConfigFile(abs)
				loadedPath = abs
				break
			}
		}
	}

	if loadedPath == "" {
		viper.SetConfigFile(".env")
	}
	viper.SetConfigType("env")

	if err := viper.ReadInConfig(); err != nil {
		log.Printf("[Config] Notice: .env file not found or failed reading: %v", err)
	} else {
		log.Printf("[Config] Environment variables loaded successfully from: %s", loadedPath)
	}

	viper.SetDefault("APP_NAME", "Voice Agent Platform (Go)")
	viper.SetDefault("API_V1_PREFIX", "/v1")
	viper.SetDefault("DEFAULT_LLM_MODEL", "gpt-4o-mini")
	viper.SetDefault("RAG_PROVIDER", "openai")
	viper.SetDefault("STT_MODE", "streaming") // Default real-time WebSocket streaming ("streaming" or "batch")
	viper.SetDefault("BARGEIN_REACTION_DELAY_MS", 80) // Default 80ms human natural reaction window
	viper.SetDefault("TTS_PROVIDER", "openai")
	viper.SetDefault("TTS_VOICE", "nova") // Smooth, warm, natural female voice for Anjali
	viper.SetDefault("SERVER_PUBLIC_HOST", "http://localhost:8000")
	viper.SetDefault("DEFAULT_TELEPHONY_PROVIDER", "plivo") // Plivo default (or switchable to exotel, twilio, telnyx)
	viper.SetDefault("CPP_SERVER_URL", "localhost:50051") // Default C++ gRPC port
	viper.SetDefault("FISH_AUDIO_LOCAL_URL", "http://localhost:8080/v1/tts")
	viper.SetDefault("MURF_VOICE", "en-US-natalie")
	viper.SetDefault("MURF_MODEL", "falcon-2")
	viper.SetDefault("DOCS_BASE_PATH", "./docs") // Default shared docs path
	viper.SetDefault("TENANT_DOCS_PATH", "./docs/demo_tenant") // Plug & play active tenant path
	viper.SetDefault("ACTIVE_PRODUCT_PATH", "./docs/demo_tenant")
	viper.SetDefault("DEFAULT_COMPANY_NAME", "Samvada_AI")
	viper.SetDefault("DEFAULT_CUSTOMER_NAME", "Valued Customer")
	viper.SetDefault("INITIAL_LANGUAGE", "english")
	viper.SetDefault("TTS_SPEED", 1.0)
	viper.SetDefault("INBOUND_GREETING_DELAY_MS", 1000)
	viper.SetDefault("FILLER_AUDIO_ENABLED", true)
	viper.SetDefault("FILLER_AUDIO_SPEED", 1.0)
	viper.SetDefault("AMBIENCE_AUDIO_ENABLED", true)
	viper.SetDefault("AMBIENCE_AUDIO_PATH", "./audio/ambience.mp3")
	viper.SetDefault("AMBIENCE_VOLUME_IDLE", 0.25)
	viper.SetDefault("AMBIENCE_VOLUME_SPEAKING", 0.10)
	viper.SetDefault("ABS_GRPC_URL", "localhost:9082")
	viper.SetDefault("TICKET_SECRET", "samvada_ticket_secret_key_2026")

	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Bind environment variables
	if err := viper.Unmarshal(&Settings); err != nil {
		log.Fatalf("Unable to decode config into struct: %v", err)
	}
}
