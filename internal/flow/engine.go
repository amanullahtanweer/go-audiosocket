package flow

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"time"
)

// FlowEngine manages the call flow execution
type FlowEngine struct {
    session     Session
    currentNode *FlowNode
    config      *FlowConfig
    timer       *GlobalTimer
    isActive    bool
    classifier  *ResponseClassifier
    waitingFor  *FlowNode // Node we're currently waiting for response on
    apiClient   *APIClient
    logger      *SessionLogger
    lastReason  string // tracks last flow reason for hangup reporting
    transferred bool   // track if transfer occurred to avoid DC fallback
}

// FlowNode represents a single step in the flow
type FlowNode struct {
	ID          string            `json:"id"`
	Type        string            `json:"type"`    // audio, question, transfer, hangup, interrupt
	Content     string            `json:"content"` // Human readable description
	AudioFile   string            `json:"audio_file"`
	Transitions map[string]string `json:"transitions"`
	Actions     []Action          `json:"actions"`
}

// Action represents an action to execute when a node is processed
type Action struct {
	Type     string            `json:"type"`     // api_call, log, transfer
	Endpoint string            `json:"endpoint"` // For API calls
	Method   string            `json:"method"`   // GET, POST, etc.
	Message  string            `json:"message"`  // For logging
	Priority string            `json:"priority"` // For API calls
	Timeout  int               `json:"timeout"`  // For transfers
	Params   map[string]string `json:"params"`   // Additional parameters
}

// FlowConfig represents the entire flow configuration
type FlowConfig struct {
	Metadata FlowMetadata `json:"metadata"`
	Nodes    []FlowNode   `json:"nodes"`
}

// FlowMetadata contains flow information
type FlowMetadata struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

// Session interface for flow engine to interact with server session
type Session interface {
    GetID() string
    PlayAudio(filename string) error
    StopAudio() error // Stops current audio playback
    StopTranscription()
    GetTranscriptionResults() <-chan TranscriptionResult
    ReportStatus(status, reason string) error
    CheckForInterrupt(text string) (string, bool) // Returns interrupt type and whether found
    EndCall() error                               // Ends the call by sending hangup command
}

// TranscriptionResult represents a transcription result
type TranscriptionResult struct {
	Text      string
	IsFinal   bool
	Timestamp time.Time
}

// NewFlowEngine creates a new flow engine instance
func NewFlowEngine(session Session, configPath string) (*FlowEngine, error) {
	// Load flow configuration
	config, err := loadFlowConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load flow config: %w", err)
	}

	// Create global timer
	timer := NewGlobalTimer(15 * time.Second)

	// Create response classifier
	classifier := NewResponseClassifier()

	// Create API client (baseURL will be configured later)
    // Placeholder; server will configure vicidial client via SetAPIClient
    apiClient := NewVicidialClient("", "", "", "", "igent", "test", "", "")

    engine := &FlowEngine{
        session:    session,
        config:     config,
        timer:      timer,
        isActive:   false,
        classifier: classifier,
        apiClient:  apiClient,
    }

	return engine, nil
}

// SetSessionLogger provides a logger to persist structured session events
func (fe *FlowEngine) SetSessionLogger(logger *SessionLogger) {
    fe.logger = logger
    if fe.apiClient != nil {
        fe.apiClient.SetLogger(logger)
    }
}

// SetAPIClient lets server provide a configured Vicidial client
func (fe *FlowEngine) SetAPIClient(client *APIClient) {
    fe.apiClient = client
    if fe.logger != nil {
        fe.apiClient.SetLogger(fe.logger)
    }
}

// GetSessionLogger returns the session logger if configured
func (fe *FlowEngine) GetSessionLogger() *SessionLogger { return fe.logger }

// loadFlowConfig loads flow configuration from JSON file
func loadFlowConfig(configPath string) (*FlowConfig, error) {
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config FlowConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &config, nil
}

