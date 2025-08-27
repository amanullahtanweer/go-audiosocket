package transcriber

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	AssemblyAIWebSocketURL = "wss://streaming.assemblyai.com/v3/ws"
	// AssemblyAI requires chunks between 50ms and 1000ms
	MinChunkDurationMs = 50
	MaxChunkDurationMs = 1000
)

type AssemblyAITranscriber struct {
	conn        *websocket.Conn
	results     chan TranscriptionResult
	fullText    strings.Builder
	mu          sync.Mutex
	sampleRate  int
	apiKey      string
	sessionID   string
	audioBuffer []byte
	bufferMu    sync.Mutex
	sendTicker  *time.Ticker
	stopSending chan struct{}
	wg          sync.WaitGroup
}

// AssemblyAI message types
type AssemblyAIMessage struct {
	Type               string  `json:"type"`
	ID                 string  `json:"id,omitempty"`
	ExpiresAt          int64   `json:"expires_at,omitempty"`
	Transcript         string  `json:"transcript,omitempty"`
	TurnIsFormatted    bool    `json:"turn_is_formatted,omitempty"`
	AudioDurationSec   float64 `json:"audio_duration_seconds,omitempty"`
	SessionDurationSec float64 `json:"session_duration_seconds,omitempty"`
}

func NewAssemblyAITranscriber(apiKey string, sampleRate int) (*AssemblyAITranscriber, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("AssemblyAI API key is required")
	}

	// AssemblyAI expects 16kHz, so we'll need to resample if input is 8kHz
	targetSampleRate := 16000

	// Connect to AssemblyAI WebSocket
	url := fmt.Sprintf("%s?sample_rate=%d&format_turns=true", AssemblyAIWebSocketURL, targetSampleRate)

	header := http.Header{}
	header.Add("Authorization", apiKey)

	conn, _, err := websocket.DefaultDialer.Dial(url, header)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to AssemblyAI: %w", err)
	}

	at := &AssemblyAITranscriber{
		conn:        conn,
		results:     make(chan TranscriptionResult, 100),
		sampleRate:  sampleRate,
		apiKey:      apiKey,
		audioBuffer: make([]byte, 0, 8000), // Buffer for ~100ms at 16kHz
		stopSending: make(chan struct{}),
	}

	// Start result handler
	go at.handleResults()

	// Start audio sender goroutine
	// This will send buffered audio every 50ms to reduce latency
	at.wg.Add(1)
	go at.audioSender()

	log.Println("AssemblyAI transcriber initialized")

	return at, nil
}

func (at *AssemblyAITranscriber) audioSender() {
	defer at.wg.Done()

	// Send audio every 50ms to minimize latency while respecting AssemblyAI limits
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			at.sendBufferedAudio()
		case <-at.stopSending:
			// Send any remaining buffered audio before stopping
			at.sendBufferedAudio()
			return
		}
	}
}

func (at *AssemblyAITranscriber) sendBufferedAudio() {
	at.bufferMu.Lock()
	defer at.bufferMu.Unlock()

	// Calculate chunk size limits based on AssemblyAI requirements
	// At 16kHz, 16-bit audio (2 bytes per sample):
	// Min 50ms = 0.05 * 16000 * 2 = 1600 bytes
	// Max 950ms = 0.95 * 16000 * 2 = 30400 bytes (staying under 1000ms limit)
	const minChunkSize = 1600
	const maxChunkSize = 30400
	
	// Only send if we have at least the minimum chunk size
	// This prevents sending chunks that are too small
	if len(at.audioBuffer) < minChunkSize {
		return
	}
	
	// Send audio in chunks that respect AssemblyAI's duration limits
	for len(at.audioBuffer) >= minChunkSize {
		chunkSize := len(at.audioBuffer)
		if chunkSize > maxChunkSize {
			chunkSize = maxChunkSize
		}
		
		// Extract chunk to send
		chunk := at.audioBuffer[:chunkSize]
		
		// Send the chunk
		if err := at.conn.WriteMessage(websocket.BinaryMessage, chunk); err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Printf("Failed to send audio to AssemblyAI: %v", err)
			}
			// Clear buffer on error to avoid infinite loop
			at.audioBuffer = at.audioBuffer[:0]
			return
		}
		
		// Remove sent chunk from buffer
		at.audioBuffer = at.audioBuffer[chunkSize:]
	}
}

