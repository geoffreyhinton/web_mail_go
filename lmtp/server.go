package lmtp

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/emersion/go-smtp"
	"github.com/geoffreyhinton/mail_go/api/models"
	"github.com/geoffreyhinton/mail_go/imap_core/indexer"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// Config holds LMTP server configuration
type Config struct {
	Host         string
	Port         int
	Banner       string
	SpamHeader   string
	MaxSize      int64
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	Enabled      bool
}

// Server represents the LMTP server
type Server struct {
	config    *Config
	db        *mongo.Database
	indexer   *indexer.Indexer
	smtp      *smtp.Server
}

// Session represents an LMTP session
type Session struct {
	server *Server
	users  []UserRecipient
}

// UserRecipient holds recipient and user information
type UserRecipient struct {
	Recipient string
	User      *models.User
	Address   *models.Address
}

// Filter represents message filtering rules
type Filter struct {
	ID     string                 `bson:"id" json:"id"`
	Query  FilterQuery            `bson:"query" json:"query"`
	Action map[string]interface{} `bson:"action" json:"action"`
}

// FilterQuery defines filter matching criteria
type FilterQuery struct {
	Headers map[string]string `bson:"headers,omitempty" json:"headers,omitempty"`
	HasAttachments *int       `bson:"ha,omitempty" json:"ha,omitempty"`
	Size           *int64     `bson:"size,omitempty" json:"size,omitempty"`
	Text           string     `bson:"text,omitempty" json:"text,omitempty"`
}

// NewServer creates a new LMTP server
func NewServer(config *Config, db *mongo.Database) (*Server, error) {
	if config == nil {
		config = &Config{
			Host:         "localhost",
			Port:         2003,
			Banner:       "Wild Duck Mail Server",
			MaxSize:      35 * 1024 * 1024, // 35MB
			ReadTimeout:  10 * time.Minute,
			WriteTimeout: 10 * time.Minute,
			Enabled:      true,
		}
	}

	server := &Server{
		config:  config,
		db:      db,
		indexer: indexer.NewIndexer(),
	}

	// Configure SMTP server for LMTP
	be := &Backend{server: server}
	s := smtp.NewServer(be)

	s.Addr = fmt.Sprintf("%s:%d", config.Host, config.Port)
	s.Domain = config.Banner
	s.MaxMessageBytes = config.MaxSize
	s.MaxRecipients = 100
	s.AllowInsecureAuth = true
	s.ReadTimeout = config.ReadTimeout
	s.WriteTimeout = config.WriteTimeout
	s.LMTP = true // Enable LMTP mode

	server.smtp = s

	return server, nil
}

// Start starts the LMTP server
func (s *Server) Start() error {
	if !s.config.Enabled {
		log.Println("LMTP server is disabled")
		return nil
	}

	log.Printf("Starting LMTP server on %s:%d", s.config.Host, s.config.Port)
	return s.smtp.ListenAndServe()
}

// Stop stops the LMTP server
func (s *Server) Stop() error {
	log.Println("Stopping LMTP server")
	return s.smtp.Close()
}

// Backend implements smtp.Backend
type Backend struct {
	server *Server
}

// NewSession creates a new SMTP session
func (be *Backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	return &Session{
		server: be.server,
		users:  make([]UserRecipient, 0),
	}, nil
}

// AuthPlain is not implemented for LMTP
func (s *Session) AuthPlain(username, password string) error {
	return smtp.ErrAuthUnsupported
}

// Mail handles the MAIL FROM command
func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
	log.Printf("LMTP: MAIL FROM: %s", from)
	// Reset session users for new message
	s.users = make([]UserRecipient, 0)
	return nil
}

// Rcpt handles the RCPT TO command
func (s *Session) Rcpt(to string) error {
	log.Printf("LMTP: RCPT TO: %s", to)
	
	// Normalize recipient address
	originalRecipient := normalizeAddress(to)
	recipient := removeAddressTag(originalRecipient)

	// Find address in database
	var address models.Address
	err := s.server.db.Collection("addresses").FindOne(
		context.Background(),
		bson.M{"address": recipient},
	).Decode(&address)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return &smtp.SMTPError{
				Code:    550,
				Message: "Unknown recipient",
			}
		}
		log.Printf("LMTP: Database error finding address: %v", err)
		return &smtp.SMTPError{
			Code:    450,
			Message: "Database error",
		}
	}

	// Find user associated with the address
	var user models.User
	err = s.server.db.Collection("users").FindOne(
		context.Background(),
		bson.M{"_id": address.User},
	).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return &smtp.SMTPError{
				Code:    550,
				Message: "Unknown recipient",
			}
		}
		log.Printf("LMTP: Database error finding user: %v", err)
		return &smtp.SMTPError{
			Code:    450,
			Message: "Database error",
		}
	}

	// Check if user is disabled
	if user.Disabled {
		return &smtp.SMTPError{
			Code:    550,
			Message: "User disabled",
		}
	}

	// Add to recipients list
	s.users = append(s.users, UserRecipient{
		Recipient: originalRecipient,
		User:      &user,
		Address:   &address,
	})

	return nil
}

