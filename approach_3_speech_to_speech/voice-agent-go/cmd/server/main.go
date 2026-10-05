package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"

	apihttp "voice-agent-go/internal/api/http"
	"voice-agent-go/internal/api/websocket"
	"voice-agent-go/internal/config"
	"voice-agent-go/internal/db"
	"voice-agent-go/internal/services"
	"voice-agent-go/internal/services/rag"
	"voice-agent-go/internal/services/stt"
)

func main() {
	log.Println("=======================================================================")
	log.Println("🚀 Starting Voice Agent Platform (Approach 3: Single Go Server + Gin)")
	log.Println("=======================================================================")

	// 1. Load Configurations from .env
	config.LoadConfig()

	// Wipe old audio caches & recordings for fresh clean startup
	_ = os.RemoveAll("audio/fillers")
	_ = os.RemoveAll("audio/backchannels")
	_ = os.RemoveAll("audio/greetings")
	_ = os.RemoveAll("audio/pregenerated")
	_ = os.RemoveAll("D:/Voice AI/recordings")
	log.Println("[Go Server] Purged old audio caches and call recordings.")

	// Initialize Global Ambience Audio Loop Service (Continuous Background Office Ambience)
	services.InitGlobalAmbienceService()

	// 2. Initialize Database pools
	if config.Settings.DatabaseUrl != "" {
		db.InitPostgres()
	} else {
		log.Println("[DB] Warning: DATABASE_URL not set. Skipping Postgres initialization.")
	}

	if config.Settings.RedisUrl != "" {
		db.InitRedis()
	} else {
		log.Println("[Redis] Warning: REDIS_URL not set. Skipping Redis initialization.")
	}

	if config.Settings.KafkaBootstrapServers != "" {
		db.InitKafka()
	} else {
		log.Println("[Kafka] Warning: KAFKA_BOOTSTRAP_SERVERS not set. Skipping Kafka initialization.")
	}
	defer db.CloseDBs()

	kafkaAudioSvc := services.InitKafkaAudioService()
	defer kafkaAudioSvc.Close()

	// 3. Initialize In-Process RAG Provider via Factory
	ragProvider, err := rag.CreateRAGProvider()
	if err != nil {
		log.Fatalf("[RAG] Failed to initialize RAG provider: %v", err)
	}
	if closer, ok := ragProvider.(interface{ Close() error }); ok {
		defer closer.Close()
	}

	emotionService, err := services.NewEmotionService()
	if err != nil {
		log.Fatalf("[CIE] Failed to initialize Emotion Service: %v", err)
	}
	defer emotionService.Close()

	// 4. Initialize Core Orchestration Services
	sessionService := services.NewSessionService(ragProvider)

	// 5. Initialize WebSockets & REST Handlers
	ttsService := services.NewTTSService()
	sttProvider := stt.CreateSTTProvider()
	callHandler := websocket.NewCallHandler(sessionService, emotionService, ragProvider, sttProvider)
	callAPIHandler := apihttp.NewCallAPIHandler(ttsService)

	customerService := services.GetCustomerService()
	reportService := services.GetReportService()
	customerAPIHandler := apihttp.NewCustomerAPIHandler(customerService)
	reportAPIHandler := apihttp.NewReportAPIHandler(reportService)

	// S2S ABS Client, Ephemeral Ticket Service & Kafka Telemetry Emitter (Phase 3.1 & 3.2)
	absClient, err := services.NewABSClient(config.Settings.AbsGrpcUrl, db.RedisClient)
	if err != nil {
		log.Printf("[ABS Client] Warning: ABS gRPC server unreachable at %s: %v", config.Settings.AbsGrpcUrl, err)
	} else if absClient != nil {
		defer absClient.Close()
		log.Println("[ABS Client] Connected to ABS gRPC server successfully.")
	}

	ticketService := services.NewTicketService(config.Settings.TicketSecret, db.RedisClient)

	brokers := []string{"localhost:9092"}
	if config.Settings.KafkaBootstrapServers != "" {
		brokers = strings.Split(config.Settings.KafkaBootstrapServers, ",")
	}
	telemetryEmitter := services.NewTelemetryEmitter(brokers)
	defer telemetryEmitter.Close()

	s2sHandler := websocket.NewS2SCallHandler(sessionService, emotionService, absClient, ticketService, telemetryEmitter)

	// 6. Register Gin Web Framework Engine
	router := gin.Default()

	// Helper to wrap http.HandlerFunc for Gin
	wrap := func(handler http.HandlerFunc) gin.HandlerFunc {
		return func(c *gin.Context) {
			handler(c.Writer, c.Request)
		}
	}

	// Customer Info & Report Generation Endpoints
	router.GET("/v1/customer", wrap(customerAPIHandler.HandleGetCustomer))
	router.GET("/v1/customer/:phone", wrap(customerAPIHandler.HandleGetCustomer))
	router.GET("/v1/reports", wrap(reportAPIHandler.HandleGetReport))
	router.GET("/v1/reports/:session_id", wrap(reportAPIHandler.HandleGetReport))
	router.GET("/v1/escalations", wrap(reportAPIHandler.HandleGetEscalations))
	router.POST("/v1/escalations/resolve", wrap(reportAPIHandler.HandleResolveEscalation))

	// Outbound Calling REST Endpoint (Single Exposed API for User)
	router.POST("/v1/calls/outbound", wrap(callAPIHandler.HandleInitiateCall))

	// Telephony Provider Answer Webhooks
	router.POST("/v1/telephony/plivo/answer", wrap(callAPIHandler.HandlePlivoAnswerXML))
	router.GET("/v1/telephony/plivo/answer", wrap(callAPIHandler.HandlePlivoAnswerXML))
	router.POST("/v1/telephony/twilio/answer", wrap(callAPIHandler.HandleTwilioAnswerXML))
	router.GET("/v1/telephony/twilio/answer", wrap(callAPIHandler.HandleTwilioAnswerXML))
	router.POST("/v1/telephony/telnyx/answer", wrap(callAPIHandler.HandleTelnyxAnswerXML))
	router.GET("/v1/telephony/telnyx/answer", wrap(callAPIHandler.HandleTelnyxAnswerXML))

	// Media Stream Entrypoint for Telephony Providers
	router.GET("/v1/stream/:call_sid", func(c *gin.Context) {
		callHandler.HandleCallStream(c.Writer, c.Request)
	})

	// Live Direct Audio & S2S Stream endpoints (Demo Playground, Agent Studio & WebRTC/WebSocket bridge)
	router.GET("/v1/audio/stream", func(c *gin.Context) {
		s2sHandler.HandleS2SCallStream(c.Writer, c.Request)
	})
	router.GET("/ws/calls/demo", func(c *gin.Context) {
		s2sHandler.HandleS2SCallStream(c.Writer, c.Request)
	})

	// Live Call Console & Operator Control Endpoints (Phase 3.3)
	liveConsoleHandler := apihttp.NewLiveConsoleHandler()
	liveConsoleWS := websocket.NewLiveConsoleWSHandler()

	router.GET("/api/v1/calls/live", liveConsoleHandler.ListLiveCalls)
	router.POST("/api/v1/calls/live/:call_id/terminate", liveConsoleHandler.TerminateCall)
	router.POST("/api/v1/calls/live/:call_id/hold", liveConsoleHandler.HoldCall)
	router.POST("/api/v1/calls/live/:call_id/transfer", liveConsoleHandler.TransferCall)
	router.POST("/api/v1/calls/live/:call_id/listen", liveConsoleHandler.ListenCall)

	// Live Console Supervisor WebSocket (Phase 3.3)
	router.GET("/ws/live-console", func(c *gin.Context) {
		liveConsoleWS.HandleLiveConsole(c.Writer, c.Request)
	})

	// Health check route
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "healthy",
			"version":   "3.0-speech-to-speech",
			"framework": "Gin Web Framework",
		})
	})

	portEnv := os.Getenv("PORT")
	if portEnv == "" {
		portEnv = os.Getenv("S2S_PORT")
	}
	if portEnv == "" {
		portEnv = "8084"
	}
	if !strings.HasPrefix(portEnv, ":") {
		portEnv = ":" + portEnv
	}
	serverPort := portEnv
	log.Printf("[Go Server] Gin Web Framework listening on port %s", serverPort)
	if err := router.Run(serverPort); err != nil {
		log.Fatalf("[Go Server] Gin Server failed to start: %v", err)
	}
}
