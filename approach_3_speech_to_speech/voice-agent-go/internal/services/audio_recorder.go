package services

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type AudioRecorderService struct {
	baseDir string
}

func NewAudioRecorderService(baseDir string) *AudioRecorderService {
	if baseDir == "" {
		baseDir = "D:/Voice AI/recordings"
	}
	_ = os.MkdirAll(baseDir, 0755)
	return &AudioRecorderService{baseDir: baseDir}
}

type CallSessionRecorder struct {
	sessionID     string
	custPath      string
	agentPath     string
	customerFile  *os.File
	agentFile     *os.File
	customerBytes int64
	agentBytes    int64
	custChan      chan []byte
	agentChan     chan []byte
	doneChan      chan struct{}
	wg            sync.WaitGroup
	mu            sync.Mutex
	closed        bool
}

func (s *AudioRecorderService) StartSessionRecorder(sessionID string) (*CallSessionRecorder, error) {
	if sessionID == "" {
		sessionID = "call_session"
	}
	sessionDir := filepath.Join(s.baseDir, sessionID)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create session recording dir: %w", err)
	}

	custPath := filepath.Join(sessionDir, "customer_input.wav")
	agentPath := filepath.Join(sessionDir, "agent_output.wav")

	cF, err := os.Create(custPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create customer audio file: %w", err)
	}

	aF, err := os.Create(agentPath)
	if err != nil {
		cF.Close()
		return nil, fmt.Errorf("failed to create agent audio file: %w", err)
	}

	// Write dummy 44-byte WAV header placeholder
	var dummyHeader [44]byte
	_, _ = cF.Write(dummyHeader[:])
	_, _ = aF.Write(dummyHeader[:])

	rec := &CallSessionRecorder{
		sessionID:    sessionID,
		custPath:     custPath,
		agentPath:    agentPath,
		customerFile: cF,
		agentFile:    aF,
		custChan:     make(chan []byte, 4000),
		agentChan:    make(chan []byte, 4000),
		doneChan:     make(chan struct{}),
	}

	rec.wg.Add(1)
	go rec.recordingWorkerLoop()

	log.Printf("[Audio Recorder] Active async recording session started: %s -> %s", sessionID, sessionDir)
	return rec, nil
}

func (r *CallSessionRecorder) recordingWorkerLoop() {
	defer r.wg.Done()
	for {
		select {
		case data, ok := <-r.custChan:
			if ok && len(data) > 0 && r.customerFile != nil {
				pcmData := mulawToPcm16(data)
				n, err := r.customerFile.Write(pcmData)
				if err == nil {
					r.customerBytes += int64(n)
				}
			}
		case data, ok := <-r.agentChan:
			if ok && len(data) > 0 && r.agentFile != nil {
				pcmData := mulawToPcm16(data)
				n, err := r.agentFile.Write(pcmData)
				if err == nil {
					r.agentBytes += int64(n)
				}
			}
		case <-r.doneChan:
			// Drain remaining frames in queues
			for len(r.custChan) > 0 {
				data := <-r.custChan
				if len(data) > 0 && r.customerFile != nil {
					pcmData := mulawToPcm16(data)
					n, err := r.customerFile.Write(pcmData)
					if err == nil {
						r.customerBytes += int64(n)
					}
				}
			}
			for len(r.agentChan) > 0 {
				data := <-r.agentChan
				if len(data) > 0 && r.agentFile != nil {
					pcmData := mulawToPcm16(data)
					n, err := r.agentFile.Write(pcmData)
					if err == nil {
						r.agentBytes += int64(n)
					}
				}
			}
			return
		}
	}
}

