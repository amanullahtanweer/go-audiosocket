package flow

import (
	"testing"
	"time"
)

// MockSession implements the Session interface for testing
type MockSession struct {
	id string
}

func (m *MockSession) GetID() string {
	return m.id
}

func (m *MockSession) PlayAudio(filename string) error {
	return nil
}

func (m *MockSession) StopTranscription() {
	// Mock implementation
}

func (m *MockSession) GetTranscriptionResults() <-chan TranscriptionResult {
	// Return empty channel for testing
	ch := make(chan TranscriptionResult)
	close(ch)
	return ch
}

func (m *MockSession) ReportStatus(status, reason string) error {
	return nil
}

func TestNewFlowEngine(t *testing.T) {
	session := &MockSession{id: "test-session"}
	
	engine, err := NewFlowEngine(session, "../../config/flow.json")
	if err != nil {
		t.Fatalf("Failed to create flow engine: %v", err)
	}
	
	if engine == nil {
		t.Fatal("Flow engine should not be nil")
	}
	
	if engine.session == nil {
		t.Error("Session should not be nil")
	}
	
	if engine.timer == nil {
		t.Error("Timer should not be nil")
	}
	
	if engine.classifier == nil {
		t.Error("Classifier should not be nil")
	}
	
	if engine.apiClient == nil {
		t.Error("API client should not be nil")
	}
}

func TestResponseClassifier(t *testing.T) {
	classifier := NewResponseClassifier()
	
	testCases := []struct {
		text         string
		expectedType ResponseType
		description  string
	}{
		{"yes", ResponsePositive, "Simple yes"},
		{"I have Medicare", ResponsePositive, "Contains positive keywords"},
		{"no", ResponseNegative, "Simple no"},
		{"I don't have coverage", ResponseNegative, "Contains negative keywords"},
		{"hello world", ResponseUnknown, "Unknown response"},
		{"maybe", ResponsePositive, "Maybe is positive"},
		{"I don't think so", ResponseNegative, "Negative phrase"},
	}
	
	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			result := classifier.ClassifyResponse(tc.text)
			if result != tc.expectedType {
				t.Errorf("Expected %s, got %s for text: %s", tc.expectedType, result, tc.text)
			}
		})
	}
}

func TestGlobalTimer(t *testing.T) {
	timer := NewGlobalTimer(100 * time.Millisecond)
	
	if timer.IsActive() {
		t.Error("Timer should not be active initially")
	}
	
	timer.Start()
	if !timer.IsActive() {
		t.Error("Timer should be active after start")
	}
	
	timer.Reset()
	if !timer.IsActive() {
		t.Error("Timer should be active after reset")
	}
	
	timer.Stop()
	if timer.IsActive() {
		t.Error("Timer should not be active after stop")
	}
}
