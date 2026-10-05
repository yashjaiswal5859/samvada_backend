package services

import (
	"encoding/base64"
	"log"
	"strings"
	"sync"
	"time"

	"voice-agent-go/internal/config"
	"voice-agent-go/internal/services/telephony"

	"github.com/gorilla/websocket"
)

// MediaPacer manages high-precision 20ms RTP frame pacing for PSTN telephony streams.
type MediaPacer struct {
	sessionID         string
	streamSid         string
	provider          telephony.TelephonyProvider
	conn              *websocket.Conn
	writeMu           *sync.Mutex
	audioBuffer       []byte
	mu                sync.Mutex
	ticker            *time.Ticker
	stopChan          chan struct{}
	running           bool
	lastAudioSentTime time.Time
	currentGain       float64
	customVolume      float64
	customVolumeSet   bool
	isBuffering       bool
}

func NewMediaPacer(sessionID string, streamSid string, provider telephony.TelephonyProvider, conn *websocket.Conn, writeMu *sync.Mutex) *MediaPacer {
	idleGain := config.Settings.AmbienceVolumeIdle
	if idleGain <= 0 {
		idleGain = 0.25
	}
	return &MediaPacer{
		sessionID:         sessionID,
		streamSid:         streamSid,
		provider:          provider,
		conn:              conn,
		writeMu:           writeMu,
		stopChan:          make(chan struct{}),
		lastAudioSentTime: time.Now(),
		currentGain:       idleGain,
		isBuffering:       true,
	}
}

// IsSpeaking returns true if agent TTS audio is currently playing or was played in the last 400ms.
func (p *MediaPacer) IsSpeaking() bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.audioBuffer) > 0 || time.Since(p.lastAudioSentTime) < 400*time.Millisecond
}

// SetStreamSid thread-safely updates the active Plivo stream ID for this session
func (p *MediaPacer) SetStreamSid(sid string) {
	if p == nil || sid == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.streamSid = sid
}

// SetAmbienceVolume thread-safely overrides the background ambience volume for this call session (invoked via API payload)
func (p *MediaPacer) SetAmbienceVolume(vol float64) {
	if p == nil || vol < 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.customVolume = vol
	p.currentGain = vol
	p.customVolumeSet = true
	log.Printf("[Media Pacer] Dynamically locked session Ambience Volume to CONSTANT %.2f for Session %s", vol, p.sessionID)
}

