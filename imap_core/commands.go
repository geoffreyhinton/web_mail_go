package imapcore

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"golang.org/x/crypto/bcrypt"
)

// handleCapability handles the CAPABILITY command
func (s *Session) handleCapability(tag string) error {
	capabilities := strings.Join(s.capabilities, " ")
	if err := s.writeUntaggedResponse(fmt.Sprintf("CAPABILITY %s", capabilities)); err != nil {
		return err
	}
	return s.writeResponse(tag, "OK", "CAPABILITY completed")
}

// handleNoop handles the NOOP command
func (s *Session) handleNoop(tag string) error {
	return s.writeResponse(tag, "OK", "NOOP completed")
}

// handleLogout handles the LOGOUT command
func (s *Session) handleLogout(tag string) error {
	if err := s.writeUntaggedResponse("BYE IMAP4rev1 Server logging out"); err != nil {
		return err
	}
	s.state = StateLogout
	return s.writeResponse(tag, "OK", "LOGOUT completed")
}

// handleStartTLS handles the STARTTLS command
func (s *Session) handleStartTLS(tag string) error {
	if s.server.options.IgnoreSTARTTLS {
		return s.writeResponse(tag, "BAD", "STARTTLS not available")
	}

	if s.server.options.TLSConfig == nil {
		return s.writeResponse(tag, "BAD", "TLS not configured")
	}

	if err := s.writeResponse(tag, "OK", "Begin TLS negotiation now"); err != nil {
		return err
	}

	// Upgrade connection to TLS
	tlsConn := tls.Server(s.conn, s.server.options.TLSConfig)
	if err := tlsConn.Handshake(); err != nil {
		return fmt.Errorf("TLS handshake failed: %w", err)
	}

	s.conn = tlsConn
	s.scanner = bufio.NewScanner(tlsConn)
	s.writer = bufio.NewWriter(tlsConn)

	return nil
}

// handleLogin handles the LOGIN command
func (s *Session) handleLogin(tag, args string) error {
	if s.authenticated {
		return s.writeResponse(tag, "BAD", "Already authenticated")
	}

	parts := parseQuotedArguments(args)
	if len(parts) != 2 {
		return s.writeResponse(tag, "BAD", "LOGIN expects username and password")
	}

	username := parts[0]
	password := parts[1]

	// Find user in database
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var user User
	err := s.server.options.Database.Collection("users").FindOne(ctx, bson.M{
		"username": username,
	}).Decode(&user)

	if err != nil {
		s.server.options.Logger.Debug("[%s] Authentication failed for user: %s", s.ID, username)
		return s.writeResponse(tag, "NO", "Authentication failed")
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		s.server.options.Logger.Debug("[%s] Password verification failed for user: %s", s.ID, username)
		return s.writeResponse(tag, "NO", "Authentication failed")
	}

	s.authenticated = true
	s.user = &user
	s.state = StateAuthenticated

	s.server.options.Logger.Info("[%s] User authenticated: %s", s.ID, username)
	return s.writeResponse(tag, "OK", "LOGIN completed")
}

// handleAuthenticate handles the AUTHENTICATE command
func (s *Session) handleAuthenticate(tag, args string) error {
	return s.writeResponse(tag, "NO", "AUTHENTICATE not implemented")
}

// handleList handles the LIST command
func (s *Session) handleList(tag, args string) error {
	if !s.authenticated {
		return s.writeResponse(tag, "NO", "Not authenticated")
	}

	// Parse arguments - simplified parsing
	// Full implementation would need proper IMAP argument parsing

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cursor, err := s.server.options.Database.Collection("mailboxes").Find(ctx, bson.M{
		"user": s.user.ID,
	})
	if err != nil {
		return s.writeResponse(tag, "NO", "Database error")
	}
	defer cursor.Close(ctx)

	var mailboxes []Mailbox
	if err := cursor.All(ctx, &mailboxes); err != nil {
		return s.writeResponse(tag, "NO", "Failed to retrieve mailboxes")
	}

	for _, mailbox := range mailboxes {
		flags := `\HasNoChildren`
		if mailbox.SpecialUse != "" {
			flags = fmt.Sprintf(`\%s`, mailbox.SpecialUse)
		}

		response := fmt.Sprintf(`LIST (%s) "/" "%s"`, flags, mailbox.Path)
		if err := s.writeUntaggedResponse(response); err != nil {
			return err
		}
	}

	return s.writeResponse(tag, "OK", "LIST completed")
}

