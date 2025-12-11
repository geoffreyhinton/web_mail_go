package main

import (
	"log"
	"time"

	"github.com/geoffreyhinton/mail_go/lmtp"
)

func main() {
	log.Println("Testing LMTP server...")

	// Connect to LMTP server
	client, err := lmtp.NewLMTPClient("localhost", 2003)
	if err != nil {
		log.Fatalf("Failed to connect to LMTP server: %v", err)
	}
	defer client.Close()

	// Create test message
	message := lmtp.CreateTestMessage(
		"sender@example.com",
		"testuser@localhost",
		"Test Message",
		"This is a test message sent via LMTP.\n\nBest regards,\nTest System",
	)

	// Send message
	err = client.SendMail(
		"sender@example.com",
		[]string{"testuser@localhost"},
		message,
	)
	
	if err != nil {
		log.Printf("Failed to send message: %v", err)
	} else {
		log.Println("Message sent successfully!")
	}

	// Wait a moment for processing
	time.Sleep(1 * time.Second)
	log.Println("LMTP test completed")
}