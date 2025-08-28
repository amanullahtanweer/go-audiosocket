package audio

import (
	"testing"
)

func TestPatternMatcherInitialization(t *testing.T) {
	// Test creating a pattern matcher
	matcher, err := NewPatternMatcher("../../config/interrupts.yaml")
	if err != nil {
		t.Fatalf("Failed to create pattern matcher: %v", err)
	}

	if matcher == nil {
		t.Fatal("Pattern matcher should not be nil")
	}

	// Get all interrupts
	interrupts := matcher.GetInterrupts()
	if len(interrupts) == 0 {
		t.Error("Expected interrupts to be loaded from config")
	}

	// Check that DNC interrupt exists
	dnc, exists := interrupts["dnc"]
	if !exists {
		t.Error("Expected DNC interrupt to exist")
	}

	if dnc.Name != "Do Not Call" {
		t.Errorf("Expected DNC name 'Do Not Call', got '%s'", dnc.Name)
	}

	if dnc.AudioFile != "dnc.wav" {
		t.Errorf("Expected DNC audio file 'dnc.wav', got '%s'", dnc.AudioFile)
	}
}

func TestDNCPatternMatching(t *testing.T) {
	matcher, err := NewPatternMatcher("../../config/interrupts.yaml")
	if err != nil {
		t.Fatalf("Failed to create pattern matcher: %v", err)
	}

	// Test exact phrase matching
	testCases := []struct {
		text        string
		shouldMatch bool
		description string
	}{
		{
			text:        "stop calling",
			shouldMatch: true,
			description: "Exact phrase match",
		},
		{
			text:        "Stop calling",
			shouldMatch: true,
			description: "Case insensitive match",
		},
		{
			text:        "STOP CALLING",
			shouldMatch: true,
			description: "All caps match",
		},
		{
			text:        "stop calling me",
			shouldMatch: true,
			description: "Extended phrase match",
		},
		{
			text:        "hello world",
			shouldMatch: false,
			description: "No match",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			result := matcher.DetectInterrupt(tc.text)

			if tc.shouldMatch {
				if result == nil {
					t.Errorf("Expected to detect interrupt for: %s", tc.text)
					return
				}

				if result.Name != "Do Not Call" {
					t.Errorf("Expected DNC interrupt, got: %s", result.Name)
				}
			} else {
				if result != nil {
					t.Errorf("Expected no interrupt detection for: %s, but got: %s", tc.text, result.Name)
				}
			}
		})
	}
}