// Start begins the flow execution
func (fe *FlowEngine) Start() error {
    fe.isActive = true

	// Find start node
	startNode := fe.findNode("start")
	if startNode == nil {
		return fmt.Errorf("start node not found in flow configuration")
	}

    fe.currentNode = startNode
    log.Printf("Flow started for session %s", fe.session.GetID())

    // Structured log
    if fe.logger != nil {
        fe.logger.LogFlowStart(fe.session.GetID(), fe.config.Metadata.Name, fe.config.Metadata.Version, time.Now())
    }

	// Execute start node
	return fe.executeNode(startNode)
}

// findNode finds a node by ID
func (fe *FlowEngine) findNode(id string) *FlowNode {
	for _, node := range fe.config.Nodes {
		if node.ID == id {
			return &node
		}
	}
	return nil
}

// executeNode executes a single flow node
func (fe *FlowEngine) executeNode(node *FlowNode) error {
    log.Printf("Executing node: %s (type: %s)", node.ID, node.Type)

    if fe.logger != nil {
        fe.logger.LogNodeStart(fe.session.GetID(), node)
    }

	switch node.Type {
	case "audio":
		return fe.handleAudioNode(node)
	case "question":
		return fe.handleQuestionNode(node)
	case "transfer":
		return fe.handleTransferNode(node)
	case "hangup":
		return fe.handleHangupNode(node)
	case "interrupt":
		return fe.handleInterruptNode(node)
	default:
		return fmt.Errorf("unknown node type: %s", node.Type)
	}
}

// handleAudioNode handles audio-only nodes
func (fe *FlowEngine) handleAudioNode(node *FlowNode) error {
	log.Printf("Playing audio: %s - %s", node.AudioFile, node.Content)

	// Play audio in background (non-blocking)
	go func() {
		if err := fe.session.PlayAudio(node.AudioFile); err != nil {
			log.Printf("Failed to play audio: %v", err)
		}
	}()

	// Move to next node immediately (don't wait for audio)
	nextNodeID := node.Transitions["default"]
	if nextNodeID == "" {
		return fmt.Errorf("no default transition for audio node %s", node.ID)
	}

	nextNode := fe.findNode(nextNodeID)
	if nextNode == nil {
		return fmt.Errorf("next node %s not found", nextNodeID)
	}

	fe.currentNode = nextNode
	return fe.executeNode(nextNode)
}

// handleQuestionNode handles question nodes (wait for response)
func (fe *FlowEngine) handleQuestionNode(node *FlowNode) error {
    log.Printf("Playing question audio: %s - %s", node.AudioFile, node.Content)

	// Play audio in background (non-blocking)
	go func() {
		if err := fe.session.PlayAudio(node.AudioFile); err != nil {
			log.Printf("Failed to play audio: %v", err)
		}
	}()

	// Start timer for response
	fe.timer.Start()

	// Wait for response or timeout (can interrupt audio)
	// This runs in the same goroutine as the flow engine
	fe.waitForResponse(node)

	return nil
}

