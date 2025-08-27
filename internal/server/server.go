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
	"github.com/amanullahtanweer/audiosocket-transcriber/internal/transcriber"
	"github.com/google/uuid"
)

type Config struct {
	Host            string
	Port            int
	VoskServerURL   string
	SampleRate      int
	OutputDir       string
	SaveTranscripts bool
}

type Server struct {
	config   Config
	listener net.Listener
	wg       sync.WaitGroup
	shutdown chan struct{}
}

type Session struct {
	id          uuid.UUID
	conn        net.Conn
	transcriber *transcriber.VoskTranscriber
	server      *Server
	audioBuffer []byte
}

func New(config Config) (*Server, error) {
	// Create output directory if needed
	if config.SaveTranscripts && config.OutputDir != "" {
		if err := os.MkdirAll(config.OutputDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create output directory: %w", err)
		}
	}

	return &Server{
		config:   config,
		shutdown: make(chan struct{}),
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
	log.Printf("Vosk server: %s", s.config.VoskServerURL)

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

	log.Printf("Session %s started", id)

	// Create transcriber for this session
	voskTranscriber, err := transcriber.NewVoskTranscriber(
		s.config.VoskServerURL,
		s.config.SampleRate,
	)
	if err != nil {
		log.Printf("Failed to create transcriber for session %s: %v", id, err)
		return
	}
	defer voskTranscriber.Close()

	session := &Session{
		id:          id,
		conn:        conn,
		transcriber: voskTranscriber,
		server:      s,
		audioBuffer: make([]byte, 0, 8000), // Buffer for 1 second of audio
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
	log.Printf("Session %s ended", id)
}

func (session *Session) handleMessage(msg audiosocket.Message) error {
	switch msg.Kind() {
	case audiosocket.KindSlin:
		// Process audio data
		audioData := msg.Payload()
		if len(audioData) > 0 {
			// Send to Vosk for transcription
			if err := session.transcriber.ProcessAudio(audioData); err != nil {
				return fmt.Errorf("failed to process audio: %w", err)
			}

			// Optionally buffer audio for saving
			if session.server.config.SaveTranscripts {
				session.audioBuffer = append(session.audioBuffer, audioData...)
			}
		}

	case audiosocket.KindDTMF:
		// Handle DTMF if needed
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
			if result.IsFinal {
				log.Printf("Session %s [%s] Final: %s", session.id, timestamp, result.Text)
			} else {
				log.Printf("Session %s [%s] Partial: %s", session.id, timestamp, result.Text)
			}
		}
	}
}

func (session *Session) finalize() {
	// Get final transcription
	fullTranscript := session.transcriber.GetFullTranscript()

	if session.server.config.SaveTranscripts && fullTranscript != "" {
		// Save transcript to file
		filename := filepath.Join(
			session.server.config.OutputDir,
			fmt.Sprintf("%s_%s.txt",
				time.Now().Format("20060102_150405"),
				session.id.String()[:8],
			),
		)

		if err := os.WriteFile(filename, []byte(fullTranscript), 0644); err != nil {
			log.Printf("Failed to save transcript: %v", err)
		} else {
			log.Printf("Session %s: Transcript saved to %s", session.id, filename)
		}

		// Optionally save raw audio
		if len(session.audioBuffer) > 0 {
			audioFilename := filepath.Join(
				session.server.config.OutputDir,
				fmt.Sprintf("%s_%s.raw",
					time.Now().Format("20060102_150405"),
					session.id.String()[:8],
				),
			)
			if err := os.WriteFile(audioFilename, session.audioBuffer, 0644); err != nil {
				log.Printf("Failed to save audio: %v", err)
			}
		}
	}
}
