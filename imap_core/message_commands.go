package imapcore

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// handleFetch handles the FETCH command
func (s *Session) handleFetch(tag, args string) error {
	if s.state != StateSelected {
		return s.writeResponse(tag, "BAD", "No mailbox selected")
	}

	// Parse sequence set and fetch items
	parts := strings.SplitN(args, " ", 2)
	if len(parts) != 2 {
		return s.writeResponse(tag, "BAD", "FETCH expects sequence set and items")
	}

	seqSet := parts[0]
	fetchItems := strings.ToUpper(parts[1])

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Parse sequence set (simplified - handles single numbers and ranges)
	var filter bson.M
	if strings.Contains(seqSet, ":") {
		// Range
		rangeParts := strings.Split(seqSet, ":")
		if len(rangeParts) == 2 {
			start, _ := strconv.Atoi(rangeParts[0])
			end := rangeParts[1]
			if end == "*" {
				filter = bson.M{"mailbox": s.selectedBox.ID, "uid": bson.M{"$gte": start}}
			} else {
				endNum, _ := strconv.Atoi(end)
				filter = bson.M{"mailbox": s.selectedBox.ID, "uid": bson.M{"$gte": start, "$lte": endNum}}
			}
		}
	} else if seqSet == "*" {
		// Last message
		filter = bson.M{"mailbox": s.selectedBox.ID}
	} else {
		// Single number
		uid, _ := strconv.Atoi(seqSet)
		filter = bson.M{"mailbox": s.selectedBox.ID, "uid": uid}
	}

	cursor, err := s.server.options.Database.Collection("messages").Find(ctx, filter,
		options.Find().SetSort(bson.M{"uid": 1}))
	if err != nil {
		return s.writeResponse(tag, "NO", "Database error")
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var msg Message
		if err := cursor.Decode(&msg); err != nil {
			continue
		}

		response := s.buildFetchResponse(msg, fetchItems)
		if err := s.writeUntaggedResponse(response); err != nil {
			return err
		}
	}

	return s.writeResponse(tag, "OK", "FETCH completed")
}

// buildFetchResponse builds a FETCH response based on requested items
func (s *Session) buildFetchResponse(msg Message, items string) string {
	var parts []string

	if strings.Contains(items, "UID") {
		parts = append(parts, fmt.Sprintf("UID %d", msg.UID))
	}

	if strings.Contains(items, "FLAGS") {
		flags := s.buildFlagsString(msg)
		parts = append(parts, fmt.Sprintf("FLAGS (%s)", flags))
	}

	if strings.Contains(items, "INTERNALDATE") {
		parts = append(parts, fmt.Sprintf(`INTERNALDATE "%s"`, msg.Date.Format("02-Jan-2006 15:04:05 -0700")))
	}

	if strings.Contains(items, "RFC822.SIZE") {
		parts = append(parts, fmt.Sprintf("RFC822.SIZE %d", msg.Size))
	}

	if strings.Contains(items, "ENVELOPE") {
		envelope := s.buildEnvelope(msg)
		parts = append(parts, fmt.Sprintf("ENVELOPE %s", envelope))
	}

	if strings.Contains(items, "BODYSTRUCTURE") {
		bodyStructure := s.buildBodyStructure(msg)
		parts = append(parts, fmt.Sprintf("BODYSTRUCTURE %s", bodyStructure))
	}

	if strings.Contains(items, "BODY.PEEK[HEADER]") || strings.Contains(items, "BODY[HEADER]") {
		header := s.buildHeaderString(msg)
		parts = append(parts, fmt.Sprintf("BODY[HEADER] {%d}\r\n%s", len(header), header))

		// Mark as seen if not PEEK
		if strings.Contains(items, "BODY[HEADER]") && !msg.Seen {
			s.markMessageSeen(msg.ID)
		}
	}

	if strings.Contains(items, "BODY.PEEK[TEXT]") || strings.Contains(items, "BODY[TEXT]") {
		text := msg.BodyText
		if text == "" {
			text = msg.BodyHTML // Fallback to HTML
		}
		parts = append(parts, fmt.Sprintf("BODY[TEXT] {%d}\r\n%s", len(text), text))

		// Mark as seen if not PEEK
		if strings.Contains(items, "BODY[TEXT]") && !msg.Seen {
			s.markMessageSeen(msg.ID)
		}
	}

	if strings.Contains(items, "RFC822") {
		// This would return the full RFC822 message
		// For now, return a placeholder
		fullMsg := s.buildFullMessage(msg)
		parts = append(parts, fmt.Sprintf("RFC822 {%d}\r\n%s", len(fullMsg), fullMsg))

		// Mark as seen
		if !msg.Seen {
			s.markMessageSeen(msg.ID)
		}
	}

	return fmt.Sprintf("%d FETCH (%s)", msg.UID, strings.Join(parts, " "))
}