// waitForResponse waits for user response or timeout
func (fe *FlowEngine) waitForResponse(node *FlowNode) {
	fe.waitingFor = node

	// Log what question we're waiting for
	log.Printf("Waiting for response to: %s (Node: %s)", node.Content, node.ID)

	// Listen for transcription results
	transcriptionChan := fe.session.GetTranscriptionResults()

	for {
		select {
		case result := <-transcriptionChan:
			if !result.IsFinal {
				// Partial transcript - only reset timer for substantial partials
				// This prevents excessive resets and premature flow transitions
				if fe.timer.IsActive() && len(result.Text) > 10 {
					fe.timer.Reset()
				}
				continue
			}

			// Final transcript - check for interrupts first
            if interruptType, found := fe.session.CheckForInterrupt(result.Text); found {
                log.Printf("Q&A INTERRUPT - Question: %s | Answer: %s | Interrupt: %s | Node: %s",
                    node.Content, result.Text, interruptType, node.ID)
                // Map interrupt to hangup reason codes used by Vicidial
                switch interruptType {
                case "dnc":
                    fe.lastReason = "DNC"
                case "not_interested":
                    fe.lastReason = "NI"
                case "robot":
                    fe.lastReason = "DNQ"
                case "amd":
                    fe.lastReason = "A"
                case "callback":
                    fe.lastReason = "CALLBK"
                default:
                    fe.lastReason = "DNQ"
                }
                if fe.logger != nil {
                    fe.logger.LogInterrupt(fe.session.GetID(), node, result.Text, interruptType)
                }
                fe.HandleInterrupt(interruptType)
                return
            }

			// No interrupt - classify response
			responseType := fe.classifier.ClassifyResponse(result.Text)

			// Log Question & Answer for training/inspection
            log.Printf("Q&A LOG - Question: %s | Answer: %s | Classification: %s | Node: %s",
                node.Content, result.Text, responseType, node.ID)
            if fe.logger != nil {
                fe.logger.LogQnA(fe.session.GetID(), node, result.Text, string(responseType))
            }

			// Find next node based on response type
			nextNodeID := node.Transitions[string(responseType)]
			if nextNodeID == "" {
				// Fallback to default transition
				nextNodeID = node.Transitions["default"]
			}

            if nextNodeID != "" {
                nextNode := fe.findNode(nextNodeID)
                if nextNode != nil {
                    log.Printf("Flow transition: %s (%s) -> %s (%s) | Response: %s",
                        node.ID, node.Content, nextNode.ID, nextNode.Content, responseType)
                    if fe.logger != nil {
                        fe.logger.LogTransition(fe.session.GetID(), node, nextNode, string(responseType))
                    }
                    // Track reason based on classification for later hangup reporting if not interrupted
                    switch string(responseType) {
                    case "negative":
                        fe.lastReason = "NI"
                    case "unknown":
                        // leave as-is
                    }

					// Stop current audio completely before transitioning
					if fe.waitingFor != nil {
						if err := fe.session.StopAudio(); err != nil {
							log.Printf("Warning: Failed to stop audio: %v", err)
						}
						
						// Small delay to ensure audio stops completely
						time.Sleep(100 * time.Millisecond)
					}

					fe.timer.Stop()
					fe.waitingFor = nil
					fe.currentNode = nextNode
					fe.executeNode(nextNode)
					return
				}
			}

        case <-fe.timer.GetTimeoutChan():
            // Timer expired - handle timeout
            log.Printf("Q&A TIMEOUT - Question: %s | Answer: [TIMEOUT] | Classification: [TIMEOUT] | Node: %s",
                node.Content, node.ID)
            if fe.logger != nil {
                fe.logger.LogTimeout(fe.session.GetID(), node)
            }
            fe.handleTimeout()
            return
        }
    }
}

// handleTimeout handles timeout events
func (fe *FlowEngine) handleTimeout() {
	if fe.waitingFor == nil {
		return
	}

	// Stop current audio before timeout transition
	if err := fe.session.StopAudio(); err != nil {
		log.Printf("Warning: Failed to stop audio during timeout: %v", err)
	}
	
	// Small delay to ensure audio stops completely
	time.Sleep(100 * time.Millisecond)

	// Find timeout transition
	nextNodeID := fe.waitingFor.Transitions["timeout"]
	if nextNodeID == "" {
		// Default timeout behavior - end call
		nextNodeID = "end_call"
	}

	nextNode := fe.findNode(nextNodeID)
	if nextNode != nil {
		fe.waitingFor = nil
		fe.currentNode = nextNode
		fe.executeNode(nextNode)
	}
}

