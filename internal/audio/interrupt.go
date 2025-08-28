package audio

import (
	"log"
	"net"
	"strings"
	"sync"
)

// InterruptType represents the type of call interruption
type InterruptType string

const (
	InterruptDNC      InterruptType = "dnc"      // Do Not Call
	InterruptRobot    InterruptType = "robot"    // Robot detection
	InterruptNI       InterruptType = "ni"       // Not Interested
	InterruptCallback InterruptType = "callback" // Call back later
)

// InterruptRule defines a keyword detection rule
type InterruptRule struct {
	Type        InterruptType
	Keywords    []string
	AudioFile   string
	Description string
}

// InterruptDetector handles keyword detection and audio response
type InterruptDetector struct {
	rules            []InterruptRule
	mu               sync.RWMutex
	isPlaying        bool
	currentInterrupt InterruptType
	stopChan         chan struct{}
	player           *Player
}

// NewInterruptDetector creates a new interrupt detector
func NewInterruptDetector(player *Player) *InterruptDetector {
	detector := &InterruptDetector{
		player:   player,
		stopChan: make(chan struct{}),
		rules:    make([]InterruptRule, 0),
	}

	// Initialize the 4 fixed interruption rules
	detector.initializeRules()

	return detector
}

// initializeRules sets up the predefined interruption rules
func (detector *InterruptDetector) initializeRules() {
	detector.rules = []InterruptRule{
		{
			Type:        InterruptDNC,
			Keywords:    []string{"dont call me", "stop calling me", "put me off the list", "remove me", "unsubscribe"},
			AudioFile:   "dnc.wav",
			Description: "Do Not Call - Customer wants to be removed from calling list",
		},
		{
			Type:        InterruptRobot,
			Keywords:    []string{"are you a robot", "am i talking to a robot", "are you human", "is this automated", "robot voice"},
			AudioFile:   "robot.wav",
			Description: "Robot Detection - Customer suspects automated system",
		},
		{
			Type:        InterruptNI,
			Keywords:    []string{"i am not interested", "i am annoyed", "not interested", "dont want", "waste of time"},
			AudioFile:   "bye.wav",
			Description: "Not Interested - Customer wants to end call",
		},
		{
			Type:        InterruptCallback,
			Keywords:    []string{"i am busy", "in a meeting", "call me back later", "call back", "busy now"},
			AudioFile:   "transfer.wav",
			Description: "Callback Request - Customer wants call back later",
		},
	}

	log.Printf("Initialized %d interruption rules", len(detector.rules))
}

// DetectInterrupt checks if the given text contains any interruption keywords
func (detector *InterruptDetector) DetectInterrupt(text string) *InterruptRule {
	detector.mu.RLock()
	defer detector.mu.RUnlock()

	// If already playing an interrupt, don't detect new ones
	if detector.isPlaying {
		return nil
	}

	// Convert text to lowercase for case-insensitive matching
	lowerText := strings.ToLower(text)

	// Check each rule for keyword matches
	for _, rule := range detector.rules {
		for _, keyword := range rule.Keywords {
			if strings.Contains(lowerText, keyword) {
				log.Printf("Interrupt detected: %s - '%s' matched keyword '%s'",
					rule.Type, text, keyword)
				return &rule
			}
		}
	}

	return nil
}

// PlayInterrupt plays the audio for the detected interruption
func (detector *InterruptDetector) PlayInterrupt(rule *InterruptRule, conn net.Conn) error {
	detector.mu.Lock()

	// If already playing, don't start another
	if detector.isPlaying {
		detector.mu.Unlock()
		log.Printf("Interrupt already playing (%s), ignoring new request (%s)",
			detector.currentInterrupt, rule.Type)
		return nil
	}

	// Mark as playing
	detector.isPlaying = true
	detector.currentInterrupt = rule.Type
	detector.mu.Unlock()

	log.Printf("Playing interrupt audio: %s (%s)", rule.AudioFile, rule.Description)

	// Play the audio file
	if err := detector.player.PlayAudio(conn, rule.AudioFile); err != nil {
		detector.mu.Lock()
		detector.isPlaying = false
		detector.currentInterrupt = ""
		detector.mu.Unlock()
		return err
	}

	// Mark as no longer playing
	detector.mu.Lock()
	detector.isPlaying = false
	detector.currentInterrupt = ""
	detector.mu.Unlock()

	log.Printf("Interrupt audio completed: %s", rule.Type)
	return nil
}

// IsPlaying returns true if an interrupt audio is currently playing
func (detector *InterruptDetector) IsPlaying() bool {
	detector.mu.RLock()
	defer detector.mu.RUnlock()
	return detector.isPlaying
}

// GetCurrentInterrupt returns the currently playing interrupt type
func (detector *InterruptDetector) GetCurrentInterrupt() InterruptType {
	detector.mu.RLock()
	defer detector.mu.RUnlock()
	return detector.currentInterrupt
}

// Stop stops any currently playing interrupt
func (detector *InterruptDetector) Stop() {
	detector.mu.Lock()
	defer detector.mu.Unlock()

	if detector.isPlaying {
		close(detector.stopChan)
		detector.isPlaying = false
		detector.currentInterrupt = ""
		// Create new stop channel for future use
		detector.stopChan = make(chan struct{})
	}
}
