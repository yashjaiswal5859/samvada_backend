package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func writeWAVHeader(f *os.File, pcmDataLen uint32, sampleRate uint32, channels uint16, bitsPerSample uint16) error {
	var header [44]byte
	copy(header[0:4], []byte("RIFF"))
	binary.LittleEndian.PutUint32(header[4:8], pcmDataLen+36)
	copy(header[8:12], []byte("WAVE"))
	copy(header[12:16], []byte("fmt "))
	binary.LittleEndian.PutUint32(header[16:20], 16)
	binary.LittleEndian.PutUint16(header[20:22], 1) // 1 = PCM linear format
	binary.LittleEndian.PutUint16(header[22:24], channels)
	binary.LittleEndian.PutUint32(header[24:28], sampleRate)
	byteRate := sampleRate * uint32(channels) * uint32(bitsPerSample) / 8
	binary.LittleEndian.PutUint32(header[28:32], byteRate)
	blockAlign := channels * bitsPerSample / 8
	binary.LittleEndian.PutUint16(header[32:34], blockAlign)
	binary.LittleEndian.PutUint16(header[34:36], bitsPerSample)
	copy(header[36:40], []byte("data"))
	binary.LittleEndian.PutUint32(header[40:44], pcmDataLen)

	_, err := f.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}
	_, err = f.Write(header[:])
	return err
}

func fixWavFile(path string) {
	fi, err := os.Stat(path)
	if err != nil {
		fmt.Printf("⚠️ File not found: %s\n", path)
		return
	}

	totalSize := fi.Size()
	if totalSize < 44 {
		fmt.Printf("⚠️ File %s too small (%d bytes)\n", path, totalSize)
		return
	}

	pcmDataLen := uint32(totalSize - 44)

	f, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		fmt.Printf("❌ Failed to open %s: %v\n", path, err)
		return
	}
	defer f.Close()

	err = writeWAVHeader(f, pcmDataLen, 8000, 1, 16)
	if err != nil {
		fmt.Printf("❌ Failed to write WAV header to %s: %v\n", path, err)
		return
	}
	_ = f.Sync()

	fmt.Printf("✅ REPAIRED %s: Total Size = %d bytes | Audio Data = %d bytes\n", filepath.Base(path), totalSize, pcmDataLen)
}

func mergeSingleMixedWavFile(custPath, agentPath, mergedPath string) error {
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
	err = writeWAVHeader(f, pcmDataLen, 8000, 1, 16)
	if err != nil {
		return err
	}
	_ = f.Sync()

	fmt.Printf("🎧 CREATED SINGLE MIXED CALL RECORDING -> %s (Size: %d bytes | Customer + Agent + Ambience)\n", filepath.Base(mergedPath), len(mixedPCM)+44)
	return nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run .\\cmd\\fixwav <recording_folder_path_or_session_id>")
		fmt.Println("Example: go run .\\cmd\\fixwav \"D:\\Voice AI\\recordings\\5c18d034-6040-4c58-91d7-a70f6fc9b377\"")
		return
	}

	target := os.Args[1]
	dirPath := target

	// If user passed session ID instead of full path
	if !filepath.IsAbs(target) && !os.IsPathSeparator(target[0]) {
		dirPath = filepath.Join("D:/Voice AI/recordings", target)
	}

	fmt.Printf("🔧 Repairing & Merging Call Audio in: %s\n", dirPath)
	custFile := filepath.Join(dirPath, "customer_input.wav")
	agentFile := filepath.Join(dirPath, "agent_output.wav")
	mergedFile := filepath.Join(dirPath, "full_call_recording.wav")

	fixWavFile(custFile)
	fixWavFile(agentFile)

	_ = mergeSingleMixedWavFile(custFile, agentFile, mergedFile)
	fmt.Println("🎉 All done! You can now play full_call_recording.wav in Media Player!")
}