// HandleInterrupt handles interrupt events from pattern matcher
func (fe *FlowEngine) HandleInterrupt(interruptType string) {
    log.Printf("Handling interrupt: %s", interruptType)

	// Stop timer if active
	if fe.timer.IsActive() {
		fe.timer.Stop()
	}

	// Stop current audio playback (if possible)
	if err := fe.session.StopAudio(); err != nil {
		log.Printf("Warning: Failed to stop audio: %v", err)
	}
	
	// Small delay to ensure audio stops completely
	time.Sleep(100 * time.Millisecond)

	// Find interrupt node
	interruptNode := fe.findNode(interruptType)
	if interruptNode != nil {
		fe.waitingFor = nil
		fe.currentNode = interruptNode
		fe.executeNode(interruptNode)
	} else {
		log.Printf("Warning: Interrupt node %s not found in flow configuration", interruptType)
	}
}

// handleTransferNode handles transfer nodes
func (fe *FlowEngine) handleTransferNode(node *FlowNode) error {
	// Play transfer audio
	if err := fe.session.PlayAudio(node.AudioFile); err != nil {
		return fmt.Errorf("failed to play audio: %w", err)
	}

    // Stop transcription (AssemblyAI)
    fe.session.StopTranscription()

    // Execute actions
    if err := fe.executeActions(node.Actions); err != nil {
        log.Printf("Warning: failed to execute transfer actions: %v", err)
    }

    // Vicidial: ra_call_control for transfer (resolved by session ID)
    if fe.apiClient != nil {
        status := fe.apiClient.TransferStatus()
        phone := fe.apiClient.TransferPhone()
        if err := fe.apiClient.UpdateRaCallControlBySession(fe.session.GetID(), "EXTENSIONTRANSFER", status, phone); err != nil {
            log.Printf("Warning: transfer ra_call_control failed: %v", err)
        }
    }

    // Mark as transferred so raw hangup does not post DC later
    fe.transferred = true

    // Flow ends here (call continues but flow is done)
    fe.isActive = false
    log.Printf("Transfer completed, flow ended for session %s", fe.session.GetID())
    if fe.logger != nil {
        fe.logger.LogFlowEnd(fe.session.GetID(), time.Now(), "transfer")
        _ = fe.logger.Close()
    }

	return nil
}

// handleHangupNode handles hangup nodes
func (fe *FlowEngine) handleHangupNode(node *FlowNode) error {
    // Play hangup audio (if specified)
    if node.AudioFile != "" {
        if err := fe.session.PlayAudio(node.AudioFile); err != nil {
            return fmt.Errorf("failed to play audio: %w", err)
        }
    }

    // Execute actions
    if err := fe.executeActions(node.Actions); err != nil {
        log.Printf("Warning: failed to execute hangup actions: %v", err)
    }

    // Vicidial: ra_call_control for hangup with flow reason
    if fe.apiClient != nil && !hasEndCallAction(node.Actions) {
        status := fe.lastReason
        if status == "" {
            status = "DC"
        }
        if err := fe.apiClient.UpdateRaCallControlBySession(fe.session.GetID(), "HANGUP", status, ""); err != nil {
            log.Printf("Warning: hangup ra_call_control failed: %v", err)
        }
    }

    // Send hangup command to end the call
    if err := fe.session.EndCall(); err != nil {
        log.Printf("Warning: failed to send hangup command: %v", err)
    }

    // Flow ends here
    fe.isActive = false
    log.Printf("Hangup completed, flow ended for session %s", fe.session.GetID())
    if fe.logger != nil {
        fe.logger.LogHangup(fe.session.GetID())
        fe.logger.LogFlowEnd(fe.session.GetID(), time.Now(), "hangup")
        _ = fe.logger.Close()
    }

    return nil
}