func (p *MediaPacer) Start() {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return
	}
	p.running = true
	p.ticker = time.NewTicker(20 * time.Millisecond) // Standard 50Hz PSTN frame clock
	p.mu.Unlock()

	log.Printf("[Media Pacer] Started 20ms audio frame pacer with Ambience Mixer for Session %s", p.sessionID)

	go func() {
		for {
			select {
			case <-p.stopChan:
				log.Printf("[Media Pacer] Stopped pacer for Session %s", p.sessionID)
				return
			case <-p.ticker.C:
				p.mu.Lock()
				sid := p.streamSid
				ttsLen := len(p.audioBuffer)

				// Enterprise Jitter Buffer Cushion (1280 bytes = 160ms of mulaw audio) to eliminate streaming drops and network jitter
				preBufferThreshold := 1280
				if p.isBuffering {
					if ttsLen >= preBufferThreshold {
						p.isBuffering = false
					} else if ttsLen == 0 {
						p.isBuffering = false
					}
				} else {
					if ttsLen == 0 {
						p.isBuffering = true
					}
				}

				isTTSActive := !p.isBuffering && ttsLen > 0

				// Ambience Volume Control: Constant if set via API body, otherwise fallback to defaults
				if p.customVolumeSet {
					p.currentGain = p.customVolume // Locked CONSTANT volume requested by API payload
				} else {
					if isTTSActive {
						p.currentGain = config.Settings.AmbienceVolumeSpeaking
						if p.currentGain <= 0 {
							p.currentGain = 0.12
						}
					} else {
						p.currentGain = config.Settings.AmbienceVolumeIdle
						if p.currentGain <= 0 {
							p.currentGain = 0.35
						}
					}
				}

				var outboundChunk []byte

				if config.Settings.AmbienceAudioEnabled && GlobalAmbienceService != nil && GlobalAmbienceService.enabled {
					// 1. Fetch 160 PCM samples (20ms @ 8kHz) of background office ambience
					ambiencePCM := GlobalAmbienceService.GetNextFrame(160)

					// 2. Fetch up to 160 bytes of G.711 mu-law TTS audio if agent is speaking
					var ttsMuLaw []byte
					if isTTSActive {
						chunkSize := 160
						if ttsLen < chunkSize {
							chunkSize = ttsLen
						}
						ttsMuLaw = make([]byte, chunkSize)
						copy(ttsMuLaw, p.audioBuffer[:chunkSize])
						p.audioBuffer = p.audioBuffer[chunkSize:]
						p.lastAudioSentTime = time.Now()
					}
					p.mu.Unlock()

					// 3. Digital Sidechain Sample Mix: Ambience (scaled by currentGain) + TTS Speech
					outboundChunk = mixAudioFrame(ambiencePCM, ttsMuLaw, p.currentGain)

				} else {
					// Fallback when Ambience is disabled: send only when TTS audio is available
					if !isTTSActive {
						p.mu.Unlock()
						continue
					}
					chunkSize := 160
					if ttsLen < chunkSize {
						chunkSize = ttsLen
					}
					outboundChunk = make([]byte, chunkSize)
					copy(outboundChunk, p.audioBuffer[:chunkSize])
					p.audioBuffer = p.audioBuffer[chunkSize:]
					p.lastAudioSentTime = time.Now()
					p.mu.Unlock()
				}

				b64Chunk := base64.StdEncoding.EncodeToString(outboundChunk)
				payload, err := p.provider.BuildAudioOutboundPayload(sid, b64Chunk)
				if err != nil {
					log.Printf("[Media Pacer] Warning: Error building outbound payload: %v", err)
					continue
				}

				p.writeMu.Lock()
				err = p.conn.WriteMessage(websocket.TextMessage, payload)
				p.writeMu.Unlock()

				if err != nil {
					p.mu.Lock()
					isRunning := p.running
					p.mu.Unlock()
					if isRunning {
						if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) ||
							strings.Contains(err.Error(), "close sent") ||
							strings.Contains(err.Error(), "closed network connection") ||
							strings.Contains(err.Error(), "broken pipe") {
							// WebSocket closed safely on call hangup — stop pacer immediately without spamming logs
							p.Stop()
						} else {
							log.Printf("[Media Pacer] Warning: WebSocket write error for Session %s: %v", p.sessionID, err)
							p.Stop()
						}
					}
				}
			}
		}
	}()
}

func mixAudioFrame(ambiencePCM []int16, ttsMuLaw []byte, gain float64) []byte {
	out := make([]byte, 160)
	ttsLen := len(ttsMuLaw)

	for i := 0; i < 160; i++ {
		// 1. Scaled Ambience sample
		ambSample := int32(float64(ambiencePCM[i]) * gain)

		// 2. TTS sample (if available)
		var ttsSample int32 = 0
		if i < ttsLen {
			ttsSample = int32(muLawToLinearTable[ttsMuLaw[i]])
		}

		// 3. Digital Mix with Hard Clipping Protection (-32768 to +32767)
		mixedPCM := ambSample + ttsSample
		if mixedPCM > 32767 {
			mixedPCM = 32767
		} else if mixedPCM < -32768 {
			mixedPCM = -32768
		}

		// 4. Encode to G.711 mu-law
		out[i] = linearToMulaw(int16(mixedPCM))
	}
	return out
}