// Data handles message data
func (s *Session) Data(r io.Reader) error {
	log.Printf("LMTP: Processing message data for %d recipients", len(s.users))

	// Read message data
	rawMessage, err := io.ReadAll(r)
	if err != nil {
		log.Printf("LMTP: Error reading message data: %v", err)
		return &smtp.SMTPError{
			Code:    450,
			Message: "Error reading message data",
		}
	}

	// Process message for each recipient
	responses := make([]error, len(s.users))
	for i, userRecipient := range s.users {
		err := s.processMessage(rawMessage, userRecipient)
		if err != nil {
			log.Printf("LMTP: Error processing message for %s: %v", userRecipient.Recipient, err)
			responses[i] = &smtp.SMTPError{
				Code:    450,
				Message: fmt.Sprintf("Error processing message: %v", err),
			}
		} else {
			log.Printf("LMTP: Message processed successfully for %s", userRecipient.Recipient)
			responses[i] = nil
		}
	}

	// For LMTP, we need to return the status for all recipients
	// Since go-smtp doesn't support per-recipient responses directly,
	// we'll return success if at least one delivery succeeded
	for _, err := range responses {
		if err == nil {
			return nil // At least one succeeded
		}
	}

	// All failed
	return &smtp.SMTPError{
		Code:    450,
		Message: "Failed to deliver to all recipients",
	}
}

// processMessage processes a message for a specific recipient
func (s *Session) processMessage(rawMessage []byte, userRecipient UserRecipient) error {
	ctx := context.Background()
	
	// Add Delivered-To header
	deliveredToHeader := fmt.Sprintf("Delivered-To: %s\r\n", userRecipient.Recipient)
	messageWithHeaders := append([]byte(deliveredToHeader), rawMessage...)

	// Parse and index the message
	envelope, bodyStructure, err := s.server.indexer.ProcessMessage(messageWithHeaders)
	if err != nil {
		return fmt.Errorf("failed to process message: %w", err)
	}

	// Apply filters
	filters, err := s.getUserFilters(userRecipient.User.ID)
	if err != nil {
		log.Printf("LMTP: Error getting filters for user %s: %v", userRecipient.User.ID.Hex(), err)
		filters = []Filter{} // Continue with empty filters
	}

	// Add spam filter if configured
	if s.server.config.SpamHeader != "" {
		spamFilter := Filter{
			ID: "SPAM",
			Query: FilterQuery{
				Headers: map[string]string{
					strings.ToLower(s.server.config.SpamHeader): "yes",
				},
			},
			Action: map[string]interface{}{
				"spam": true,
			},
		}
		filters = append(filters, spamFilter)
	}

	// Determine target mailbox and flags
	mailboxPath := "INBOX"
	flags := []string{}
	deleteMessage := false

	for _, filter := range filters {
		if s.matchesFilter(filter, envelope, bodyStructure, messageWithHeaders) {
			log.Printf("LMTP: Filter %s matched for user %s", filter.ID, userRecipient.User.ID.Hex())
			
			// Apply filter actions
			if action, ok := filter.Action["spam"].(bool); ok && action {
				mailboxPath = "Junk"
			}
			if action, ok := filter.Action["seen"].(bool); ok && action {
				flags = append(flags, "\\Seen")
			}
			if action, ok := filter.Action["flag"].(bool); ok && action {
				flags = append(flags, "\\Flagged")
			}
			if action, ok := filter.Action["delete"].(bool); ok && action {
				deleteMessage = true
				break
			}
			if mailbox, ok := filter.Action["mailbox"].(string); ok && mailbox != "" {
				mailboxPath = mailbox
			}
		}
	}

	// If message should be deleted, skip storage
	if deleteMessage {
		log.Printf("LMTP: Message deleted by filter for user %s", userRecipient.User.ID.Hex())
		return nil
	}

	// Find target mailbox
	var mailbox models.Mailbox
	mailboxFilter := bson.M{"user": userRecipient.User.ID}
	
	// Try to find by special use first (for Junk), then by path
	if mailboxPath == "Junk" {
		mailboxFilter["specialUse"] = "\\Junk"
	} else {
		mailboxFilter["path"] = mailboxPath
	}

	err = s.server.db.Collection("mailboxes").FindOne(ctx, mailboxFilter).Decode(&mailbox)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			// Fallback to INBOX if target mailbox doesn't exist
			err = s.server.db.Collection("mailboxes").FindOne(ctx, 
				bson.M{"user": userRecipient.User.ID, "path": "INBOX"}).Decode(&mailbox)
			if err != nil {
				return fmt.Errorf("failed to find INBOX: %w", err)
			}
		} else {
			return fmt.Errorf("failed to find mailbox: %w", err)
		}
	}

	// Get next UID for mailbox
	result, err := s.server.db.Collection("mailboxes").UpdateOne(
		ctx,
		bson.M{"_id": mailbox.ID},
		bson.M{"$inc": bson.M{"uidNext": 1, "modifyIndex": 1}},
	)
	if err != nil || result.ModifiedCount == 0 {
		return fmt.Errorf("failed to update mailbox UID: %w", err)
	}

	// Create message document
	now := time.Now().UTC()
	messageDoc := models.Message{
		User:     userRecipient.User.ID,
		Mailbox:  mailbox.ID,
		UID:      mailbox.UIDNext,
		ModSeq:   mailbox.ModifyIndex + 1,
		Size:     int64(len(messageWithHeaders)),
		Flags:    flags,
		Subject:  envelope.Subject,
		Date:     envelope.Date,
		Unseen:   !contains(flags, "\\Seen"),
		Undeleted: true,
		Flagged:  contains(flags, "\\Flagged"),
		Draft:    contains(flags, "\\Draft"),
		Created:  now,
	}

	// Set From address from envelope
	if len(envelope.From) > 0 {
		messageDoc.Meta.From = envelope.From[0].Address
		messageDoc.MimeTree.ParsedHeader.From = envelope.From
	}

	// Set To addresses
	if len(envelope.To) > 0 {
		messageDoc.MimeTree.ParsedHeader.To = envelope.To
	}

	// Set other envelope data
	messageDoc.MimeTree.ParsedHeader.Subject = envelope.Subject
	messageDoc.MimeTree.ParsedHeader.Date = envelope.Date

	// Store message
	_, err = s.server.db.Collection("messages").InsertOne(ctx, messageDoc)
	if err != nil {
		return fmt.Errorf("failed to store message: %w", err)
	}

	// Update user storage quota
	_, err = s.server.db.Collection("users").UpdateOne(
		ctx,
		bson.M{"_id": userRecipient.User.ID},
		bson.M{"$inc": bson.M{"storageUsed": messageDoc.Size}},
	)
	if err != nil {
		log.Printf("LMTP: Warning - failed to update user storage quota: %v", err)
	}

	log.Printf("LMTP: Message stored successfully for user %s in mailbox %s", 
		userRecipient.User.ID.Hex(), mailbox.Path)

	return nil
}