// handleInterruptNode handles interrupt nodes
func (fe *FlowEngine) handleInterruptNode(node *FlowNode) error {
    // Play interrupt audio (if specified)
    if node.AudioFile != "" {
        if err := fe.session.PlayAudio(node.AudioFile); err != nil {
            return fmt.Errorf("failed to play audio: %w", err)
        }
    }

	// Execute actions
	if err := fe.executeActions(node.Actions); err != nil {
		log.Printf("Warning: failed to execute interrupt actions: %v", err)
	}

	// Move to next node (usually end_call)
	nextNodeID := node.Transitions["default"]
	if nextNodeID != "" {
		nextNode := fe.findNode(nextNodeID)
		if nextNode != nil {
			fe.currentNode = nextNode
			return fe.executeNode(nextNode)
		}
	}

    // Flow ends here
    fe.isActive = false
    log.Printf("Interrupt completed, flow ended for session %s", fe.session.GetID())
    if fe.logger != nil {
        fe.logger.LogFlowEnd(fe.session.GetID(), time.Now(), "interrupt")
        _ = fe.logger.Close()
    }

    return nil
}

// executeActions executes all actions for a node
func (fe *FlowEngine) executeActions(actions []Action) error {
    for _, action := range actions {
        switch action.Type {
        case "api_call":
            // Execute API call based on endpoint
            if err := fe.executeAPICall(action); err != nil {
                log.Printf("Warning: API call failed: %v", err)
                if fe.logger != nil {
                    fe.logger.LogAPICall(fe.session.GetID(), action.Endpoint, "error")
                }
            } else {
                log.Printf("API call successful: %s %s", action.Method, action.Endpoint)
                if fe.logger != nil {
                    fe.logger.LogAPICall(fe.session.GetID(), action.Endpoint, "ok")
                }
            }
        case "log":
            log.Printf("Log action: %s", action.Message)
        case "transfer":
            log.Printf("Transfer action: destination=%s, timeout=%d", action.Endpoint, action.Timeout)
            if fe.logger != nil {
                fe.logger.LogTransfer(fe.session.GetID(), action.Endpoint)
            }
        default:
            log.Printf("Unknown action type: %s", action.Type)
        }
    }
    return nil
}

// executeAPICall executes an API call action
func (fe *FlowEngine) executeAPICall(action Action) error {
    if fe.apiClient == nil {
        return fmt.Errorf("api client not configured")
    }
    // Map legacy endpoints from flow.json to Vicidial functions
    switch action.Endpoint {
    case "/add_to_dnc":
        // Do not call Vicidial immediately; mark intent and defer to hangup
        fe.lastReason = "DNC"
        return nil
    case "/mark_not_interested":
        fe.lastReason = "NI"
        return nil
    case "/schedule_callback":
        fe.lastReason = "CALLBK"
        return nil
    case "/transfer_call":
        return fe.apiClient.UpdateRaCallControlBySession(fe.session.GetID(), "EXTENSIONTRANSFER", fe.apiClient.TransferStatus(), fe.apiClient.TransferPhone())
    case "/end_call":
        status := fe.lastReason
        if status == "" {
            status = "DC"
        }
        return fe.apiClient.UpdateRaCallControlBySession(fe.session.GetID(), "HANGUP", status, "")
    default:
        return fmt.Errorf("unknown action endpoint: %s", action.Endpoint)
    }
}

// IsActive returns whether the flow is currently active
func (fe *FlowEngine) IsActive() bool {
    return fe.isActive
}

// GetCurrentNode returns the current node
func (fe *FlowEngine) GetCurrentNode() *FlowNode {
    return fe.currentNode
}

// Close releases resources like the session logger
func (fe *FlowEngine) Close() {
    if fe.logger != nil {
        _ = fe.logger.Close()
    }
}

// GetLastReason returns the last determined final reason (e.g., A, NI, DNC, CALLBK)
func (fe *FlowEngine) GetLastReason() string { return fe.lastReason }

// WasTransferred indicates if a transfer has occurred in this flow
func (fe *FlowEngine) WasTransferred() bool { return fe.transferred }

// hasEndCallAction checks if actions include an explicit /end_call API call
func hasEndCallAction(actions []Action) bool {
    for _, a := range actions {
        if a.Type == "api_call" && a.Endpoint == "/end_call" {
            return true
        }
    }
    return false
}