var muLawToLinearTable = [256]int16{
	-32124, -31100, -30076, -29052, -28028, -27004, -25980, -24956,
	-23932, -22908, -21884, -20860, -19836, -18812, -17788, -16764,
	-15996, -15484, -14972, -14460, -13948, -13436, -12924, -12412,
	-11900, -11388, -10876, -10364,  -9852,  -9340,  -8828,  -8316,
	 -7932,  -7676,  -7420,  -7164,  -6908,  -6652,  -6396,  -6140,
	 -5884,  -5628,  -5372,  -5116,  -4860,  -4604,  -4348,  -4092,
	 -3899,  -3771,  -3643,  -3515,  -3387,  -3259,  -3131,  -3003,
	 -2875,  -2747,  -2619,  -2491,  -2363,  -2235,  -2107,  -1979,
	 -1883,  -1819,  -1755,  -1691,  -1627,  -1563,  -1499,  -1435,
	 -1371,  -1307,  -1243,  -1179,  -1115,  -1051,   -987,   -923,
	  -875,   -843,   -811,   -779,   -747,   -715,   -683,   -651,
	  -619,   -587,   -555,   -523,   -491,   -459,   -427,   -395,
	  -371,   -355,   -339,   -323,   -307,   -291,   -275,   -259,
	  -243,   -227,   -211,   -195,   -179,   -163,   -147,   -131,
	  -119,   -111,   -103,    -95,    -87,    -79,    -71,    -63,
	   -55,    -47,    -39,    -31,    -23,    -15,     -7,      1,
	 32124,  31100,  30076,  29052,  28028,  27004,  25980,  24956,
	 23932,  22908,  21884,  20860,  19836,  18812,  17788,  16764,
	 15996,  15484,  14972,  14460,  13948,  13436,  12924,  12412,
	 11900,  11388,  10876,  10364,   9852,   9340,   8828,   8316,
	  7932,   7676,   7420,   7164,   6908,   6652,   6396,   6140,
	  5884,   5628,   5372,   5116,   4860,   4604,   4348,   4092,
	  3899,   3771,   3643,   3515,   3387,   3259,   3131,   3003,
	  2875,   2747,   2619,   2491,   2363,   2235,   2107,   1979,
	  1883,   1819,   1755,   1691,   1627,   1563,   1499,   1435,
	  1371,   1307,   1243,   1179,   1115,   1051,    987,    923,
	   875,    843,    811,    779,    747,    715,    683,    651,
	   619,    587,    555,    523,    491,    459,    427,    395,
	   371,    355,    339,    323,    307,    291,    275,    259,
	   243,    227,    211,    195,    179,    163,    147,    131,
	   119,    111,    103,     95,     87,     79,     71,     63,
	    55,     47,     39,     31,     23,     15,      7,     -1,
}

// PushAudio enqueues synthesized mulaw audio bytes into the Pacer's Ring Buffer
func (p *MediaPacer) PushAudio(data []byte) {
	if p == nil || len(data) == 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	p.audioBuffer = append(p.audioBuffer, data...)
	log.Printf("[Media Pacer] Enqueued %d audio bytes to pacer buffer (Total in buffer: %d bytes)", len(data), len(p.audioBuffer))
}

// IsPlaying returns true if there is pending audio in the pacer queue
func (p *MediaPacer) IsPlaying() bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.audioBuffer) > 0
}

// LastAudioSentTime returns the timestamp of when the agent last sent an audio frame
func (p *MediaPacer) LastAudioSentTime() time.Time {
	if p == nil {
		return time.Time{}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastAudioSentTime
}

// ClearBuffer (Barge-In / Interruption) flushes the queue instantly when user interrupts
func (p *MediaPacer) ClearBuffer() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	flushedBytes := len(p.audioBuffer)
	p.audioBuffer = nil
	if flushedBytes > 0 {
		log.Printf("[Media Pacer] BARGE-IN INTERRUPTION: Flushed %d pending audio bytes for Session %s", flushedBytes, p.sessionID)
		if p.conn != nil && p.writeMu != nil {
			clearPayload := []byte(`{"event":"clearAudio"}`)
			p.writeMu.Lock()
			_ = p.conn.WriteMessage(websocket.TextMessage, clearPayload)
			p.writeMu.Unlock()
		}
	}
}

func (p *MediaPacer) Flush() {
	p.ClearBuffer()
}

// Stop cleanly terminates the Media Pacer background goroutine
func (p *MediaPacer) Stop() {
	if p == nil {
		return
	}
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return
	}
	p.running = false
	if p.ticker != nil {
		p.ticker.Stop()
	}
	close(p.stopChan)
	p.mu.Unlock()
}