// buildFlagsString builds the flags string for a message
func (s *Session) buildFlagsString(msg Message) string {
	var flags []string

	if msg.Seen {
		flags = append(flags, "\\Seen")
	}
	if msg.Answered {
		flags = append(flags, "\\Answered")
	}
	if msg.Flagged {
		flags = append(flags, "\\Flagged")
	}
	if msg.Deleted {
		flags = append(flags, "\\Deleted")
	}
	if msg.Draft {
		flags = append(flags, "\\Draft")
	}

	return strings.Join(flags, " ")
}

// buildEnvelope builds the ENVELOPE structure for a message
func (s *Session) buildEnvelope(msg Message) string {
	// ENVELOPE structure: date subject from sender reply-to to cc bcc in-reply-to message-id
	date := fmt.Sprintf(`"%s"`, msg.Date.Format("Mon, 02 Jan 2006 15:04:05 -0700"))
	subject := s.quoteString(msg.Subject)
	from := s.buildAddressList([]string{msg.From})
	sender := from  // Usually same as from
	replyTo := from // Simplified
	to := s.buildAddressList(msg.To)
	cc := s.buildAddressList(msg.CC)
	bcc := "NIL" // Usually not included
	inReplyTo := s.quoteString(msg.InReplyTo)
	messageID := s.quoteString(msg.MessageID)

	return fmt.Sprintf("(%s %s %s %s %s %s %s %s %s %s)",
		date, subject, from, sender, replyTo, to, cc, bcc, inReplyTo, messageID)
}

// buildAddressList builds an address list for ENVELOPE
func (s *Session) buildAddressList(addresses []string) string {
	if len(addresses) == 0 {
		return "NIL"
	}

	var addressStructs []string
	for _, addr := range addresses {
		// Parse email address (simplified)
		parts := strings.Split(addr, "@")
		if len(parts) == 2 {
			local := parts[0]
			domain := parts[1]
			addressStructs = append(addressStructs, fmt.Sprintf(`(NIL NIL "%s" "%s")`, local, domain))
		}
	}

	if len(addressStructs) == 0 {
		return "NIL"
	}

	return fmt.Sprintf("(%s)", strings.Join(addressStructs, ""))
}

// buildBodyStructure builds the BODYSTRUCTURE for a message
func (s *Session) buildBodyStructure(msg Message) string {
	// Simplified BODYSTRUCTURE - would need full MIME parsing in real implementation
	if msg.BodyHTML != "" && msg.BodyText != "" {
		// Multipart alternative
		textPart := `("TEXT" "PLAIN" ("CHARSET" "UTF-8") NIL NIL "7BIT" %d NIL NIL NIL)`
		htmlPart := `("TEXT" "HTML" ("CHARSET" "UTF-8") NIL NIL "7BIT" %d NIL NIL NIL)`
		return fmt.Sprintf(`((%s)(%s) "ALTERNATIVE" ("BOUNDARY" "boundary123") NIL NIL NIL)`,
			fmt.Sprintf(textPart, len(msg.BodyText)),
			fmt.Sprintf(htmlPart, len(msg.BodyHTML)))
	} else if msg.BodyHTML != "" {
		return fmt.Sprintf(`("TEXT" "HTML" ("CHARSET" "UTF-8") NIL NIL "7BIT" %d NIL NIL NIL)`, len(msg.BodyHTML))
	} else {
		return fmt.Sprintf(`("TEXT" "PLAIN" ("CHARSET" "UTF-8") NIL NIL "7BIT" %d NIL NIL NIL)`, len(msg.BodyText))
	}
}

// buildHeaderString builds the header string for a message
func (s *Session) buildHeaderString(msg Message) string {
	var headers []string

	headers = append(headers, fmt.Sprintf("From: %s", msg.From))
	if len(msg.To) > 0 {
		headers = append(headers, fmt.Sprintf("To: %s", strings.Join(msg.To, ", ")))
	}
	if len(msg.CC) > 0 {
		headers = append(headers, fmt.Sprintf("Cc: %s", strings.Join(msg.CC, ", ")))
	}
	headers = append(headers, fmt.Sprintf("Subject: %s", msg.Subject))
	headers = append(headers, fmt.Sprintf("Date: %s", msg.Date.Format("Mon, 02 Jan 2006 15:04:05 -0700")))
	if msg.MessageID != "" {
		headers = append(headers, fmt.Sprintf("Message-ID: %s", msg.MessageID))
	}

	return strings.Join(headers, "\r\n") + "\r\n"
}

