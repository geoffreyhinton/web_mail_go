package main

import (
	"context"
	"crypto/tls"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	imapcore "github.com/geoffreyhinton/mail_go/imap_core"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Config represents the IMAP server configuration
type Config struct {
	IMAP struct {
		Port           int    `json:"port" default:"143"`
		Host           string `json:"host" default:"0.0.0.0"`
		Secure         bool   `json:"secure" default:"false"`
		IgnoreSTARTTLS bool   `json:"ignoreSTARTTLS" default:"false"`
		MaxMB          int    `json:"maxMB" default:"25"`
		MaxStorage     int64  `json:"maxStorage" default:"1073741824"` // 1GB
		KeyFile        string `json:"key"`
		CertFile       string `json:"cert"`
	} `json:"imap"`
	MongoDB struct {
		URI      string `json:"uri" default:"mongodb://localhost:27017"`
		Database string `json:"database" default:"maildb"`
	} `json:"mongodb"`
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		IMAP: struct {
			Port           int    `json:"port" default:"143"`
			Host           string `json:"host" default:"0.0.0.0"`
			Secure         bool   `json:"secure" default:"false"`
			IgnoreSTARTTLS bool   `json:"ignoreSTARTTLS" default:"false"`
			MaxMB          int    `json:"maxMB" default:"25"`
			MaxStorage     int64  `json:"maxStorage" default:"1073741824"`
			KeyFile        string `json:"key"`
			CertFile       string `json:"cert"`
		}{
			Port:       143,
			Host:       "0.0.0.0",
			Secure:     false,
			MaxMB:      25,
			MaxStorage: 1073741824,
		},
		MongoDB: struct {
			URI      string `json:"uri" default:"mongodb://localhost:27017"`
			Database string `json:"database" default:"maildb"`
		}{
			URI:      "mongodb://localhost:27017",
			Database: "maildb",
		},
	}
}

// Logger interface for structured logging
type Logger interface {
	Info(msg string, fields ...interface{})
	Debug(msg string, fields ...interface{})
	Error(msg string, fields ...interface{})
}

// DefaultLogger provides basic logging implementation
type DefaultLogger struct{}

func (l *DefaultLogger) Info(msg string, fields ...interface{}) {
	log.Printf("[INFO] IMAP: "+msg, fields...)
}

func (l *DefaultLogger) Debug(msg string, fields ...interface{}) {
	log.Printf("[DEBUG] IMAP: "+msg, fields...)
}

func (l *DefaultLogger) Error(msg string, fields ...interface{}) {
	log.Printf("[ERROR] IMAP: "+msg, fields...)
}

func main() {
	// Load configuration
	config := DefaultConfig()

	// Initialize MongoDB connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientOptions := options.Client().ApplyURI(config.MongoDB.URI)
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer client.Disconnect(context.Background())

	// Verify MongoDB connection
	if err := client.Ping(ctx, nil); err != nil {
		log.Fatalf("Failed to ping MongoDB: %v", err)
	}

	database := client.Database(config.MongoDB.Database)
	logger := &DefaultLogger{}

	// Note: Indexer functionality would be integrated when processing APPEND commands

	// Create server options
	serverOptions := &imapcore.ServerOptions{
		Host:           config.IMAP.Host,
		Port:           config.IMAP.Port,
		Secure:         config.IMAP.Secure,
		IgnoreSTARTTLS: config.IMAP.IgnoreSTARTTLS,

		MaxStorage: config.IMAP.MaxStorage,
		Logger:     logger,
		Database:   database,
	}

	// Load TLS certificates if configured
	if config.IMAP.KeyFile != "" && config.IMAP.CertFile != "" {
		cert, err := tls.LoadX509KeyPair(config.IMAP.CertFile, config.IMAP.KeyFile)
		if err != nil {
			log.Fatalf("Failed to load TLS certificate: %v", err)
		}
		serverOptions.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
		}
	}

	// Create and start IMAP server
	server, err := imapcore.NewIMAPServer(serverOptions)
	if err != nil {
		log.Fatalf("Failed to create IMAP server: %v", err)
	}

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start server in goroutine
	go func() {
		logger.Info("Starting IMAP server on %s:%d", config.IMAP.Host, config.IMAP.Port)
		if err := server.Start(); err != nil {
			log.Fatalf("IMAP server failed: %v", err)
		}
	}()

	// Wait for shutdown signal
	<-sigChan
	logger.Info("Shutting down IMAP server...")

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("Error during server shutdown: %v", err)
	}

	logger.Info("IMAP server stopped")
}
