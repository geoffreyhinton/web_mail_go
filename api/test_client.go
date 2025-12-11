package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

const baseURL = "http://localhost:8080/api"

func main() {
	fmt.Println("Testing Mail API...")
	
	// Test health check
	fmt.Println("\n1. Testing health check...")
	resp, err := http.Get("http://localhost:8080/health")
	if err != nil {
		log.Printf("Health check failed: %v", err)
	} else {
		fmt.Printf("Health check response: %d\n", resp.StatusCode)
		resp.Body.Close()
	}
	
	// Test create user
	fmt.Println("\n2. Testing user creation...")
	user := map[string]interface{}{
		"username": "testuser",
		"password": "password123",
		"address":  "testuser@example.com",
		"quota":    1073741824, // 1GB
	}
	
	userJSON, _ := json.Marshal(user)
	resp, err = http.Post(baseURL+"/users", "application/json", bytes.NewBuffer(userJSON))
	if err != nil {
		log.Printf("User creation failed: %v", err)
		return
	}
	defer resp.Body.Close()
	
	var createResult map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&createResult)
	fmt.Printf("User creation response: %+v\n", createResult)
	
	if !createResult["success"].(bool) {
		log.Printf("Failed to create user")
		return
	}
	
	userID := createResult["id"].(string)
	fmt.Printf("Created user with ID: %s\n", userID)
	
	// Test get user
	fmt.Println("\n3. Testing get user...")
	resp, err = http.Get(baseURL + "/users/" + userID)
	if err != nil {
		log.Printf("Get user failed: %v", err)
		return
	}
	defer resp.Body.Close()
	
	var getUserResult map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&getUserResult)
	fmt.Printf("Get user response: %+v\n", getUserResult)
	
	// Test list users
	fmt.Println("\n4. Testing list users...")
	resp, err = http.Get(baseURL + "/users?limit=10")
	if err != nil {
		log.Printf("List users failed: %v", err)
		return
	}
	defer resp.Body.Close()
	
	var listResult map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&listResult)
	fmt.Printf("List users response: success=%v, total=%v\n", 
		listResult["success"], listResult["total"])
	
	// Test get user mailboxes
	fmt.Println("\n5. Testing get user mailboxes...")
	resp, err = http.Get(baseURL + "/users/" + userID + "/mailboxes?counters=true")
	if err != nil {
		log.Printf("Get mailboxes failed: %v", err)
		return
	}
	defer resp.Body.Close()
	
	var mailboxResult map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&mailboxResult)
	fmt.Printf("Get mailboxes response: success=%v\n", mailboxResult["success"])
	
	if mailboxes, ok := mailboxResult["mailboxes"].([]interface{}); ok {
		fmt.Printf("Found %d mailboxes\n", len(mailboxes))
		for _, mb := range mailboxes {
			if mbMap, ok := mb.(map[string]interface{}); ok {
				fmt.Printf("  - %s (%s)\n", mbMap["name"], mbMap["id"])
			}
		}
	}
	
	fmt.Println("\nAPI testing completed!")
}

// Helper function to create test messages (would need to be implemented)
func createTestMessage(userID, mailboxID string) {
	// This would create a test message in the database
	// Implementation depends on your message creation API
}