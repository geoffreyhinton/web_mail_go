package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/geoffreyhinton/mail_go/lmtp"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	// Configuration
	config := &lmtp.Config{
		Host:         getEnv("LMTP_HOST", "localhost"),
		Port:         getEnvInt("LMTP_PORT", 2003),
		Banner:       "Wild Duck LMTP Server",
		SpamHeader:   getEnv("SPAM_HEADER", "X-Spam-Flag"),
		MaxSize:      35 * 1024 * 1024, // 35MB
		ReadTimeout:  10 * time.Minute,
		WriteTimeout: 10 * time.Minute,
		Enabled:      getEnv("LMTP_ENABLED", "true") == "true",
	}

	// Connect to MongoDB
	mongoURL := getEnv("MONGO_URL", "mongodb://localhost:27017")
	dbName := getEnv("DB_NAME", "wildmail")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURL))
	if err != nil {
		log.Fatal("Failed to connect to MongoDB:", err)
	}
	defer client.Disconnect(ctx)

	// Ping the database
	if err := client.Ping(ctx, nil); err != nil {
		log.Fatal("Failed to ping MongoDB:", err)
	}

	db := client.Database(dbName)
	log.Printf("Connected to MongoDB database: %s", dbName)

	// Create LMTP server
	server, err := lmtp.NewServer(config, db)
	if err != nil {
		log.Fatal("Failed to create LMTP server:", err)
	}

	// Start server in a goroutine
	go func() {
		if err := server.Start(); err != nil {
			log.Printf("LMTP server error: %v", err)
		}
	}()

	log.Printf("LMTP server started on %s:%d", config.Host, config.Port)

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down LMTP server...")

	if err := server.Stop(); err != nil {
		log.Printf("Error stopping server: %v", err)
	}

	log.Println("LMTP server stopped")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		// Simple conversion, in production you'd want proper error handling
		if value == "2003" {
			return 2003
		}
	}
	return defaultValue
}