// buildFullMessage builds the full RFC822 message
func (s *Session) buildFullMessage(msg Message) string {
	header := s.buildHeaderString(msg)
	body := msg.BodyText
	if body == "" {
		body = msg.BodyHTML
	}
	return header + "\r\n" + body
}

// handleStore handles the STORE command
func (s *Session) handleStore(tag, args string) error {
	if s.state != StateSelected {
		return s.writeResponse(tag, "BAD", "No mailbox selected")
	}

	// Parse STORE arguments
	parts := strings.SplitN(args, " ", 3)
	if len(parts) != 3 {
		return s.writeResponse(tag, "BAD", "STORE expects sequence, action, and flags")
	}

	seqSet := parts[0]
	action := strings.ToUpper(parts[1])
	flagsStr := parts[2]

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Parse sequence set (simplified)
	var filter bson.M
	if seqSet == "*" {
		filter = bson.M{"mailbox": s.selectedBox.ID}
	} else {
		uid, _ := strconv.Atoi(seqSet)
		filter = bson.M{"mailbox": s.selectedBox.ID, "uid": uid}
	}

	// Parse flags
	flags := s.parseFlags(flagsStr)

	var update bson.M
	if strings.HasPrefix(action, "+FLAGS") {
		// Add flags
		update = bson.M{"$set": flags}
	} else if strings.HasPrefix(action, "-FLAGS") {
		// Remove flags
		unsetFlags := make(bson.M)
		for flag := range flags {
			unsetFlags[flag] = false
		}
		update = bson.M{"$set": unsetFlags}
	} else if action == "FLAGS" {
		// Replace flags
		update = bson.M{"$set": flags}
	} else {
		return s.writeResponse(tag, "BAD", "Invalid STORE action")
	}

	_, err := s.server.options.Database.Collection("messages").UpdateMany(ctx, filter, update)
	if err != nil {
		return s.writeResponse(tag, "NO", "Failed to update flags")
	}

	// Send FETCH response if not SILENT
	if !strings.Contains(action, "SILENT") {
		cursor, err := s.server.options.Database.Collection("messages").Find(ctx, filter)
		if err == nil {
			for cursor.Next(ctx) {
				var msg Message
				if err := cursor.Decode(&msg); err == nil {
					response := fmt.Sprintf("%d FETCH (FLAGS (%s))", msg.UID, s.buildFlagsString(msg))
					s.writeUntaggedResponse(response)
				}
			}
			cursor.Close(ctx)
		}
	}

	return s.writeResponse(tag, "OK", "STORE completed")
}

// parseFlags parses a flags string into a bson.M
func (s *Session) parseFlags(flagsStr string) bson.M {
	flags := bson.M{
		"seen":     false,
		"answered": false,
		"flagged":  false,
		"deleted":  false,
		"draft":    false,
	}

	flagsStr = strings.Trim(flagsStr, "()")
	flagList := strings.Fields(flagsStr)

	for _, flag := range flagList {
		switch strings.ToUpper(flag) {
		case "\\SEEN":
			flags["seen"] = true
		case "\\ANSWERED":
			flags["answered"] = true
		case "\\FLAGGED":
			flags["flagged"] = true
		case "\\DELETED":
			flags["deleted"] = true
		case "\\DRAFT":
			flags["draft"] = true
		}
	}

	return flags
}

