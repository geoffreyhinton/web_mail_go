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

// handleSearch handles the SEARCH command
func (s *Session) handleSearch(tag, args string) error {
	if s.state != StateSelected {
		return s.writeResponse(tag, "BAD", "No mailbox selected")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Parse search criteria
	criteria := strings.ToUpper(strings.TrimSpace(args))
	filter := s.buildSearchFilter(criteria)

	// Add mailbox filter
	filter["mailbox"] = s.selectedBox.ID

	cursor, err := s.server.options.Database.Collection("messages").Find(ctx, filter,
		options.Find().SetSort(bson.M{"uid": 1}))
	if err != nil {
		return s.writeResponse(tag, "NO", "Search failed")
	}
	defer cursor.Close(ctx)

	var uids []string
	for cursor.Next(ctx) {
		var msg Message
		if err := cursor.Decode(&msg); err != nil {
			continue
		}
		uids = append(uids, strconv.FormatInt(msg.UID, 10))
	}

	// Send search results
	response := fmt.Sprintf("SEARCH %s", strings.Join(uids, " "))
	if err := s.writeUntaggedResponse(response); err != nil {
		return err
	}

	return s.writeResponse(tag, "OK", "SEARCH completed")
}

// buildSearchFilter builds a MongoDB filter from IMAP search criteria
func (s *Session) buildSearchFilter(criteria string) bson.M {
	filter := bson.M{}

	// Split criteria into tokens
	tokens := s.parseSearchTokens(criteria)

	for i, token := range tokens {
		switch token {
		case "ALL":
			// No additional filter needed

		case "ANSWERED":
			filter["answered"] = true

		case "UNANSWERED":
			filter["answered"] = false

		case "DELETED":
			filter["deleted"] = true

		case "UNDELETED":
			filter["deleted"] = false

		case "FLAGGED":
			filter["flagged"] = true

		case "UNFLAGGED":
			filter["flagged"] = false

		case "SEEN":
			filter["seen"] = true

		case "UNSEEN":
			filter["seen"] = false

		case "NEW":
			filter["seen"] = false
			filter["recent"] = true

		case "OLD":
			filter["recent"] = false

		case "RECENT":
			filter["recent"] = true

		case "DRAFT":
			filter["draft"] = true

		case "UNDRAFT":
			filter["draft"] = false

		case "FROM":
			if i+1 < len(tokens) {
				pattern := s.unquoteString(tokens[i+1])
				filter["from"] = bson.M{"$regex": pattern, "$options": "i"}
			}

		case "TO":
			if i+1 < len(tokens) {
				pattern := s.unquoteString(tokens[i+1])
				filter["to"] = bson.M{"$regex": pattern, "$options": "i"}
			}

		case "CC":
			if i+1 < len(tokens) {
				pattern := s.unquoteString(tokens[i+1])
				filter["cc"] = bson.M{"$regex": pattern, "$options": "i"}
			}

		case "BCC":
			if i+1 < len(tokens) {
				pattern := s.unquoteString(tokens[i+1])
				filter["bcc"] = bson.M{"$regex": pattern, "$options": "i"}
			}

		case "SUBJECT":
			if i+1 < len(tokens) {
				pattern := s.unquoteString(tokens[i+1])
				filter["subject"] = bson.M{"$regex": pattern, "$options": "i"}
			}

		case "BODY":
			if i+1 < len(tokens) {
				pattern := s.unquoteString(tokens[i+1])
				filter["$or"] = []bson.M{
					{"bodyText": bson.M{"$regex": pattern, "$options": "i"}},
					{"bodyHTML": bson.M{"$regex": pattern, "$options": "i"}},
				}
			}

		case "TEXT":
			if i+1 < len(tokens) {
				pattern := s.unquoteString(tokens[i+1])
				filter["$or"] = []bson.M{
					{"subject": bson.M{"$regex": pattern, "$options": "i"}},
					{"from": bson.M{"$regex": pattern, "$options": "i"}},
					{"to": bson.M{"$regex": pattern, "$options": "i"}},
					{"bodyText": bson.M{"$regex": pattern, "$options": "i"}},
					{"bodyHTML": bson.M{"$regex": pattern, "$options": "i"}},
				}
			}

		case "HEADER":
			if i+2 < len(tokens) {
				headerName := strings.ToLower(s.unquoteString(tokens[i+1]))
				headerValue := s.unquoteString(tokens[i+2])

				switch headerName {
				case "from":
					filter["from"] = bson.M{"$regex": headerValue, "$options": "i"}
				case "to":
					filter["to"] = bson.M{"$regex": headerValue, "$options": "i"}
				case "subject":
					filter["subject"] = bson.M{"$regex": headerValue, "$options": "i"}
				case "message-id":
					filter["messageId"] = bson.M{"$regex": headerValue, "$options": "i"}
				}
			}

		case "LARGER":
			if i+1 < len(tokens) {
				size, err := strconv.Atoi(tokens[i+1])
				if err == nil {
					filter["size"] = bson.M{"$gt": size}
				}
			}

		case "SMALLER":
			if i+1 < len(tokens) {
				size, err := strconv.Atoi(tokens[i+1])
				if err == nil {
					filter["size"] = bson.M{"$lt": size}
				}
			}

		case "UID":
			if i+1 < len(tokens) {
				uidRange := tokens[i+1]
				if strings.Contains(uidRange, ":") {
					parts := strings.Split(uidRange, ":")
					if len(parts) == 2 {
						start, _ := strconv.ParseInt(parts[0], 10, 64)
						if parts[1] == "*" {
							filter["uid"] = bson.M{"$gte": start}
						} else {
							end, _ := strconv.ParseInt(parts[1], 10, 64)
							filter["uid"] = bson.M{"$gte": start, "$lte": end}
						}
					}
				} else {
					uid, _ := strconv.ParseInt(uidRange, 10, 64)
					filter["uid"] = uid
				}
			}

		case "BEFORE":
			if i+1 < len(tokens) {
				date := s.parseSearchDate(tokens[i+1])
				if !date.IsZero() {
					filter["date"] = bson.M{"$lt": date}
				}
			}

		case "ON":
			if i+1 < len(tokens) {
				date := s.parseSearchDate(tokens[i+1])
				if !date.IsZero() {
					nextDay := date.AddDate(0, 0, 1)
					filter["date"] = bson.M{"$gte": date, "$lt": nextDay}
				}
			}

		case "SINCE":
			if i+1 < len(tokens) {
				date := s.parseSearchDate(tokens[i+1])
				if !date.IsZero() {
					filter["date"] = bson.M{"$gte": date}
				}
			}
		}
	}

	return filter
}

// parseSearchTokens parses search criteria into tokens
func (s *Session) parseSearchTokens(criteria string) []string {
	var tokens []string
	var current string
	var inQuotes bool

	for i, char := range criteria {
		if char == '"' {
			inQuotes = !inQuotes
			current += string(char)
		} else if char == ' ' && !inQuotes {
			if current != "" {
				tokens = append(tokens, current)
				current = ""
			}
		} else {
			current += string(char)
		}

		// Handle end of string
		if i == len(criteria)-1 && current != "" {
			tokens = append(tokens, current)
		}
	}

	return tokens
}

// parseSearchDate parses a date string for search
func (s *Session) parseSearchDate(dateStr string) time.Time {
	// Remove quotes if present
	dateStr = strings.Trim(dateStr, `"`)

	// Try different date formats
	formats := []string{
		"2-Jan-2006",
		"02-Jan-2006",
		"2006-01-02",
		"01/02/2006",
	}

	for _, format := range formats {
		if date, err := time.Parse(format, dateStr); err == nil {
			return date
		}
	}

	return time.Time{}
}

// handleUIDSearch handles the UID SEARCH command
func (s *Session) handleUIDSearch(tag, args string) error {
	// UID SEARCH is identical to SEARCH in terms of filtering,
	// but the response format is different (already returns UIDs)
	return s.handleSearch(tag, args)
}

// handleUIDFetch handles the UID FETCH command
func (s *Session) handleUIDFetch(tag, args string) error {
	if s.state != StateSelected {
		return s.writeResponse(tag, "BAD", "No mailbox selected")
	}

	parts := strings.SplitN(args, " ", 2)
	if len(parts) != 2 {
		return s.writeResponse(tag, "BAD", "UID FETCH expects UID set and items")
	}

	uidSet := parts[0]
	fetchItems := strings.ToUpper(parts[1])

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Parse UID set
	var filter bson.M
	if strings.Contains(uidSet, ":") {
		// Range
		rangeParts := strings.Split(uidSet, ":")
		if len(rangeParts) == 2 {
			start, _ := strconv.ParseInt(rangeParts[0], 10, 64)
			end := rangeParts[1]
			if end == "*" {
				filter = bson.M{"mailbox": s.selectedBox.ID, "uid": bson.M{"$gte": start}}
			} else {
				endNum, _ := strconv.ParseInt(end, 10, 64)
				filter = bson.M{"mailbox": s.selectedBox.ID, "uid": bson.M{"$gte": start, "$lte": endNum}}
			}
		}
	} else if uidSet == "*" {
		// Last message
		filter = bson.M{"mailbox": s.selectedBox.ID}
	} else {
		// Single UID or comma-separated UIDs
		if strings.Contains(uidSet, ",") {
			uidList := strings.Split(uidSet, ",")
			var uids []int64
			for _, uidStr := range uidList {
				if uid, err := strconv.ParseInt(strings.TrimSpace(uidStr), 10, 64); err == nil {
					uids = append(uids, uid)
				}
			}
			filter = bson.M{"mailbox": s.selectedBox.ID, "uid": bson.M{"$in": uids}}
		} else {
			uid, _ := strconv.ParseInt(uidSet, 10, 64)
			filter = bson.M{"mailbox": s.selectedBox.ID, "uid": uid}
		}
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

		// Always include UID in UID FETCH
		items := fetchItems
		if !strings.Contains(items, "UID") {
			items = "UID " + items
		}

		response := s.buildFetchResponse(msg, items)
		if err := s.writeUntaggedResponse(response); err != nil {
			return err
		}
	}

	return s.writeResponse(tag, "OK", "UID FETCH completed")
}

// handleUIDStore handles the UID STORE command
func (s *Session) handleUIDStore(tag, args string) error {
	if s.state != StateSelected {
		return s.writeResponse(tag, "BAD", "No mailbox selected")
	}

	parts := strings.SplitN(args, " ", 3)
	if len(parts) != 3 {
		return s.writeResponse(tag, "BAD", "UID STORE expects UID set, action, and flags")
	}

	uidSet := parts[0]
	action := strings.ToUpper(parts[1])
	flagsStr := parts[2]

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Parse UID set
	var filter bson.M
	if uidSet == "*" {
		filter = bson.M{"mailbox": s.selectedBox.ID}
	} else if strings.Contains(uidSet, ",") {
		uidList := strings.Split(uidSet, ",")
		var uids []int64
		for _, uidStr := range uidList {
			if uid, err := strconv.ParseInt(strings.TrimSpace(uidStr), 10, 64); err == nil {
				uids = append(uids, uid)
			}
		}
		filter = bson.M{"mailbox": s.selectedBox.ID, "uid": bson.M{"$in": uids}}
	} else {
		uid, _ := strconv.ParseInt(uidSet, 10, 64)
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
					response := fmt.Sprintf("%d FETCH (UID %d FLAGS (%s))", msg.UID, msg.UID, s.buildFlagsString(msg))
					s.writeUntaggedResponse(response)
				}
			}
			cursor.Close(ctx)
		}
	}

	return s.writeResponse(tag, "OK", "UID STORE completed")
}

// handleUIDCopy handles the UID COPY command
func (s *Session) handleUIDCopy(tag, args string) error {
	if s.state != StateSelected {
		return s.writeResponse(tag, "BAD", "No mailbox selected")
	}

	parts := strings.SplitN(args, " ", 2)
	if len(parts) != 2 {
		return s.writeResponse(tag, "BAD", "UID COPY expects UID set and mailbox")
	}

	uidSet := parts[0]
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

	// Parse UID set
	var filter bson.M
	if uidSet == "*" {
		filter = bson.M{"mailbox": s.selectedBox.ID}
	} else {
		uid, _ := strconv.ParseInt(uidSet, 10, 64)
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

	return s.writeResponse(tag, "OK", "UID COPY completed")
}

// unquoteString removes quotes from a string
func (s *Session) unquoteString(str string) string {
	if len(str) >= 2 && str[0] == '"' && str[len(str)-1] == '"' {
		return str[1 : len(str)-1]
	}
	return str
}
