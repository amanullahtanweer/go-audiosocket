package audio

import (
	"os"
	"testing"
)

func TestNewPlayer(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "audio_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Test creating player with empty directory
	player, err := NewPlayer(tempDir)
	if err != nil {
		t.Fatalf("Failed to create player with empty dir: %v", err)
	}

	if player == nil {
		t.Fatal("Player should not be nil")
	}

	if len(player.audioCache) != 0 {
		t.Errorf("Expected empty cache, got %d files", len(player.audioCache))
	}
}

func TestLoadWAVFile(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "audio_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	player := &Player{
		audioCache: make(map[string][]byte),
		audioDir:   tempDir,
	}

	// Test loading non-existent file
	_, err = player.loadWAVFile("nonexistent.wav")
	if err == nil {
		t.Error("Expected error when loading non-existent file")
	}
}