// handleCopy handles the COPY command
func (s *Session) handleCopy(tag, args string) error {
	if s.state != StateSelected {
		return s.writeResponse(tag, "BAD", "No mailbox selected")
	}

	parts := strings.SplitN(args, " ", 2)
	if len(parts) != 2 {
		return s.writeResponse(tag, "BAD", "COPY expects sequence set and mailbox")
	}

	seqSet := parts[0]
	destMailbox := parseMailboxPath(parts[1])

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Find destination mailbox
	var destBox Mailbox
	err := s.server.options.Database.Collection("mailboxes").FindOne(ctx, bson.M{
		"user": s.user.ID,
		"path": destMailbox,
	}).Decode(&destBox)

	if err != nil {
		return s.writeResponse(tag, "NO", "[TRYCREATE] Destination mailbox does not exist")
	}

	// Parse sequence set and find messages
	var filter bson.M
	if seqSet == "*" {
		filter = bson.M{"mailbox": s.selectedBox.ID}
	} else {
		uid, _ := strconv.Atoi(seqSet)
		filter = bson.M{"mailbox": s.selectedBox.ID, "uid": uid}
	}

	cursor, err := s.server.options.Database.Collection("messages").Find(ctx, filter)
	if err != nil {
		return s.writeResponse(tag, "NO", "Database error")
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var msg Message
		if err := cursor.Decode(&msg); err != nil {
			continue
		}

		// Create copy with new UID
		newMsg := msg
		newMsg.ID = primitive.NewObjectID()
		newMsg.Mailbox = destBox.ID
		newMsg.UID = destBox.UIDNext

		_, err := s.server.options.Database.Collection("messages").InsertOne(ctx, newMsg)
		if err != nil {
			continue
		}

		// Update destination mailbox UID counter
		s.server.options.Database.Collection("mailboxes").UpdateOne(ctx, bson.M{
			"_id": destBox.ID,
		}, bson.M{
			"$inc": bson.M{"uidNext": 1},
		})

		destBox.UIDNext++
	}

	return s.writeResponse(tag, "OK", "COPY completed")
}

// markMessageSeen marks a message as seen
func (s *Session) markMessageSeen(msgID primitive.ObjectID) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	s.server.options.Database.Collection("messages").UpdateOne(ctx, bson.M{
		"_id": msgID,
	}, bson.M{
		"$set": bson.M{"seen": true},
	})
}

// handleMove handles the MOVE command (IMAP4rev1 extension)
func (s *Session) handleMove(tag, args string) error {
	if s.state != StateSelected {
		return s.writeResponse(tag, "BAD", "No mailbox selected")
	}

	parts := strings.SplitN(args, " ", 2)
	if len(parts) != 2 {
		return s.writeResponse(tag, "BAD", "MOVE expects sequence set and mailbox")
	}

	seqSet := parts[0]
	destMailbox := parseMailboxPath(parts[1])

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Find destination mailbox
	var destBox Mailbox
	err := s.server.options.Database.Collection("mailboxes").FindOne(ctx, bson.M{
		"user": s.user.ID,
		"path": destMailbox,
	}).Decode(&destBox)

	if err != nil {
		return s.writeResponse(tag, "NO", "[TRYCREATE] Destination mailbox does not exist")
	}

	// Parse sequence set and find messages
	var filter bson.M
	if seqSet == "*" {
		filter = bson.M{"mailbox": s.selectedBox.ID}
	} else {
		uid, _ := strconv.Atoi(seqSet)
		filter = bson.M{"mailbox": s.selectedBox.ID, "uid": uid}
	}

	cursor, err := s.server.options.Database.Collection("messages").Find(ctx, filter)
	if err != nil {
		return s.writeResponse(tag, "NO", "Database error")
	}
	defer cursor.Close(ctx)

	var movedUIDs []int64
	for cursor.Next(ctx) {
		var msg Message
		if err := cursor.Decode(&msg); err != nil {
			continue
		}

		// Move message to destination mailbox
		newUID := destBox.UIDNext
		_, err := s.server.options.Database.Collection("messages").UpdateOne(ctx, bson.M{
			"_id": msg.ID,
		}, bson.M{
			"$set": bson.M{
				"mailbox": destBox.ID,
				"uid":     newUID,
			},
		})
		if err != nil {
			continue
		}

		// Update destination mailbox UID counter
		s.server.options.Database.Collection("mailboxes").UpdateOne(ctx, bson.M{
			"_id": destBox.ID,
		}, bson.M{
			"$inc": bson.M{"uidNext": 1},
		})

		destBox.UIDNext++
		movedUIDs = append(movedUIDs, msg.UID)
	}

	// Send EXPUNGE responses for moved messages
	for _, uid := range movedUIDs {
		if err := s.writeUntaggedResponse(fmt.Sprintf("%d EXPUNGE", uid)); err != nil {
			return err
		}
	}

	return s.writeResponse(tag, "OK", "MOVE completed")
}

// quoteString quotes a string for IMAP responses
func (s *Session) quoteString(str string) string {
	if str == "" {
		return "NIL"
	}
	return fmt.Sprintf(`"%s"`, strings.ReplaceAll(str, `"`, `\"`))
}