func (r *CallSessionRecorder) WriteCustomerAudio(data []byte) {
	if r == nil || len(data) == 0 {
		return
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	buf := make([]byte, len(data))
	copy(buf, data)
	r.mu.Unlock()

	select {
	case r.custChan <- buf:
	default:
		log.Printf("[Audio Recorder Warning] Customer recording channel full for session %s, dropping frame", r.sessionID)
	}
}

func (r *CallSessionRecorder) WriteAgentAudio(data []byte) {
	if r == nil || len(data) == 0 {
		return
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	buf := make([]byte, len(data))
	copy(buf, data)
	r.mu.Unlock()

	select {
	case r.agentChan <- buf:
	default:
		log.Printf("[Audio Recorder Warning] Agent recording channel full for session %s, dropping frame", r.sessionID)
	}
}

func (r *CallSessionRecorder) WriteTranscript(speaker string, text string) {
	if r == nil || text == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	sessionDir := filepath.Dir(r.custPath)
	txPath := filepath.Join(sessionDir, "transcript_log.json")

	entry := map[string]interface{}{
		"timestamp": time.Now().Format(time.RFC3339),
		"speaker":   speaker, // "user" (Deepgram STT) or "agent" (LLM/Anjali)
		"text":      text,
	}

	b, _ := json.Marshal(entry)
	b = append(b, '\n')

	f, err := os.OpenFile(txPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		_, _ = f.Write(b)
		f.Close()
	}
}

func (r *CallSessionRecorder) WriteSessionLog(component string, level string, message string) {
	if r == nil || message == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	sessionDir := filepath.Dir(r.custPath)
	logPath := filepath.Join(sessionDir, "session_debug.log")

	timestampStr := time.Now().Format(time.RFC3339)
	logLine := fmt.Sprintf("[%s] [%s] [%s] %s\n", timestampStr, level, component, message)

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		_, _ = f.WriteString(logLine)
		f.Close()
	}
}

func (r *CallSessionRecorder) Close() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	r.mu.Unlock()

	close(r.doneChan)
	r.wg.Wait()

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.customerFile != nil {
		WriteWAVHeader(r.customerFile, uint32(r.customerBytes), 8000, 1, 16)
		_ = r.customerFile.Sync()
		_ = r.customerFile.Close()
		r.customerFile = nil
	}

	if r.agentFile != nil {
		WriteWAVHeader(r.agentFile, uint32(r.agentBytes), 8000, 1, 16)
		_ = r.agentFile.Sync()
		_ = r.agentFile.Close()
		r.agentFile = nil
	}

	// Automatically generate single merged call recording (full_call_recording.wav) containing customer + agent + ambience
	mergedPath := filepath.Join(filepath.Dir(r.custPath), "full_call_recording.wav")
	if err := MergeSessionSingleMixedRecording(r.custPath, r.agentPath, mergedPath); err == nil {
		log.Printf("🎧 [Audio Recorder] Successfully merged single call recording -> %s", mergedPath)
	}

	log.Printf("[Audio Recorder] SAVED CALL RECORDINGS & TRANSCRIPT LOG for Session %s", r.sessionID)
}

func MergeSessionSingleMixedRecording(custPath, agentPath, mergedPath string) error {
	custBytes, err1 := os.ReadFile(custPath)
	agentBytes, err2 := os.ReadFile(agentPath)

	var custPCM []byte
	var agentPCM []byte

	if err1 == nil && len(custBytes) > 44 {
		custPCM = custBytes[44:]
	}
	if err2 == nil && len(agentBytes) > 44 {
		agentPCM = agentBytes[44:]
	}

	custSamples := len(custPCM) / 2
	agentSamples := len(agentPCM) / 2

	maxSamples := custSamples
	if agentSamples > maxSamples {
		maxSamples = agentSamples
	}

	if maxSamples == 0 {
		return fmt.Errorf("no audio data to merge")
	}

	// Single Mono channel 16-bit PCM (2 bytes per sample frame)
	mixedPCM := make([]byte, maxSamples*2)

	for i := 0; i < maxSamples; i++ {
		var s1 int16 = 0
		var s2 int16 = 0

		if i < custSamples {
			s1 = int16(binary.LittleEndian.Uint16(custPCM[i*2 : i*2+2]))
		}
		if i < agentSamples {
			s2 = int16(binary.LittleEndian.Uint16(agentPCM[i*2 : i*2+2]))
		}

		// Additive audio sample mixing with 16-bit PCM soft clipping protection
		mixed := int32(s1) + int32(s2)
		if mixed > 32767 {
			mixed = 32767
		} else if mixed < -32768 {
			mixed = -32768
		}

		binary.LittleEndian.PutUint16(mixedPCM[i*2:i*2+2], uint16(int16(mixed)))
	}

	f, err := os.Create(mergedPath)
	if err != nil {
		return err
	}
	defer f.Close()

	var dummyHeader [44]byte
	_, _ = f.Write(dummyHeader[:])
	_, _ = f.Write(mixedPCM)

	pcmDataLen := uint32(len(mixedPCM))
	WriteWAVHeader(f, pcmDataLen, 8000, 1, 16) // 1 channel Mono mixed audio
	_ = f.Sync()
	return nil
}

