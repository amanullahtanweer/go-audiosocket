package metrics

import (
	"fmt"
	"sync"
	"time"
)

type SessionMetrics struct {
	Provider         string
	SessionID        string
	StartTime        time.Time
	EndTime          time.Time
	AudioBytes       int
	TranscriptLength int
	PartialCount     int
	FinalCount       int
	FirstResultTime  *time.Time
	mu               sync.Mutex
}

func NewSessionMetrics(provider, sessionID string) *SessionMetrics {
	return &SessionMetrics{
		Provider:  provider,
		SessionID: sessionID,
		StartTime: time.Now(),
	}
}

func (m *SessionMetrics) AddAudioBytes(bytes int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.AudioBytes += bytes
}

func (m *SessionMetrics) AddTranscriptResult(text string, isFinal bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.FirstResultTime == nil {
		now := time.Now()
		m.FirstResultTime = &now
	}

	m.TranscriptLength += len(text)
	if isFinal {
		m.FinalCount++
	} else {
		m.PartialCount++
	}
}

func (m *SessionMetrics) Finalize() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.EndTime = time.Now()
}

func (m *SessionMetrics) Summary() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	duration := m.EndTime.Sub(m.StartTime)
	var latency time.Duration
	if m.FirstResultTime != nil {
		latency = m.FirstResultTime.Sub(m.StartTime)
	}

	audioDuration := float64(m.AudioBytes) / (8000 * 2) // Assuming 8kHz, 16-bit

	return fmt.Sprintf(
		"Provider: %s\n"+
			"Session: %s\n"+
			"Duration: %v\n"+
			"Audio Duration: %.2f seconds\n"+
			"Audio Bytes: %d\n"+
			"Transcript Length: %d chars\n"+
			"First Result Latency: %v\n"+
			"Partial Results: %d\n"+
			"Final Results: %d\n"+
			"Real-time Factor: %.2fx\n",
		m.Provider,
		m.SessionID,
		duration,
		audioDuration,
		m.AudioBytes,
		m.TranscriptLength,
		latency,
		m.PartialCount,
		m.FinalCount,
		duration.Seconds()/audioDuration,
	)
}