// handleLsub handles the LSUB command
func (s *Session) handleLsub(tag, args string) error {
	if !s.authenticated {
		return s.writeResponse(tag, "NO", "Not authenticated")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cursor, err := s.server.options.Database.Collection("mailboxes").Find(ctx, bson.M{
		"user":       s.user.ID,
		"subscribed": true,
	})
	if err != nil {
		return s.writeResponse(tag, "NO", "Database error")
	}
	defer cursor.Close(ctx)

	var mailboxes []Mailbox
	if err := cursor.All(ctx, &mailboxes); err != nil {
		return s.writeResponse(tag, "NO", "Failed to retrieve mailboxes")
	}

	for _, mailbox := range mailboxes {
		flags := `\HasNoChildren`
		response := fmt.Sprintf(`LSUB (%s) "/" "%s"`, flags, mailbox.Path)
		if err := s.writeUntaggedResponse(response); err != nil {
			return err
		}
	}

	return s.writeResponse(tag, "OK", "LSUB completed")
}

// handleSelect handles the SELECT command
func (s *Session) handleSelect(tag, args string) error {
	if !s.authenticated {
		return s.writeResponse(tag, "NO", "Not authenticated")
	}

	mailboxPath := parseMailboxPath(args)
	if mailboxPath == "" {
		return s.writeResponse(tag, "BAD", "Invalid mailbox name")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var mailbox Mailbox
	err := s.server.options.Database.Collection("mailboxes").FindOne(ctx, bson.M{
		"user": s.user.ID,
		"path": mailboxPath,
	}).Decode(&mailbox)

	if err != nil {
		return s.writeResponse(tag, "NO", "Mailbox does not exist")
	}

	// Get message count and UID list
	cursor, err := s.server.options.Database.Collection("messages").Find(ctx, bson.M{
		"mailbox": mailbox.ID,
	}, nil)
	if err != nil {
		return s.writeResponse(tag, "NO", "Failed to access messages")
	}
	defer cursor.Close(ctx)

	var messages []Message
	if err := cursor.All(ctx, &messages); err != nil {
		return s.writeResponse(tag, "NO", "Failed to retrieve messages")
	}

	// Build UID list
	mailbox.UIDList = make([]int64, len(messages))
	for i, msg := range messages {
		mailbox.UIDList[i] = msg.UID
	}

	s.selectedBox = &mailbox
	s.state = StateSelected

	// Send required untagged responses
	if err := s.writeUntaggedResponse(fmt.Sprintf("%d EXISTS", len(messages))); err != nil {
		return err
	}

	// Count recent messages (simplified - could be more complex)
	if err := s.writeUntaggedResponse("0 RECENT"); err != nil {
		return err
	}

	// Send flags
	flags := strings.Join(append([]string{"\\Answered", "\\Flagged", "\\Deleted", "\\Seen", "\\Draft"}, mailbox.Flags...), " ")
	if err := s.writeUntaggedResponse(fmt.Sprintf("FLAGS (%s)", flags)); err != nil {
		return err
	}

	if err := s.writeUntaggedResponse(fmt.Sprintf("OK [PERMANENTFLAGS (%s \\*)] Limited", flags)); err != nil {
		return err
	}

	if err := s.writeUntaggedResponse(fmt.Sprintf("OK [UIDNEXT %d] Predicted next UID", mailbox.UIDNext)); err != nil {
		return err
	}

	if err := s.writeUntaggedResponse(fmt.Sprintf("OK [UIDVALIDITY %d] UIDs valid", mailbox.UIDValidity)); err != nil {
		return err
	}

	return s.writeResponse(tag, "OK", fmt.Sprintf("[READ-WRITE] SELECT completed, now in selected state"))
}

// handleExamine handles the EXAMINE command (similar to SELECT but read-only)
func (s *Session) handleExamine(tag, args string) error {
	// Implementation similar to SELECT but set read-only mode
	err := s.handleSelect(tag, args)
	if err != nil {
		return err
	}

	// Override the final response to indicate read-only
	return s.writeResponse(tag, "OK", "[READ-ONLY] EXAMINE completed, now in selected state")
}

// handleCreate handles the CREATE command
func (s *Session) handleCreate(tag, args string) error {
	if !s.authenticated {
		return s.writeResponse(tag, "NO", "Not authenticated")
	}

	mailboxPath := parseMailboxPath(args)
	if mailboxPath == "" {
		return s.writeResponse(tag, "BAD", "Invalid mailbox name")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Check if mailbox already exists
	count, err := s.server.options.Database.Collection("mailboxes").CountDocuments(ctx, bson.M{
		"user": s.user.ID,
		"path": mailboxPath,
	})
	if err != nil {
		return s.writeResponse(tag, "NO", "Database error")
	}
	if count > 0 {
		return s.writeResponse(tag, "NO", "[ALREADYEXISTS] Mailbox already exists")
	}

	// Create new mailbox
	mailbox := Mailbox{
		ID:          primitive.NewObjectID(),
		User:        s.user.ID,
		Path:        mailboxPath,
		UIDValidity: time.Now().Unix(),
		UIDNext:     1,

		Subscribed: true,
		Flags:      []string{},
	}

	_, err = s.server.options.Database.Collection("mailboxes").InsertOne(ctx, mailbox)
	if err != nil {
		return s.writeResponse(tag, "NO", "Failed to create mailbox")
	}

	return s.writeResponse(tag, "OK", "CREATE completed")
}

// Helper functions

// parseQuotedArguments parses IMAP quoted arguments
func parseQuotedArguments(args string) []string {
	var result []string
	var current strings.Builder
	inQuotes := false
	escaped := false

	for _, char := range args {
		if escaped {
			current.WriteRune(char)
			escaped = false
			continue
		}

		switch char {
		case '\\':
			if inQuotes {
				escaped = true
			} else {
				current.WriteRune(char)
			}
		case '"':
			inQuotes = !inQuotes
		case ' ':
			if !inQuotes {
				if current.Len() > 0 {
					result = append(result, current.String())
					current.Reset()
				}
			} else {
				current.WriteRune(char)
			}
		default:
			current.WriteRune(char)
		}
	}

	if current.Len() > 0 {
		result = append(result, current.String())
	}

	return result
}

// parseMailboxPath extracts the mailbox path from arguments
func parseMailboxPath(args string) string {
	args = strings.TrimSpace(args)
	if strings.HasPrefix(args, "\"") && strings.HasSuffix(args, "\"") {
		return args[1 : len(args)-1]
	}
	return args
}

// writeResponse writes a tagged response
func (s *Session) writeResponse(tag, status, text string) error {
	response := fmt.Sprintf("%s %s %s\r\n", tag, status, text)
	_, err := s.writer.WriteString(response)
	if err != nil {
		return err
	}
	return s.writer.Flush()
}

// writeUntaggedResponse writes an untagged response
func (s *Session) writeUntaggedResponse(text string) error {
	response := fmt.Sprintf("* %s\r\n", text)
	_, err := s.writer.WriteString(response)
	if err != nil {
		return err
	}
	return s.writer.Flush()
}
