package transcriber

// Transcriber is the common interface for all transcription providers
type Transcriber interface {
	ProcessAudio(audioData []byte) error
	Results() <-chan TranscriptionResult
	GetFullTranscript() string
	AddMarker(marker string)
	Close() error
}

// TranscriptionResult represents a transcription result
type TranscriptionResult struct {
	Text       string
	IsFinal    bool
	Confidence float64 // Optional confidence score
	Timestamp  float64 // Optional timestamp
}
