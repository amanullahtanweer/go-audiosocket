package flow

import (
	"log"
	"time"
)

// GlobalTimer manages the global timeout for user responses
type GlobalTimer struct {
	duration      time.Duration
	timer         *time.Timer
	resetChan     chan struct{}
	timeoutChan   chan struct{}
	isActive      bool
	lastReset     time.Time
	resetDebounce time.Duration // Minimum time between resets
}

// NewGlobalTimer creates a new global timer
func NewGlobalTimer(duration time.Duration) *GlobalTimer {
	return &GlobalTimer{
		duration:      duration,
		resetChan:     make(chan struct{}),
		timeoutChan:   make(chan struct{}),
		isActive:      false,
		resetDebounce: 500 * time.Millisecond, // 500ms debounce
	}
}

// Start starts the timer
func (gt *GlobalTimer) Start() {
	if gt.isActive {
		gt.Stop()
	}

	gt.isActive = true
	gt.timer = time.AfterFunc(gt.duration, func() {
		gt.timeoutChan <- struct{}{}
		gt.isActive = false
	})

	// log.Printf("Global timer started: %v", gt.duration)
}

// Stop stops the timer
func (gt *GlobalTimer) Stop() {
	if gt.timer != nil {
		gt.timer.Stop()
		gt.timer = nil
	}
	gt.isActive = false
	// log.Printf("Global timer stopped")
}

// Reset resets the timer (stops current, starts new)
func (gt *GlobalTimer) Reset() {
	// Check if enough time has passed since last reset
	if time.Since(gt.lastReset) < gt.resetDebounce {
		return // Skip reset if too soon
	}

	if gt.isActive {
		gt.Stop()
	}
	gt.Start()
	gt.lastReset = time.Now()
	log.Printf("Global timer reset")
}

// IsActive returns whether the timer is currently active
func (gt *GlobalTimer) IsActive() bool {
	return gt.isActive
}

// GetTimeoutChan returns the channel for timeout events
func (gt *GlobalTimer) GetTimeoutChan() <-chan struct{} {
	return gt.timeoutChan
}

// GetResetChan returns the channel for reset signals
func (gt *GlobalTimer) GetResetChan() chan<- struct{} {
	return gt.resetChan
}

// GetDuration returns the timer duration
func (gt *GlobalTimer) GetDuration() time.Duration {
	return gt.duration
}
