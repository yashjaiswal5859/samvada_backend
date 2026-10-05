package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"voice-agent-go/internal/config"
	"voice-agent-go/internal/services"

	"github.com/segmentio/kafka-go"
)

func main() {
	log.Println("[Kafka Worker] Bootstrapping Voice AI Audio, Transcript & Log Processing Worker Server...")
	config.LoadConfig()

	_ = os.MkdirAll("D:/Voice AI/logs", 0755)
	recorderSvc := services.NewAudioRecorderService("D:/Voice AI/recordings")
	activeRecorders := make(map[string]*services.CallSessionRecorder)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if config.Settings.KafkaBootstrapServers != "" {
		log.Printf("[Kafka Worker] Subscribing to Kafka Topics (%s, %s, %s) at %s...",
			services.KafkaAudioTopic, services.KafkaTranscriptTopic, services.KafkaLogTopic, config.Settings.KafkaBootstrapServers)

		// 1. Audio Stream Consumer
		go func() {
			reader := kafka.NewReader(kafka.ReaderConfig{
				Brokers:  []string{config.Settings.KafkaBootstrapServers},
				Topic:    services.KafkaAudioTopic,
				GroupID:  "voice_audio_worker_group",
				MinBytes: 10,
				MaxBytes: 10 * 1024 * 1024,
			})
			defer reader.Close()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					m, err := reader.ReadMessage(ctx)
					if err != nil {
						time.Sleep(500 * time.Millisecond)
						continue
					}
					var event services.AudioFrameEvent
					if err := json.Unmarshal(m.Value, &event); err == nil {
						processAudioEvent(&event, recorderSvc, activeRecorders)
					}
				}
			}
		}()

		// 2. Transcript Consumer
		go func() {
			reader := kafka.NewReader(kafka.ReaderConfig{
				Brokers:  []string{config.Settings.KafkaBootstrapServers},
				Topic:    services.KafkaTranscriptTopic,
				GroupID:  "voice_tx_worker_group",
				MinBytes: 10,
				MaxBytes: 10 * 1024 * 1024,
			})
			defer reader.Close()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					m, err := reader.ReadMessage(ctx)
					if err != nil {
						time.Sleep(500 * time.Millisecond)
						continue
					}
					var event services.TranscriptEvent
					if err := json.Unmarshal(m.Value, &event); err == nil {
						processTranscriptEvent(&event, recorderSvc, activeRecorders)
					}
				}
			}
		}()

		// 3. System Logs Consumer (Writes to D:/Voice AI/logs/system_events.log)
		go func() {
			reader := kafka.NewReader(kafka.ReaderConfig{
				Brokers:  []string{config.Settings.KafkaBootstrapServers},
				Topic:    services.KafkaLogTopic,
				GroupID:  "voice_log_worker_group",
				MinBytes: 10,
				MaxBytes: 10 * 1024 * 1024,
			})
			defer reader.Close()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					m, err := reader.ReadMessage(ctx)
					if err != nil {
						time.Sleep(500 * time.Millisecond)
						continue
					}
					var event services.LogEvent
					if err := json.Unmarshal(m.Value, &event); err == nil {
						processLogEvent(&event)
					}
				}
			}
		}()

		<-ctx.Done()
		log.Println("[Kafka Worker] Shutting down worker...")
		closeAllRecorders(activeRecorders)
		return
	} else {
		log.Println("[Kafka Worker] Running in Standalone Worker Mode (Local Async Queues for Audio, Transcripts & Logs)...")
		kafkaSvc := services.InitKafkaAudioService()
		audioQueue := kafkaSvc.GetLocalAudioQueue()
		txQueue := kafkaSvc.GetLocalTxQueue()
		logQueue := kafkaSvc.GetLocalLogQueue()

		for {
			select {
			case <-ctx.Done():
				log.Println("[Kafka Worker] Shutting down worker...")
				closeAllRecorders(activeRecorders)
				return
			case event, ok := <-audioQueue:
				if ok {
					processAudioEvent(&event, recorderSvc, activeRecorders)
				}
			case txEvent, ok := <-txQueue:
				if ok {
					processTranscriptEvent(&txEvent, recorderSvc, activeRecorders)
				}
			case logEvent, ok := <-logQueue:
				if ok {
					processLogEvent(&logEvent)
				}
			}
		}
	}
}

func processAudioEvent(event *services.AudioFrameEvent, recorderSvc *services.AudioRecorderService, activeRecorders map[string]*services.CallSessionRecorder) {
	if event == nil || event.SessionID == "" {
		return
	}

	rec, exists := activeRecorders[event.SessionID]
	if !exists && event.EventType != "stop" {
		var err error
		rec, err = recorderSvc.StartSessionRecorder(event.SessionID)
		if err != nil {
			log.Printf("[Kafka Worker] Error starting recorder for session %s: %v", event.SessionID, err)
			return
		}
		activeRecorders[event.SessionID] = rec
	}

	rawBytes, _ := base64.StdEncoding.DecodeString(event.Payload)

	switch event.EventType {
	case "start":
		log.Printf("[Kafka Worker] Session %s recording started", event.SessionID)
	case "media":
		// Audio file recording is managed directly by CallHandler in real-time.
		// Worker maintains session log tracking and analytics.
		_ = rawBytes
	case "stop":
		if rec != nil {
			rec.Close()
			delete(activeRecorders, event.SessionID)
			log.Printf("[Kafka Worker] Session %s recording finalized & closed", event.SessionID)
		}
	}
}

func processTranscriptEvent(txEvent *services.TranscriptEvent, recorderSvc *services.AudioRecorderService, activeRecorders map[string]*services.CallSessionRecorder) {
	if txEvent == nil || txEvent.SessionID == "" || txEvent.Text == "" {
		return
	}

	log.Printf("[TRANSCRIPT WORKER] [%s] %s: %q", txEvent.SessionID, txEvent.Speaker, txEvent.Text)

	rec, exists := activeRecorders[txEvent.SessionID]
	if !exists {
		var err error
		rec, err = recorderSvc.StartSessionRecorder(txEvent.SessionID)
		if err == nil {
			activeRecorders[txEvent.SessionID] = rec
		}
	}

	// Transcript writing is handled in real-time by CallHandler.
	_ = rec
}

func processLogEvent(logEvent *services.LogEvent) {
	if logEvent == nil || logEvent.Message == "" || logEvent.SessionID == "" {
		return
	}

	// Filter Worker Console stdout: Print ONLY Agent Emotion logs & Speech Text
	if strings.Contains(logEvent.Message, "[AGENT INTERNAL EMOTION]") || logEvent.Component == "EMOTION" {
		log.Printf("[WORKER LOG] [%s] %s", logEvent.SessionID, logEvent.Message)
	}

	timestampStr := time.UnixMilli(logEvent.Timestamp).Format(time.RFC3339)
	logLine := fmt.Sprintf("[%s] [%s] [%s] %s\n", timestampStr, logEvent.Level, logEvent.Component, logEvent.Message)

	// Append exclusively to Per-Session Debug Log File inside recordings/{session_id}
	sessionDir := filepath.Join("D:/Voice AI/recordings", logEvent.SessionID)
	_ = os.MkdirAll(sessionDir, 0755)
	sessionLogPath := filepath.Join(sessionDir, "session_debug.log")

	sf, err := os.OpenFile(sessionLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		_, _ = sf.WriteString(logLine)
		sf.Close()
	}
}

func closeAllRecorders(recorders map[string]*services.CallSessionRecorder) {
	for sid, rec := range recorders {
		if rec != nil {
			rec.Close()
		}
		delete(recorders, sid)
	}
}