func (at *AssemblyAITranscriber) ProcessAudio(audioData []byte) error {
	at.bufferMu.Lock()
	defer at.bufferMu.Unlock()

	// If input is 8kHz, we need to resample to 16kHz for AssemblyAI
	processedData := audioData
	if at.sampleRate == 8000 {
		processedData = at.resample8to16(audioData)
	}

	// Add to buffer
	at.audioBuffer = append(at.audioBuffer, processedData...)

	return nil
}

// Simple upsampling from 8kHz to 16kHz (linear interpolation)
func (at *AssemblyAITranscriber) resample8to16(input []byte) []byte {
	// Convert bytes to int16 samples
	samples := make([]int16, len(input)/2)
	for i := 0; i < len(samples); i++ {
		samples[i] = int16(binary.LittleEndian.Uint16(input[i*2 : i*2+2]))
	}

	// Upsample by factor of 2 (8kHz -> 16kHz)
	upsampled := make([]int16, len(samples)*2)
	for i := 0; i < len(samples)-1; i++ {
		upsampled[i*2] = samples[i]
		// Linear interpolation for the sample in between
		upsampled[i*2+1] = (samples[i] + samples[i+1]) / 2
	}
	// Handle last sample
	if len(samples) > 0 {
		upsampled[len(upsampled)-2] = samples[len(samples)-1]
		upsampled[len(upsampled)-1] = samples[len(samples)-1]
	}

	// Convert back to bytes
	output := make([]byte, len(upsampled)*2)
	for i, sample := range upsampled {
		binary.LittleEndian.PutUint16(output[i*2:i*2+2], uint16(sample))
	}

	return output
}

func (at *AssemblyAITranscriber) handleResults() {
	for {
		_, message, err := at.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("AssemblyAI WebSocket error: %v", err)
			}
			close(at.results)
			return
		}

		var msg AssemblyAIMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("Failed to parse AssemblyAI message: %v", err)
			continue
		}

		switch msg.Type {
		case "Begin":
			at.sessionID = msg.ID
			log.Printf("AssemblyAI session started: %s", msg.ID)

		case "Turn":
			if msg.Transcript != "" {
				// Handle both formatted and unformatted transcripts
				if msg.TurnIsFormatted {
					// This is a final, formatted transcript
					at.mu.Lock()
					if at.fullText.Len() > 0 {
						at.fullText.WriteString(" ")
					}
					at.fullText.WriteString(msg.Transcript)
					at.mu.Unlock()

					at.results <- TranscriptionResult{
						Text:    msg.Transcript,
						IsFinal: true,
					}
				} else {
					// This is a partial transcript
					at.results <- TranscriptionResult{
						Text:    msg.Transcript,
						IsFinal: false,
					}
				}
			}

		case "Termination":
			log.Printf("AssemblyAI session terminated. Audio duration: %.2fs, Session duration: %.2fs",
				msg.AudioDurationSec, msg.SessionDurationSec)
		}
	}
}

func (at *AssemblyAITranscriber) Results() <-chan TranscriptionResult {
	return at.results
}

func (at *AssemblyAITranscriber) GetFullTranscript() string {
	at.mu.Lock()
	defer at.mu.Unlock()
	return at.fullText.String()
}

func (at *AssemblyAITranscriber) AddMarker(marker string) {
	at.mu.Lock()
	defer at.mu.Unlock()

	if at.fullText.Len() > 0 {
		at.fullText.WriteString(" ")
	}
	at.fullText.WriteString(marker)
}

func (at *AssemblyAITranscriber) Close() error {
	// Stop the audio sender
	close(at.stopSending)
	at.wg.Wait()

	// Send any remaining audio in buffer (even if less than minimum)
	at.bufferMu.Lock()
	if len(at.audioBuffer) > 0 {
		// Try to send remaining audio, but don't fail close if it errors
		_ = at.conn.WriteMessage(websocket.BinaryMessage, at.audioBuffer)
		at.audioBuffer = at.audioBuffer[:0]
	}
	at.bufferMu.Unlock()

	// Send termination message to AssemblyAI
	terminateMsg := AssemblyAIMessage{
		Type: "Terminate",
	}

	msgBytes, err := json.Marshal(terminateMsg)
	if err == nil {
		at.conn.WriteMessage(websocket.TextMessage, msgBytes)
		// Give AssemblyAI time to process termination
		time.Sleep(500 * time.Millisecond)
	}

	return at.conn.Close()
}
