package audio

import (
	"testing"
)

func TestInterruptDetectorInitialization(t *testing.T) {
	// Create a mock player
	mockPlayer := &Player{
		audioCache: make(map[string][]byte),
		audioDir:   "./test_audio",
	}

	detector := NewInterruptDetector(mockPlayer)

	if detector == nil {
		t.Fatal("InterruptDetector should not be nil")
	}

	if len(detector.rules) != 4 {
		t.Errorf("Expected 4 rules, got %d", len(detector.rules))
	}

	// Check that all expected rule types are present
	expectedTypes := map[InterruptType]bool{
		InterruptDNC:      false,
		InterruptRobot:    false,
		InterruptNI:       false,
		InterruptCallback: false,
	}

	for _, rule := range detector.rules {
		expectedTypes[rule.Type] = true
	}

	for interruptType, found := range expectedTypes {
		if !found {
			t.Errorf("Missing interrupt type: %s", interruptType)
		}
	}
}

func TestInterruptDetection(t *testing.T) {
	mockPlayer := &Player{
		audioCache: make(map[string][]byte),
		audioDir:   "./test_audio",
	}

	detector := NewInterruptDetector(mockPlayer)

	// Test DNC detection
	testCases := []struct {
		text         string
		expectedType InterruptType
		expectedFile string
		shouldDetect bool
	}{
		{
			text:         "dont call me anymore",
			expectedType: InterruptDNC,
			expectedFile: "dnc.wav",
			shouldDetect: true,
		},
		{
			text:         "are you a robot?",
			expectedType: InterruptRobot,
			expectedFile: "robot.wav",
			shouldDetect: true,
		},
		{
			text:         "i am not interested",
			expectedType: InterruptNI,
			expectedFile: "bye.wav",
			shouldDetect: true,
		},
		{
			text:         "call me back later",
			expectedType: InterruptCallback,
			expectedFile: "transfer.wav",
			shouldDetect: true,
		},
		{
			text:         "hello, how are you?",
			expectedType: "",
			expectedFile: "",
			shouldDetect: false,
		},
		{
			text:         "I AM BUSY RIGHT NOW",
			expectedType: InterruptCallback,
			expectedFile: "transfer.wav",
			shouldDetect: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.text, func(t *testing.T) {
			rule := detector.DetectInterrupt(tc.text)

			if tc.shouldDetect {
				if rule == nil {
					t.Errorf("Expected to detect interrupt for: %s", tc.text)
					return
				}

				if rule.Type != tc.expectedType {
					t.Errorf("Expected type %s, got %s", tc.expectedType, rule.Type)
				}

				if rule.AudioFile != tc.expectedFile {
					t.Errorf("Expected audio file %s, got %s", tc.expectedFile, rule.AudioFile)
				}
			} else {
				if rule != nil {
					t.Errorf("Expected no interrupt detection for: %s, but got %s", tc.text, rule.Type)
				}
			}
		})
	}
}

func TestInterruptPrevention(t *testing.T) {
	mockPlayer := &Player{
		audioCache: make(map[string][]byte),
		audioDir:   "./test_audio",
	}

	detector := NewInterruptDetector(mockPlayer)

	// Simulate an interrupt already playing
	detector.mu.Lock()
	detector.isPlaying = true
	detector.currentInterrupt = InterruptDNC
	detector.mu.Unlock()

	// Try to detect another interrupt
	rule := detector.DetectInterrupt("are you a robot?")
	if rule != nil {
		t.Error("Should not detect new interrupt when one is already playing")
	}

	// Reset state
	detector.mu.Lock()
	detector.isPlaying = false
	detector.currentInterrupt = ""
	detector.mu.Unlock()

	// Now should detect
	rule = detector.DetectInterrupt("are you a robot?")
	if rule == nil {
		t.Error("Should detect interrupt when none is playing")
	}
}

func TestKeywordVariations(t *testing.T) {
	mockPlayer := &Player{
		audioCache: make(map[string][]byte),
		audioDir:   "./test_audio",
	}

	detector := NewInterruptDetector(mockPlayer)

	// Test various keyword variations
	variations := []string{
		"stop calling me",
		"STOP CALLING ME",
		"Stop Calling Me",
		"please stop calling me",
		"i want you to stop calling me",
	}

	for _, variation := range variations {
		rule := detector.DetectInterrupt(variation)
		if rule == nil {
			t.Errorf("Failed to detect DNC interrupt for variation: %s", variation)
		} else if rule.Type != InterruptDNC {
			t.Errorf("Expected DNC interrupt for variation: %s, got %s", variation, rule.Type)
		}
	}
}
