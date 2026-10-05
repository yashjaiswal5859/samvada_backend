// =============================================================================
// Samvada Platform — Live Console Real-Time Supervisor WebSocket (Phase 3.3)
// =============================================================================
// Streams live transcript turns, sentiment transitions, and supervisor audio
// to the browser frontend live console.
// =============================================================================

package websocket

import (
	"context"
	"log"
	"net/http"
	"time"

	"voice-agent-go/internal/db"

	"github.com/gorilla/websocket"
)

type LiveConsoleWSHandler struct{}

func NewLiveConsoleWSHandler() *LiveConsoleWSHandler {
	return &LiveConsoleWSHandler{}
}

func (h *LiveConsoleWSHandler) HandleLiveConsole(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[Live Console WS] Upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	callID := r.URL.Query().Get("call_id")
	log.Printf("[Live Console WS] Supervisor connected for call: %s", callID)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// If callID is specified and Redis is active, subscribe to live call telemetry channel
	if db.RedisClient != nil && callID != "" {
		channelName := "call:telemetry:" + callID
		pubsub := db.RedisClient.Subscribe(ctx, channelName)
		defer pubsub.Close()

		ch := pubsub.Channel()
		go func() {
			for msg := range ch {
				if err := conn.WriteMessage(websocket.TextMessage, []byte(msg.Payload)); err != nil {
					break
				}
			}
		}()
	}

	// Ping/pong heartbeat loop
	for {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}
