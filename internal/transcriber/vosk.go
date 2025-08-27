package transcriber

import (
    "encoding/json"
    "fmt"
    "log"
    "strings"
    "sync"

    "github.com/gorilla/websocket"
)

type VoskTranscriber struct {
    conn         *websocket.Conn
    results      chan TranscriptionResult
    fullText     strings.Builder
    mu           sync.Mutex
    sampleRate   int
}

type TranscriptionResult struct {
    Text    string
    IsFinal bool
}

type VoskResult struct {
    Text   string `json:"text"`
    Result []struct {
        Word  string  `json:"word"`
        Start float64 `json:"start"`
        End   float64 `json:"end"`
        Conf  float64 `json:"conf"`
    } `json:"result"`
    Partial string `json:"partial"`
}

func NewVoskTranscriber(serverURL string, sampleRate int) (*VoskTranscriber, error) {
    // Connect to Vosk server WebSocket
    url := fmt.Sprintf("%s/ws?sample_rate=%d", serverURL, sampleRate)
    conn, _, err := websocket.DefaultDialer.Dial(url, nil)
    if err != nil {
        return nil, fmt.Errorf("failed to connect to Vosk server: %w", err)
    }

    vt := &VoskTranscriber{
        conn:       conn,
        results:    make(chan TranscriptionResult, 100),
        sampleRate: sampleRate,
    }

    // Start result handler
    go vt.handleResults()

    return vt, nil
}

func (vt *VoskTranscriber) ProcessAudio(audioData []byte) error {
    vt.mu.Lock()
    defer vt.mu.Unlock()

    // Send audio data to Vosk
    if err := vt.conn.WriteMessage(websocket.BinaryMessage, audioData); err != nil {
        return fmt.Errorf("failed to send audio to Vosk: %w", err)
    }

    return nil
}

func (vt *VoskTranscriber) handleResults() {
    for {
        _, message, err := vt.conn.ReadMessage()
        if err != nil {
            if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
                log.Printf("Vosk WebSocket error: %v", err)
            }
            close(vt.results)
            return
        }

        var result VoskResult
        if err := json.Unmarshal(message, &result); err != nil {
            log.Printf("Failed to parse Vosk result: %v", err)
            continue
        }

        // Handle partial results
        if result.Partial != "" {
            vt.results <- TranscriptionResult{
                Text:    result.Partial,
                IsFinal: false,
            }
        }

        // Handle final results
        if result.Text != "" {
            vt.mu.Lock()
            if vt.fullText.Len() > 0 {
                vt.fullText.WriteString(" ")
            }
            vt.fullText.WriteString(result.Text)
            vt.mu.Unlock()

            vt.results <- TranscriptionResult{
                Text:    result.Text,
                IsFinal: true,
            }
        }
    }
}

func (vt *VoskTranscriber) Results() <-chan TranscriptionResult {
    return vt.results
}

func (vt *VoskTranscriber) GetFullTranscript() string {
    vt.mu.Lock()
    defer vt.mu.Unlock()
    return vt.fullText.String()
}

func (vt *VoskTranscriber) AddMarker(marker string) {
    vt.mu.Lock()
    defer vt.mu.Unlock()
    
    if vt.fullText.Len() > 0 {
        vt.fullText.WriteString(" ")
    }
    vt.fullText.WriteString(marker)
}

func (vt *VoskTranscriber) Close() error {
    // Send EOF to Vosk to get final results
    if err := vt.conn.WriteMessage(websocket.TextMessage, []byte(`{"eof": 1}`)); err != nil {
        log.Printf("Failed to send EOF to Vosk: %v", err)
    }
    
    return vt.conn.Close()
}