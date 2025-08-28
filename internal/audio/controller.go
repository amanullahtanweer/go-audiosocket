package audio

import (
	"log"
	"net"
	"sync"
	"time"
)

// AudioRequest represents an audio playback request
type AudioRequest struct {
	Type     string // "greeting", "ambient", "interrupt"
	Filename string
	Priority int // Higher priority interrupts lower priority audio
}

// AudioController manages all audio playback to prevent overlapping
type AudioController struct {
	player     *Player
	conn       net.Conn
	queue      chan AudioRequest
	stopChan   chan struct{}
	mu         sync.RWMutex
	isPlaying  bool
	currentReq *AudioRequest
}

// NewAudioController creates a new audio controller
func NewAudioController(player *Player, conn net.Conn) *AudioController {
	controller := &AudioController{
		player:   player,
		conn:     conn,
		queue:    make(chan AudioRequest, 10), // Buffer for 10 requests
		stopChan: make(chan struct{}),
	}

	// Start the audio controller
	go controller.run()

	return controller
}

// run processes audio requests from the queue
func (controller *AudioController) run() {
	for {
		select {
		case <-controller.stopChan:
			log.Printf("Audio controller stopped")
			return
		case req := <-controller.queue:
			controller.playAudio(req)
		}
	}
}

// playAudio plays a single audio request
func (controller *AudioController) playAudio(req AudioRequest) {
	controller.mu.Lock()
	controller.isPlaying = true
	controller.currentReq = &req
	controller.mu.Unlock()

	log.Printf("Playing audio: %s (%s)", req.Filename, req.Type)

	// Play the audio file
	if err := controller.player.PlayAudio(controller.conn, req.Filename); err != nil {
		log.Printf("Failed to play audio %s: %v", req.Filename, err)
	} else {
		log.Printf("Completed audio: %s (%s)", req.Filename, req.Type)
	}

	controller.mu.Lock()
	controller.isPlaying = false
	controller.currentReq = nil
	controller.mu.Unlock()
}

// PlayGreeting plays greeting audio
func (controller *AudioController) PlayGreeting() {
	req := AudioRequest{
		Type:     "greeting",
		Filename: "greeting.wav",
		Priority: 1,
	}
	controller.queue <- req
}

// StartAmbientAudio starts continuous ambient audio
func (controller *AudioController) StartAmbientAudio() {
	go func() {
		for {
			select {
			case <-controller.stopChan:
				log.Printf("Ambient audio stopped")
				return
			default:
				// Check if we can play ambient audio (no higher priority audio playing)
				controller.mu.RLock()
				canPlay := !controller.isPlaying || controller.currentReq.Type == "ambient"
				controller.mu.RUnlock()

				if canPlay {
					req := AudioRequest{
						Type:     "ambient",
						Filename: "bg_last30s.wav",
						Priority: 0, // Lowest priority
					}
					controller.queue <- req
				}

				// Wait before next loop
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()
}

// PlayInterrupt plays interruption audio (highest priority)
func (controller *AudioController) PlayInterrupt(filename string) {
	req := AudioRequest{
		Type:     "interrupt",
		Filename: filename,
		Priority: 2, // Highest priority
	}
	controller.queue <- req
}

// Stop stops the audio controller
func (controller *AudioController) Stop() {
	close(controller.stopChan)
}

// IsPlaying returns true if any audio is currently playing
func (controller *AudioController) IsPlaying() bool {
	controller.mu.RLock()
	defer controller.mu.RUnlock()
	return controller.isPlaying
}

// GetCurrentAudio returns the currently playing audio request
func (controller *AudioController) GetCurrentAudio() *AudioRequest {
	controller.mu.RLock()
	defer controller.mu.RUnlock()
	return controller.currentReq
}
