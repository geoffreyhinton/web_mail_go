package imapcore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// handleDelete handles the DELETE command
func (s *Session) handleDelete(tag, args string) error {
	if !s.authenticated {
		return s.writeResponse(tag, "NO", "Not authenticated")
	}

	mailboxPath := parseMailboxPath(args)
	if mailboxPath == "" {
		return s.writeResponse(tag, "BAD", "Invalid mailbox name")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Find mailbox
	var mailbox Mailbox
	err := s.server.options.Database.Collection("mailboxes").FindOne(ctx, bson.M{
		"user": s.user.ID,
		"path": mailboxPath,
	}).Decode(&mailbox)

	if err != nil {
		return s.writeResponse(tag, "NO", "[NONEXISTENT] Mailbox does not exist")
	}

	// Check if it's a special mailbox
	if mailbox.SpecialUse != "" {
		return s.writeResponse(tag, "NO", "[CANNOT] Cannot delete special mailbox")
	}

	// Delete all messages in the mailbox
	_, err = s.server.options.Database.Collection("messages").DeleteMany(ctx, bson.M{
		"mailbox": mailbox.ID,
	})
	if err != nil {
		return s.writeResponse(tag, "NO", "Failed to delete messages")
	}

	// Delete the mailbox
	_, err = s.server.options.Database.Collection("mailboxes").DeleteOne(ctx, bson.M{
		"_id": mailbox.ID,
	})
	if err != nil {
		return s.writeResponse(tag, "NO", "Failed to delete mailbox")
	}

	return s.writeResponse(tag, "OK", "DELETE completed")
}

// handleRename handles the RENAME command
func (s *Session) handleRename(tag, args string) error {
	if !s.authenticated {
		return s.writeResponse(tag, "NO", "Not authenticated")
	}

	parts := parseQuotedArguments(args)
	if len(parts) != 2 {
		return s.writeResponse(tag, "BAD", "RENAME expects old and new mailbox names")
	}

	oldPath := parts[0]
	newPath := parts[1]

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Check if new mailbox already exists
	count, err := s.server.options.Database.Collection("mailboxes").CountDocuments(ctx, bson.M{
		"user": s.user.ID,
		"path": newPath,
	})
	if err != nil {
		return s.writeResponse(tag, "NO", "Database error")
	}
	if count > 0 {
		return s.writeResponse(tag, "NO", "[ALREADYEXISTS] Destination mailbox already exists")
	}

	// Rename the mailbox
	result, err := s.server.options.Database.Collection("mailboxes").UpdateOne(ctx, bson.M{
		"user": s.user.ID,
		"path": oldPath,
	}, bson.M{
		"$set": bson.M{"path": newPath},
	})

	if err != nil {
		return s.writeResponse(tag, "NO", "Database error")
	}
	if result.MatchedCount == 0 {
		return s.writeResponse(tag, "NO", "[NONEXISTENT] Source mailbox does not exist")
	}

	return s.writeResponse(tag, "OK", "RENAME completed")
}

// handleSubscribe handles the SUBSCRIBE command
func (s *Session) handleSubscribe(tag, args string) error {
	if !s.authenticated {
		return s.writeResponse(tag, "NO", "Not authenticated")
	}

	mailboxPath := parseMailboxPath(args)
	if mailboxPath == "" {
		return s.writeResponse(tag, "BAD", "Invalid mailbox name")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := s.server.options.Database.Collection("mailboxes").UpdateOne(ctx, bson.M{
		"user": s.user.ID,
		"path": mailboxPath,
	}, bson.M{
		"$set": bson.M{"subscribed": true},
	})

	if err != nil {
		return s.writeResponse(tag, "NO", "Database error")
	}
	if result.MatchedCount == 0 {
		return s.writeResponse(tag, "NO", "[NONEXISTENT] Mailbox does not exist")
	}

	return s.writeResponse(tag, "OK", "SUBSCRIBE completed")
}

// handleUnsubscribe handles the UNSUBSCRIBE command
func (s *Session) handleUnsubscribe(tag, args string) error {
	if !s.authenticated {
		return s.writeResponse(tag, "NO", "Not authenticated")
	}

	mailboxPath := parseMailboxPath(args)
	if mailboxPath == "" {
		return s.writeResponse(tag, "BAD", "Invalid mailbox name")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := s.server.options.Database.Collection("mailboxes").UpdateOne(ctx, bson.M{
		"user": s.user.ID,
		"path": mailboxPath,
	}, bson.M{
		"$set": bson.M{"subscribed": false},
	})

	if err != nil {
		return s.writeResponse(tag, "NO", "Database error")
	}
	if result.MatchedCount == 0 {
		return s.writeResponse(tag, "NO", "[NONEXISTENT] Mailbox does not exist")
	}

	return s.writeResponse(tag, "OK", "UNSUBSCRIBE completed")
}

// handleStatus handles the STATUS command
func (s *Session) handleStatus(tag, args string) error {
	if !s.authenticated {
		return s.writeResponse(tag, "NO", "Not authenticated")
	}

	// Parse mailbox and status items
	parts := strings.SplitN(args, " ", 2)
	if len(parts) != 2 {
		return s.writeResponse(tag, "BAD", "STATUS expects mailbox and status items")
	}

	mailboxPath := parseMailboxPath(parts[0])
	statusItems := parts[1]

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Find mailbox
	var mailbox Mailbox
	err := s.server.options.Database.Collection("mailboxes").FindOne(ctx, bson.M{
		"user": s.user.ID,
		"path": mailboxPath,
	}).Decode(&mailbox)

	if err != nil {
		return s.writeResponse(tag, "NO", "[NONEXISTENT] Mailbox does not exist")
	}

	// Count total messages
	totalCount, err := s.server.options.Database.Collection("messages").CountDocuments(ctx, bson.M{
		"mailbox": mailbox.ID,
	})
	if err != nil {
		return s.writeResponse(tag, "NO", "Database error")
	}

	// Count unseen messages
	unseenCount, err := s.server.options.Database.Collection("messages").CountDocuments(ctx, bson.M{
		"mailbox": mailbox.ID,
		"seen":    false,
	})
	if err != nil {
		return s.writeResponse(tag, "NO", "Database error")
	}

	// Build status response
	var statusPairs []string

	// Parse requested items (simplified parsing)
	if strings.Contains(statusItems, "MESSAGES") {
		statusPairs = append(statusPairs, fmt.Sprintf("MESSAGES %d", totalCount))
	}
	if strings.Contains(statusItems, "RECENT") {
		statusPairs = append(statusPairs, "RECENT 0") // Simplified
	}
	if strings.Contains(statusItems, "UIDNEXT") {
		statusPairs = append(statusPairs, fmt.Sprintf("UIDNEXT %d", mailbox.UIDNext))
	}
	if strings.Contains(statusItems, "UIDVALIDITY") {
		statusPairs = append(statusPairs, fmt.Sprintf("UIDVALIDITY %d", mailbox.UIDValidity))
	}
	if strings.Contains(statusItems, "UNSEEN") {
		statusPairs = append(statusPairs, fmt.Sprintf("UNSEEN %d", unseenCount))
	}

	response := fmt.Sprintf(`STATUS "%s" (%s)`, mailboxPath, strings.Join(statusPairs, " "))
	if err := s.writeUntaggedResponse(response); err != nil {
		return err
	}

	return s.writeResponse(tag, "OK", "STATUS completed")
}

// handleAppend handles the APPEND command
func (s *Session) handleAppend(tag, args string) error {
	if !s.authenticated {
		return s.writeResponse(tag, "NO", "Not authenticated")
	}

	// Parse APPEND arguments (simplified)
	parts := parseQuotedArguments(args)
	if len(parts) < 2 {
		return s.writeResponse(tag, "BAD", "APPEND expects mailbox and message")
	}

	mailboxPath := parts[0]
	// In a full implementation, you would parse flags, date, and message literal
	// This is a simplified version

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Find mailbox
	var mailbox Mailbox
	err := s.server.options.Database.Collection("mailboxes").FindOne(ctx, bson.M{
		"user": s.user.ID,
		"path": mailboxPath,
	}).Decode(&mailbox)

	if err != nil {
		return s.writeResponse(tag, "NO", "[TRYCREATE] Mailbox does not exist")
	}

	// For a complete implementation, you would:
	// 1. Read the message literal from the client
	// 2. Parse the RFC822 message using the indexer
	// 3. Store the message in the database
	// 4. Update mailbox UID counters

	// Simplified response
	return s.writeResponse(tag, "NO", "APPEND not fully implemented")
}

// handleClose handles the CLOSE command
func (s *Session) handleClose(tag string) error {
	if s.state != StateSelected {
		return s.writeResponse(tag, "BAD", "No mailbox selected")
	}

	// Expunge deleted messages silently
	if s.selectedBox != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err := s.server.options.Database.Collection("messages").DeleteMany(ctx, bson.M{
			"mailbox": s.selectedBox.ID,
			"deleted": true,
		})
		if err != nil {
			s.server.options.Logger.Error("[%s] Failed to expunge messages during CLOSE: %v", s.ID, err)
		}
	}

	s.selectedBox = nil
	s.state = StateAuthenticated

	return s.writeResponse(tag, "OK", "CLOSE completed")
}

// handleExpunge handles the EXPUNGE command
func (s *Session) handleExpunge(tag string) error {
	if s.state != StateSelected {
		return s.writeResponse(tag, "BAD", "No mailbox selected")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Find messages marked for deletion
	cursor, err := s.server.options.Database.Collection("messages").Find(ctx, bson.M{
		"mailbox": s.selectedBox.ID,
		"deleted": true,
	}, options.Find().SetSort(bson.M{"uid": 1}))

	if err != nil {
		return s.writeResponse(tag, "NO", "Database error")
	}
	defer cursor.Close(ctx)

	var deletedUIDs []int64
	for cursor.Next(ctx) {
		var msg Message
		if err := cursor.Decode(&msg); err != nil {
			continue
		}
		deletedUIDs = append(deletedUIDs, msg.UID)
	}

	// Delete the messages
	_, err = s.server.options.Database.Collection("messages").DeleteMany(ctx, bson.M{
		"mailbox": s.selectedBox.ID,
		"deleted": true,
	})
	if err != nil {
		return s.writeResponse(tag, "NO", "Failed to delete messages")
	}

	// Send EXPUNGE responses
	for _, uid := range deletedUIDs {
		if err := s.writeUntaggedResponse(fmt.Sprintf("%d EXPUNGE", uid)); err != nil {
			return err
		}
	}

	return s.writeResponse(tag, "OK", "EXPUNGE completed")
}

// handleGetQuotaRoot handles the GETQUOTAROOT command
func (s *Session) handleGetQuotaRoot(tag, args string) error {
	if !s.authenticated {
		return s.writeResponse(tag, "NO", "Not authenticated")
	}

	mailboxPath := parseMailboxPath(args)

	// Send quota root response
	if err := s.writeUntaggedResponse(fmt.Sprintf(`QUOTAROOT "%s" ""`, mailboxPath)); err != nil {
		return err
	}

	// Send quota response
	quota := s.user.Quota
	if quota == 0 {
		quota = s.server.options.MaxStorage
	}

	used := s.user.Used
	quotaKB := quota / 1024
	usedKB := used / 1024

	if err := s.writeUntaggedResponse(fmt.Sprintf(`QUOTA "" (STORAGE %d %d)`, usedKB, quotaKB)); err != nil {
		return err
	}

	return s.writeResponse(tag, "OK", "GETQUOTAROOT completed")
}

// handleGetQuota handles the GETQUOTA command
func (s *Session) handleGetQuota(tag, args string) error {
	if !s.authenticated {
		return s.writeResponse(tag, "NO", "Not authenticated")
	}

	quotaRoot := parseMailboxPath(args)
	if quotaRoot != "" {
		return s.writeResponse(tag, "NO", "[NONEXISTENT] Quota root does not exist")
	}

	quota := s.user.Quota
	if quota == 0 {
		quota = s.server.options.MaxStorage
	}

	used := s.user.Used
	quotaKB := quota / 1024
	usedKB := used / 1024

	if err := s.writeUntaggedResponse(fmt.Sprintf(`QUOTA "" (STORAGE %d %d)`, usedKB, quotaKB)); err != nil {
		return err
	}

	return s.writeResponse(tag, "OK", "GETQUOTA completed")
}
