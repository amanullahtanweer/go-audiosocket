package server

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
	"github.com/amanullahtanweer/audiosocket-transcriber/internal/audio"
	"github.com/amanullahtanweer/audiosocket-transcriber/internal/transcriber"
	"github.com/google/uuid"
)

type Config struct {
    Host            string
    Port            int
    Provider        string // "vosk" or "assemblyai"
    VoskServerURL   string
    AssemblyAPIKey  string
    SampleRate      int
    OutputDir       string
    SaveTranscripts bool
    SaveAudio       bool
    AudioDir        string // Directory containing audio files
}

type Server struct {
    config     Config
    listener   net.Listener
    wg         sync.WaitGroup
    shutdown   chan struct{}
    audioPlayer *audio.Player
}

type Session struct {
    id          uuid.UUID
    conn        net.Conn
    transcriber transcriber.Transcriber
    server      *Server
    audioBuffer []byte
    startTime   time.Time
    stopAmbient chan struct{} // Channel to stop ambient audio
    interruptDetector *audio.InterruptDetector // Handles keyword detection and audio responses
}

func New(config Config) (*Server, error) {
    // Create output directory if needed
    if (config.SaveTranscripts || config.SaveAudio) && config.OutputDir != "" {
        if err := os.MkdirAll(config.OutputDir, 0755); err != nil {
            return nil, fmt.Errorf("failed to create output directory: %w", err)
        }
    }

    // Initialize audio player if audio directory is specified
    var audioPlayer *audio.Player
    if config.AudioDir != "" {
        var err error
        audioPlayer, err = audio.NewPlayer(config.AudioDir)
        if err != nil {
            return nil, fmt.Errorf("failed to initialize audio player: %w", err)
        }
    }

    return &Server{
        config:     config,
        shutdown:   make(chan struct{}),
        audioPlayer: audioPlayer,
    }, nil
}

func (s *Server) Start() error {
    addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
    listener, err := net.Listen("tcp", addr)
    if err != nil {
        return fmt.Errorf("failed to listen on %s: %w", addr, err)
    }
    s.listener = listener

    log.Printf("AudioSocket server listening on %s", addr)
    log.Printf("Transcription provider: %s", s.config.Provider)

    for {
        select {
        case <-s.shutdown:
            return nil
        default:
            conn, err := listener.Accept()
            if err != nil {
                select {
                case <-s.shutdown:
                    return nil
                default:
                    log.Printf("Accept error: %v", err)
                    continue
                }
            }

            s.wg.Add(1)
            go s.handleConnection(conn)
        }
    }
}

func (s *Server) Stop() {
    close(s.shutdown)
    if s.listener != nil {
        s.listener.Close()
    }
    s.wg.Wait()
}

func (s *Server) handleConnection(conn net.Conn) {
    defer s.wg.Done()
    defer conn.Close()

    log.Printf("New connection from %s", conn.RemoteAddr())

    // Read the initial ID message
    id, err := audiosocket.GetID(conn)
    if err != nil {
        log.Printf("Failed to get ID: %v", err)
        return
    }

    log.Printf("Session %s started with %s", id, s.config.Provider)

    // Create appropriate transcriber based on provider
    var sessionTranscriber transcriber.Transcriber
    
    switch s.config.Provider {
    case "vosk":
        sessionTranscriber, err = transcriber.NewVoskTranscriber(
            s.config.VoskServerURL,
            s.config.SampleRate,
        )
    case "assemblyai":
        sessionTranscriber, err = transcriber.NewAssemblyAITranscriber(
            s.config.AssemblyAPIKey,
            s.config.SampleRate,
        )
    default:
        err = fmt.Errorf("unknown provider: %s", s.config.Provider)
    }

    if err != nil {
        log.Printf("Failed to create transcriber for session %s: %v", id, err)
        return
    }
    defer sessionTranscriber.Close()

    session := &Session{
        id:          id,
        conn:        conn,
        transcriber: sessionTranscriber,
        server:      s,
        audioBuffer: make([]byte, 0, 16000), // Buffer for ~1 second of audio
        startTime:   time.Now(),
        stopAmbient: make(chan struct{}),
    }

    // Initialize interrupt detector if audio player is available
    if s.audioPlayer != nil {
        session.interruptDetector = audio.NewInterruptDetector(s.audioPlayer)
    }

    // Initialize interrupt detector if audio player is available
    if s.audioPlayer != nil {
        session.interruptDetector = audio.NewInterruptDetector(s.audioPlayer)
    }

    // Play greeting audio if audio player is available
    if s.audioPlayer != nil {
        if err := s.audioPlayer.PlayGreeting(conn); err != nil {
            log.Printf("Session %s: Failed to play greeting: %v", id, err)
        } else {
            log.Printf("Session %s: Greeting audio played", id)
        }
        
        // Start ambient audio
        s.audioPlayer.StartAmbientAudio(conn, session.stopAmbient)
    }

    // Start transcription handler
    go session.handleTranscription()

    // Process messages
    for {
        msg, err := audiosocket.NextMessage(conn)
        if err != nil {
            if err != io.EOF {
                log.Printf("Session %s: Failed to read message: %v", id, err)
            }
            break
        }

        if err := session.handleMessage(msg); err != nil {
            log.Printf("Session %s: Error handling message: %v", id, err)
            break
        }

        // Check if it's a hangup message
        if msg.Kind() == audiosocket.KindHangup {
            log.Printf("Session %s: Received hangup", id)
            break
        }
    }

    // Finalize transcription
    session.finalize()
    
    duration := time.Since(session.startTime)
    log.Printf("Session %s ended (Duration: %v, Provider: %s)", id, duration, s.config.Provider)
}