func WriteWAVHeader(f *os.File, pcmDataLen uint32, sampleRate uint32, channels uint16, bitsPerSample uint16) {
	var header [44]byte
	copy(header[0:4], []byte("RIFF"))
	binary.LittleEndian.PutUint32(header[4:8], pcmDataLen+36)
	copy(header[8:12], []byte("WAVE"))
	copy(header[12:16], []byte("fmt "))
	binary.LittleEndian.PutUint32(header[16:20], 16)
	binary.LittleEndian.PutUint16(header[20:22], 1) // 1 = PCM linear format (universal Windows playback)
	binary.LittleEndian.PutUint16(header[22:24], channels)
	binary.LittleEndian.PutUint32(header[24:28], sampleRate)
	byteRate := sampleRate * uint32(channels) * uint32(bitsPerSample) / 8
	binary.LittleEndian.PutUint32(header[28:32], byteRate)
	blockAlign := channels * bitsPerSample / 8
	binary.LittleEndian.PutUint16(header[32:34], blockAlign)
	binary.LittleEndian.PutUint16(header[34:36], bitsPerSample)
	copy(header[36:40], []byte("data"))
	binary.LittleEndian.PutUint32(header[40:44], pcmDataLen)

	_, _ = f.Seek(0, 0)
	_, _ = f.Write(header[:])
	_ = f.Sync()
}

func MulawSampleToPcm16(uVal byte) int16 {
	uVal = ^uVal
	exponent := (uVal >> 4) & 0x07
	mantissa := uVal & 0x0F
	sample := int16((int32(mantissa)<<3 + 132) << exponent - 132)
	if (uVal & 0x80) != 0 {
		return sample
	}
	return -sample
}

func mulawToPcm16(mulawData []byte) []byte {
	pcmBytes := make([]byte, len(mulawData)*2)
	for i, uVal := range mulawData {
		sample := MulawSampleToPcm16(uVal)
		binary.LittleEndian.PutUint16(pcmBytes[i*2:i*2+2], uint16(sample))
	}
	return pcmBytes
}

// ApplyDynamicGain applies Dynamic Range Compression (DRC) to 8kHz mu-law audio.
// Quiet speech/whispers get a 3.5x boost (+11dB), moderate speech gets 1.8x, and loud speech is limited to 1.0x.
func ApplyDynamicGain(mulawData []byte) []byte {
	if len(mulawData) == 0 {
		return mulawData
	}
	boosted := make([]byte, len(mulawData))
	for i, uVal := range mulawData {
		sample := MulawSampleToPcm16(uVal)
		absPcm := sample
		if absPcm < 0 {
			absPcm = -absPcm
		}

		var gain float32
		if absPcm < 4000 {
			gain = 3.5 // Soft speech / whisper boost (+11 dB)
		} else if absPcm < 14000 {
			gain = 1.8 // Moderate speech boost
		} else {
			gain = 1.0 // Peak limiter (loud speech un-boosted)
		}

		newPcmFloat := float32(sample) * gain
		if newPcmFloat > 32767 {
			newPcmFloat = 32767
		} else if newPcmFloat < -32768 {
			newPcmFloat = -32768
		}

		boosted[i] = linearToMulaw(int16(newPcmFloat))
	}
	return boosted
}

func IsSpeechFrame(mulawData []byte) bool {
	speechCount := 0
	for _, uVal := range mulawData {
		sample := MulawSampleToPcm16(uVal)
		if sample > 150 || sample < -150 { // Lowered energy threshold from 350 to 150 to catch soft whispers
			speechCount++
		}
	}
	return speechCount > (len(mulawData) / 16)
}
