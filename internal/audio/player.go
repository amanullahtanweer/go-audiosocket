package audio

/*
🚨 CRITICAL AUDIO RULES - NEVER FORGET! 🚨

AudioSocket Audio Playback Rules:
- ALWAYS use audiosocket.DefaultSlinChunkSize = 320 bytes
- NEVER use custom chunk sizes like 160 bytes
- 320 bytes = 8000Hz × 20ms × 2 bytes (correct)
- 160 bytes = 8000Hz × 10ms × 2 bytes (WRONG - causes slow motion!)
- Use audiosocket.SendSlinChunks() - NOT custom implementations

See CODE_RULES.md for complete documentation.
*/

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/CyCoreSystems/audiosocket"
)

// Player handles audio file loading and playback
type Player struct {
	audioCache map[string][]byte
	mutex      sync.RWMutex
	audioDir   string
}

// NewPlayer creates a new audio player instance
func NewPlayer(audioDir string) (*Player, error) {
	player := &Player{
		audioCache: make(map[string][]byte),
		audioDir:   audioDir,
	}

	// Pre-load audio files
	if err := player.preloadAudioFiles(); err != nil {
		return nil, fmt.Errorf("failed to preload audio files: %w", err)
	}

	return player, nil
}

// preloadAudioFiles loads all WAV files from the audio directory into memory
func (p *Player) preloadAudioFiles() error {
	files, err := filepath.Glob(filepath.Join(p.audioDir, "*.wav"))
	if err != nil {
		return fmt.Errorf("failed to glob audio files: %w", err)
	}

	backgroundFiles, err := filepath.Glob(filepath.Join(p.audioDir, "background", "*.wav"))
	if err != nil {
		return fmt.Errorf("failed to glob background audio files: %w", err)
	}

	// Combine all audio files
	allFiles := append(files, backgroundFiles...)

	for _, file := range allFiles {
		filename := filepath.Base(file)
		audioData, err := p.loadWAVFile(file)
		if err != nil {
			log.Printf("Warning: Failed to load audio file %s: %v", filename, err)
			continue
		}

		p.mutex.Lock()
		p.audioCache[filename] = audioData
		p.mutex.Unlock()

		log.Printf("Loaded audio file: %s (%d bytes)", filename, len(audioData))
	}

	return nil
}

// loadWAVFile reads a WAV file and returns the raw PCM data
func (p *Player) loadWAVFile(filepath string) ([]byte, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Read WAV header to find data chunk
	header := make([]byte, 44)
	if _, err := io.ReadFull(file, header); err != nil {
		return nil, fmt.Errorf("failed to read WAV header: %w", err)
	}

	// Verify it's a WAV file
	if string(header[0:4]) != "RIFF" || string(header[8:12]) != "WAVE" {
		return nil, fmt.Errorf("not a valid WAV file")
	}

	// Find the data chunk
	dataStart := 44
	for i := 12; i < len(header)-4; i++ {
		if string(header[i:i+4]) == "data" {
			dataStart = i + 8
			break
		}
	}

	// Seek to data chunk and read PCM data
	if _, err := file.Seek(int64(dataStart), 0); err != nil {
		return nil, fmt.Errorf("failed to seek to data chunk: %w", err)
	}

	return io.ReadAll(file)
}

// GetAudio returns cached audio data for a given filename
func (p *Player) GetAudio(filename string) ([]byte, bool) {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	audioData, exists := p.audioCache[filename]
	return audioData, exists
}

// PlayAudio sends audio data through the AudioSocket connection
func (p *Player) PlayAudio(conn net.Conn, filename string) error {
	audioData, exists := p.GetAudio(filename)
	if !exists {
		return fmt.Errorf("audio file not found: %s", filename)
	}

	// Use the built-in SendSlinChunks function with correct chunk size
	// DefaultSlinChunkSize = 320 bytes (8000Hz * 20ms * 2 bytes)
	if err := audiosocket.SendSlinChunks(conn, audiosocket.DefaultSlinChunkSize, audioData); err != nil {
		return fmt.Errorf("failed to send audio: %w", err)
	}

	log.Printf("Played audio file: %s (%d bytes)", filename, len(audioData))
	return nil
}

// PlayGreeting plays the greeting audio when a call connects
func (p *Player) PlayGreeting(conn net.Conn) error {
	// Try different greeting files in order of preference
	greetingFiles := []string{"greeting.wav", "hello.wav"}

	for _, filename := range greetingFiles {
		if _, exists := p.GetAudio(filename); exists {
			return p.PlayAudio(conn, filename)
		}
	}

	return fmt.Errorf("no greeting audio file found")
}

// StartAmbientAudio starts playing background ambient audio continuously
// DISABLED FOR NOW - Will be re-enabled later when we solve the audio mixing issues
func (p *Player) StartAmbientAudio(conn net.Conn, stopChan <-chan struct{}) {
	log.Printf("Ambient audio DISABLED - will be re-enabled later")
	// TODO: Re-enable ambient audio when we solve the audio mixing issues
	return
}

// PlayAmbientAudioWithPause plays ambient audio with frequent pause checks
func (p *Player) PlayAmbientAudioWithPause(conn net.Conn, filename string, pauseChan <-chan struct{}, stopChan <-chan struct{}) error {
	audioData, exists := p.GetAudio(filename)
	if !exists {
		return fmt.Errorf("audio file not found: %s", filename)
	}

	// For 8kHz audio, send in 20ms chunks (320 bytes = 8000Hz * 0.02s * 2 bytes)
	chunkSize := audiosocket.DefaultSlinChunkSize

	// Send chunks with frequent pause checks
	for i := 0; i < len(audioData); i += chunkSize {
		// Check for pause/stop signals before each chunk
		select {
		case <-pauseChan:
			log.Printf("Ambient audio paused mid-playback")
			return nil
		case <-stopChan:
			log.Printf("Ambient audio stopped mid-playback")
			return nil
		default:
			// Continue playing
		}

		end := i + chunkSize
		if end > len(audioData) {
			end = len(audioData)
		}

		chunk := audioData[i:end]
		if _, err := conn.Write(audiosocket.SlinMessage(chunk)); err != nil {
			return fmt.Errorf("failed to send ambient audio chunk: %w", err)
		}

		// Small delay between chunks
		time.Sleep(20 * time.Millisecond)
	}

	return nil
}