func (session *Session) handleMessage(msg audiosocket.Message) error {
    switch msg.Kind() {
    case audiosocket.KindSlin:
        // Process audio data
        audioData := msg.Payload()
        if len(audioData) > 0 {
            // Send to transcriber
            if err := session.transcriber.ProcessAudio(audioData); err != nil {
                return fmt.Errorf("failed to process audio: %w", err)
            }
            
            // Buffer audio for saving if configured
            if session.server.config.SaveAudio {
                session.audioBuffer = append(session.audioBuffer, audioData...)
            }
        }

    case audiosocket.KindDTMF:
        // Handle DTMF
        if len(msg.Payload()) > 0 {
            digit := msg.Payload()[0]
            log.Printf("Session %s: DTMF digit: %c", session.id, digit)
            session.transcriber.AddMarker(fmt.Sprintf("[DTMF: %c]", digit))
        }

    case audiosocket.KindSilence:
        log.Printf("Session %s: Silence detected", session.id)
        session.transcriber.AddMarker("[SILENCE]")

    case audiosocket.KindError:
        errCode := msg.ErrorCode()
        return fmt.Errorf("received error code: %d", errCode)
    }

    return nil
}

func (session *Session) handleTranscription() {
    for result := range session.transcriber.Results() {
        if result.Text != "" {
            timestamp := time.Now().Format("15:04:05")
            provider := session.server.config.Provider
            
            if result.IsFinal {
                log.Printf("[%s] Session %s [%s] Final: %s", provider, session.id, timestamp, result.Text)
                
                // Check for interrupts only on final transcriptions
                if session.interruptDetector != nil {
                    if interruptRule := session.interruptDetector.DetectInterrupt(result.Text); interruptRule != nil {
                        log.Printf("Session %s: Playing interrupt audio: %s", session.id, interruptRule.Type)
                        
                        // Play the interrupt audio
                        go func() {
                            if err := session.interruptDetector.PlayInterrupt(interruptRule, session.conn); err != nil {
                                log.Printf("Session %s: Failed to play interrupt audio: %v", session.id, err)
                            }
                        }()
                    }
                }
            } else {
                log.Printf("[%s] Session %s [%s] Partial: %s", provider, session.id, timestamp, result.Text)
            }
        }
    }
}

func (session *Session) finalize() {
    // Stop ambient audio
    close(session.stopAmbient)
    
    // Stop any playing interrupts
    if session.interruptDetector != nil {
        session.interruptDetector.Stop()
    }
    
    // Get final transcription
    fullTranscript := session.transcriber.GetFullTranscript()
    
    if session.server.config.SaveTranscripts && fullTranscript != "" {
        // Add metadata to transcript
        metadata := fmt.Sprintf("Session ID: %s\nProvider: %s\nStart Time: %s\nDuration: %v\nSample Rate: %dHz\n\n---TRANSCRIPT---\n\n",
            session.id,
            session.server.config.Provider,
            session.startTime.Format("2006-01-02 15:04:05"),
            time.Since(session.startTime),
            session.server.config.SampleRate,
        )
        
        fullContent := metadata + fullTranscript
        
        // Save transcript to file
        filename := filepath.Join(
            session.server.config.OutputDir,
            fmt.Sprintf("%s_%s_%s.txt", 
                session.startTime.Format("20060102_150405"),
                session.server.config.Provider,
                session.id.String()[:8],
            ),
        )
        
        if err := os.WriteFile(filename, []byte(fullContent), 0644); err != nil {
            log.Printf("Failed to save transcript: %v", err)
        } else {
            log.Printf("Session %s: Transcript saved to %s", session.id, filename)
        }
    }
    
    // Save raw audio if configured
    if session.server.config.SaveAudio && len(session.audioBuffer) > 0 {
        audioFilename := filepath.Join(
            session.server.config.OutputDir,
            fmt.Sprintf("%s_%s_%s.raw", 
                session.startTime.Format("20060102_150405"),
                session.server.config.Provider,
                session.id.String()[:8],
            ),
        )
        
        if err := os.WriteFile(audioFilename, session.audioBuffer, 0644); err != nil {
            log.Printf("Failed to save audio: %v", err)
        } else {
            log.Printf("Session %s: Audio saved to %s (%.2f seconds)", 
                session.id, 
                audioFilename, 
                float64(len(session.audioBuffer))/(float64(session.server.config.SampleRate)*2))
        }
    }
}