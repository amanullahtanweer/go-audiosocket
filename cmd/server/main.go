package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/amanullahtanweer/audiosocket-transcriber/internal/server"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Server struct {
		Host string `yaml:"host"`
		Port int    `yaml:"port"`
	} `yaml:"server"`
	Vosk struct {
		ServerURL  string `yaml:"server_url"`
		SampleRate int    `yaml:"sample_rate"`
	} `yaml:"vosk"`
	Transcription struct {
		OutputDir       string `yaml:"output_dir"`
		SaveTranscripts bool   `yaml:"save_transcripts"`
	} `yaml:"transcription"`
}

func main() {
	var configFile string
	flag.StringVar(&configFile, "config", "config.yaml", "Configuration file path")
	flag.Parse()

	// Load configuration
	config := &Config{}
	if err := loadConfig(configFile, config); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Create and start server
	srv, err := server.New(server.Config{
		Host:            config.Server.Host,
		Port:            config.Server.Port,
		VoskServerURL:   config.Vosk.ServerURL,
		SampleRate:      config.Vosk.SampleRate,
		OutputDir:       config.Transcription.OutputDir,
		SaveTranscripts: config.Transcription.SaveTranscripts,
	})
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// Start server in background
	go func() {
		if err := srv.Start(); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down server...")
	srv.Stop()
}

func loadConfig(filename string, config *Config) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	decoder := yaml.NewDecoder(file)
	return decoder.Decode(config)
}
