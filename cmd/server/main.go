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
    
    Transcription struct {
        Provider        string `yaml:"provider"` // "vosk" or "assemblyai"
        OutputDir       string `yaml:"output_dir"`
        SaveTranscripts bool   `yaml:"save_transcripts"`
        SaveAudio       bool   `yaml:"save_audio"`
        SaveSessionLogs bool   `yaml:"save_session_logs"`
    } `yaml:"transcription"`
    
    Vosk struct {
        ServerURL  string `yaml:"server_url"`
        SampleRate int    `yaml:"sample_rate"`
    } `yaml:"vosk"`
    
    AssemblyAI struct {
        APIKey     string `yaml:"api_key"`
        SampleRate int    `yaml:"sample_rate"`
    } `yaml:"assemblyai"`

    Vicidial struct {
        ServerURL      string `yaml:"server_url"`
        AdminDir       string `yaml:"admin_dir"`
        APIUser        string `yaml:"api_user"`
        APIPass        string `yaml:"api_pass"`
        SourceRA       string `yaml:"source_ra"`
        SourceAdmin    string `yaml:"source_admin"`
        TransferStatus string `yaml:"transfer_status"`
        TransferPhone  string `yaml:"transfer_phone"`
    } `yaml:"vicidial"`

    Redis struct {
        Addr   string `yaml:"addr"`   // default localhost:6379
        DB     int    `yaml:"db"`     // default 0
        Prefix string `yaml:"prefix"` // optional; leave empty to use bare UUID keys
    } `yaml:"redis"`
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

    // Validate provider
    if config.Transcription.Provider != "vosk" && config.Transcription.Provider != "assemblyai" {
        log.Fatalf("Invalid transcription provider: %s. Must be 'vosk' or 'assemblyai'", config.Transcription.Provider)
    }

    // Create server config
    serverConfig := server.Config{
        Host:            config.Server.Host,
        Port:            config.Server.Port,
        Provider:        config.Transcription.Provider,
        OutputDir:       config.Transcription.OutputDir,
        SaveTranscripts: config.Transcription.SaveTranscripts,
        SaveAudio:       config.Transcription.SaveAudio,
        SaveSessionLogs: config.Transcription.SaveSessionLogs,
        AudioDir:        "./audios", // Directory containing audio files
        VicidialServerURL:   config.Vicidial.ServerURL,
        VicidialAdminDir:    config.Vicidial.AdminDir,
        VicidialAPIUser:     config.Vicidial.APIUser,
        VicidialAPIPass:     config.Vicidial.APIPass,
        VicidialSourceRA:    config.Vicidial.SourceRA,
        VicidialSourceAdmin: config.Vicidial.SourceAdmin,
        TransferStatus:      config.Vicidial.TransferStatus,
        TransferPhone:       config.Vicidial.TransferPhone,
        RedisAddr:           config.Redis.Addr,
        RedisDB:             config.Redis.DB,
        RedisPrefix:         config.Redis.Prefix,
    }

    // Add provider-specific config
    if config.Transcription.Provider == "vosk" {
        serverConfig.VoskServerURL = config.Vosk.ServerURL
        serverConfig.SampleRate = config.Vosk.SampleRate
    } else {
        serverConfig.AssemblyAPIKey = config.AssemblyAI.APIKey
        serverConfig.SampleRate = config.AssemblyAI.SampleRate
    }

    // Create and start server
    srv, err := server.New(serverConfig)
    if err != nil {
        log.Fatalf("Failed to create server: %v", err)
    }

    // Start server in background
    go func() {
        if err := srv.Start(); err != nil {
            log.Fatalf("Server error: %v", err)
        }
    }()

    log.Printf("AudioSocket server started with %s transcription provider", config.Transcription.Provider)

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