// getUserFilters retrieves filters for a user
func (s *Session) getUserFilters(userID primitive.ObjectID) ([]Filter, error) {
	// For now, return empty filters
	// In a full implementation, this would fetch user-specific filters from the database
	return []Filter{}, nil
}

// matchesFilter checks if a message matches a filter
func (s *Session) matchesFilter(filter Filter, envelope *indexer.Envelope, bodyStructure *indexer.BodyStructure, rawMessage []byte) bool {
	// Check header filters
	if len(filter.Query.Headers) > 0 {
		messageStr := strings.ToLower(string(rawMessage))
		for header, value := range filter.Query.Headers {
			headerPattern := fmt.Sprintf("%s:\\s*%s", strings.ToLower(header), strings.ToLower(value))
			matched, _ := regexp.MatchString(headerPattern, messageStr)
			if !matched {
				return false
			}
		}
	}

	// Check attachment filter
	if filter.Query.HasAttachments != nil {
		hasAttachments := bodyStructure != nil && len(bodyStructure.Attachments) > 0
		if *filter.Query.HasAttachments > 0 && !hasAttachments {
			return false
		}
		if *filter.Query.HasAttachments < 0 && hasAttachments {
			return false
		}
	}

	// Check size filter
	if filter.Query.Size != nil {
		messageSize := int64(len(rawMessage))
		filterSize := *filter.Query.Size
		if filterSize < 0 && messageSize > -filterSize {
			return false
		}
		if filterSize > 0 && messageSize < filterSize {
			return false
		}
	}

	// Check text filter
	if filter.Query.Text != "" {
		messageText := strings.ToLower(string(rawMessage))
		if !strings.Contains(messageText, strings.ToLower(filter.Query.Text)) {
			return false
		}
	}

	return true
}

// Reset resets the session
func (s *Session) Reset() {
	s.users = make([]UserRecipient, 0)
}

// Logout closes the session
func (s *Session) Logout() error {
	return nil
}

// Helper functions

// normalizeAddress normalizes an email address
func normalizeAddress(addr string) string {
	return strings.ToLower(strings.TrimSpace(addr))
}

// removeAddressTag removes the +tag part from an email address
func removeAddressTag(addr string) string {
	re := regexp.MustCompile(`\+[^@]*@`)
	return re.ReplaceAllString(addr, "@")
}

// contains checks if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}