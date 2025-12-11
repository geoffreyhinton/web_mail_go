package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/geoffreyhinton/mail_go/api/models"
	"github.com/geoffreyhinton/mail_go/api/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MessageHandler struct {
	db *mongo.Database
}

func NewMessageHandler(db *mongo.Database) *MessageHandler {
	return &MessageHandler{db: db}
}

// GetMessages retrieves paginated messages from a mailbox
func (h *MessageHandler) GetMessages(c *gin.Context) {
	userID, err := utils.ParseObjectID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Invalid user ID"})
		return
	}

	mailboxID, err := utils.ParseObjectID(c.Param("mailboxId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Invalid mailbox ID"})
		return
	}

	limit := utils.ParseIntParam(c.Query("limit"), 20)
	page := utils.ParseIntParam(c.Query("page"), 1)
	order := c.Query("order")

	if limit > 250 {
		limit = 250
	}
	if limit < 1 {
		limit = 1
	}

	if order != "asc" && order != "desc" {
		order = "desc"
	}

	// Check if mailbox exists and belongs to user
	var mailbox models.Mailbox
	err = h.db.Collection("mailboxes").FindOne(
		context.Background(),
		bson.M{"_id": mailboxID, "user": userID},
	).Decode(&mailbox)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, models.APIError{Error: "Mailbox not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}

	// Build filter
	filter := bson.M{"mailbox": mailboxID}

	// Count total messages
	total, err := h.db.Collection("messages").CountDocuments(context.Background(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}

	// Calculate skip
	skip := (page - 1) * limit

	// Build sort order
	sortOrder := 1 // ascending
	if order == "desc" {
		sortOrder = -1
	}

	// Find messages
	opts := options.Find().
		SetLimit(int64(limit)).
		SetSkip(int64(skip)).
		SetSort(bson.D{{Key: "uid", Value: sortOrder}})

	cursor, err := h.db.Collection("messages").Find(context.Background(), filter, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}
	defer cursor.Close(context.Background())

	var messages []models.Message
	if err := cursor.All(context.Background(), &messages); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}

	// Convert to response format
	results := make([]gin.H, len(messages))
	for i, msg := range messages {
		// Get from address
		var from models.EmailAddress
		if len(msg.MimeTree.ParsedHeader.From) > 0 {
			from = msg.MimeTree.ParsedHeader.From[0]
		} else if len(msg.MimeTree.ParsedHeader.Sender) > 0 {
			from = msg.MimeTree.ParsedHeader.Sender[0]
		} else if msg.Meta.From != "" {
			from = models.EmailAddress{Name: "", Address: msg.Meta.From}
		}

		results[i] = gin.H{
			"id":          utils.FormatMessageID(msg.ID, msg.UID),
			"mailbox":     mailboxID.Hex(),
			"thread":      msg.Thread.Hex(),
			"from":        from,
			"subject":     msg.Subject,
			"date":        msg.Date,
			"intro":       msg.Intro,
			"attachments": msg.HasAttach,
			"seen":        msg.GetSeen(),
			"deleted":     msg.GetDeleted(),
			"flagged":     msg.Flagged,
			"draft":       msg.Draft,
		}
	}

	// Build pagination URLs
	baseURL := fmt.Sprintf("/api/users/%s/mailboxes/%s/messages", userID.Hex(), mailboxID.Hex())
	var prevUrl, nextUrl *string

	if page > 1 {
		prev := fmt.Sprintf("%s?page=%d&limit=%d&order=%s", baseURL, page-1, limit, order)
		prevUrl = &prev
	}

	if int64(page*limit) < total {
		next := fmt.Sprintf("%s?page=%d&limit=%d&order=%s", baseURL, page+1, limit, order)
		nextUrl = &next
	}

	response := gin.H{
		"success":    true,
		"total":      total,
		"page":       page,
		"specialUse": mailbox.SpecialUse,
		"results":    results,
	}

	if prevUrl != nil {
		response["prev"] = *prevUrl
	}
	if nextUrl != nil {
		response["next"] = *nextUrl
	}

	c.JSON(http.StatusOK, response)
}

// GetMessage retrieves a single message
func (h *MessageHandler) GetMessage(c *gin.Context) {
	userID, err := utils.ParseObjectID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Invalid user ID"})
		return
	}

	mailboxID, err := utils.ParseObjectID(c.Param("mailboxId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Invalid mailbox ID"})
		return
	}

	messageID, uid, err := utils.ParseMessageID(c.Param("messageId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Invalid message ID"})
		return
	}

	replaceCidLinks := utils.ParseBoolParam(c.Query("replaceCidLinks"))

	// Find the message
	var message models.Message
	err = h.db.Collection("messages").FindOne(
		context.Background(),
		bson.M{
			"_id":     messageID,
			"mailbox": mailboxID,
			"uid":     uid,
			"user":    userID,
		},
	).Decode(&message)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, models.APIError{Error: "Message not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}

	parsedHeader := message.MimeTree.ParsedHeader

	// Get from address
	var from []models.EmailAddress
	if len(parsedHeader.From) > 0 {
		from = parsedHeader.From
	} else if len(parsedHeader.Sender) > 0 {
		from = parsedHeader.Sender
	} else if message.Meta.From != "" {
		from = []models.EmailAddress{{Name: "", Address: message.Meta.From}}
	}

	// Process HTML content if replaceCidLinks is requested
	html := message.HTML
	if replaceCidLinks && len(html) > 0 {
		baseURL := fmt.Sprintf("/api/users/%s/mailboxes/%s/messages/%s/attachments/",
			userID.Hex(), mailboxID.Hex(), utils.FormatMessageID(messageID, uid))
		
		for i, content := range html {
			// Replace attachment: links with API URLs
			html[i] = strings.ReplaceAll(content, "attachment:", baseURL)
		}
	}

	// Prepare list information if available
	var list *gin.H
	if len(parsedHeader.ListID) > 0 || len(parsedHeader.ListUnsub) > 0 {
		listInfo := gin.H{}
		if len(parsedHeader.ListID) > 0 {
			listInfo["id"] = parsedHeader.ListID[0]
		}
		if len(parsedHeader.ListUnsub) > 0 {
			listInfo["unsubscribe"] = parsedHeader.ListUnsub
		}
		list = &listInfo
	}

	// Check for expiry
	var expires *string
	if message.Exp {
		expiryTime := message.Received.Format(time.RFC3339)
		expires = &expiryTime
	}

	response := gin.H{
		"success":   true,
		"id":        utils.FormatMessageID(messageID, uid),
		"subject":   message.Subject,
		"messageId": message.MessageID,
		"date":      message.Date,
		"seen":      message.GetSeen(),
		"deleted":   message.GetDeleted(),
		"flagged":   message.Flagged,
		"draft":     message.Draft,
		"html":      html,
		"attachments": message.Attachments,
	}

	if len(from) > 0 {
		response["from"] = from[0]
	}

	if len(parsedHeader.ReplyTo) > 0 {
		response["replyTo"] = parsedHeader.ReplyTo
	}

	if len(parsedHeader.To) > 0 {
		response["to"] = parsedHeader.To
	}

	if len(parsedHeader.CC) > 0 {
		response["cc"] = parsedHeader.CC
	}

	if list != nil {
		response["list"] = *list
	}

	if expires != nil {
		response["expires"] = *expires
	}

	c.JSON(http.StatusOK, response)
}

// UpdateMessage updates message flags and properties
func (h *MessageHandler) UpdateMessage(c *gin.Context) {
	userID, err := utils.ParseObjectID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Invalid user ID"})
		return
	}

	mailboxID, err := utils.ParseObjectID(c.Param("mailboxId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Invalid mailbox ID"})
		return
	}

	messageID, uid, err := utils.ParseMessageID(c.Param("messageId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Invalid message ID"})
		return
	}

	var req struct {
		NewMailbox *string    `json:"newMailbox,omitempty"`
		Seen       *bool      `json:"seen,omitempty"`
		Deleted    *bool      `json:"deleted,omitempty"`
		Flagged    *bool      `json:"flagged,omitempty"`
		Draft      *bool      `json:"draft,omitempty"`
		Expires    *time.Time `json:"expires,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{Error: err.Error()})
		return
	}

	// Handle mailbox move if requested
	if req.NewMailbox != nil {
		newMailboxID, err := utils.ParseObjectID(*req.NewMailbox)
		if err != nil {
			c.JSON(http.StatusBadRequest, models.APIError{Error: "Invalid new mailbox ID"})
			return
		}

		err = h.moveMessage(userID, mailboxID, newMailboxID, messageID, uid)
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
			return
		}

		// TODO: Return new message ID after move
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"mailbox": newMailboxID.Hex(),
		})
		return
	}

	// Update message flags
	update := bson.M{"$set": bson.M{}}
	addFlags := []string{}
	removeFlags := []string{}

	if req.Seen != nil {
		update["$set"].(bson.M)["unseen"] = !*req.Seen
		if *req.Seen {
			addFlags = append(addFlags, "\\Seen")
		} else {
			removeFlags = append(removeFlags, "\\Seen")
		}
	}

	if req.Deleted != nil {
		update["$set"].(bson.M)["undeleted"] = !*req.Deleted
		if *req.Deleted {
			addFlags = append(addFlags, "\\Deleted")
		} else {
			removeFlags = append(removeFlags, "\\Deleted")
		}
	}

	if req.Flagged != nil {
		update["$set"].(bson.M)["flagged"] = *req.Flagged
		if *req.Flagged {
			addFlags = append(addFlags, "\\Flagged")
		} else {
			removeFlags = append(removeFlags, "\\Flagged")
		}
	}

	if req.Draft != nil {
		update["$set"].(bson.M)["draft"] = *req.Draft
		if *req.Draft {
			addFlags = append(addFlags, "\\Draft")
		} else {
			removeFlags = append(removeFlags, "\\Draft")
		}
	}

	if req.Expires != nil {
		update["$set"].(bson.M)["exp"] = true
		update["$set"].(bson.M)["rdate"] = *req.Expires
	}

	if len(update["$set"].(bson.M)) == 0 {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Nothing was changed"})
		return
	}

	// Handle flag operations
	if len(addFlags) > 0 {
		if update["$addToSet"] == nil {
			update["$addToSet"] = bson.M{}
		}
		update["$addToSet"].(bson.M)["flags"] = bson.M{"$each": addFlags}
	}

	if len(removeFlags) > 0 {
		if update["$pull"] == nil {
			update["$pull"] = bson.M{}
		}
		update["$pull"].(bson.M)["flags"] = bson.M{"$in": removeFlags}
	}

	// Update mailbox modify index
	_, err = h.db.Collection("mailboxes").UpdateOne(
		context.Background(),
		bson.M{"_id": mailboxID, "user": userID},
		bson.M{"$inc": bson.M{"modifyIndex": 1}},
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}

	// Update the message
	result, err := h.db.Collection("messages").UpdateOne(
		context.Background(),
		bson.M{
			"_id":     messageID,
			"mailbox": mailboxID,
			"uid":     uid,
		},
		update,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, models.APIError{Error: "Message not found"})
		return
	}

	c.JSON(http.StatusOK, models.APISuccess{Success: true})
}

// DeleteMessage deletes a message
func (h *MessageHandler) DeleteMessage(c *gin.Context) {
	userID, err := utils.ParseObjectID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Invalid user ID"})
		return
	}

	mailboxID, err := utils.ParseObjectID(c.Param("mailboxId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Invalid mailbox ID"})
		return
	}

	messageID, uid, err := utils.ParseMessageID(c.Param("messageId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Invalid message ID"})
		return
	}

	// Find the message first to get its details
	var message models.Message
	err = h.db.Collection("messages").FindOne(
		context.Background(),
		bson.M{
			"_id":     messageID,
			"mailbox": mailboxID,
			"uid":     uid,
		},
	).Decode(&message)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, models.APIError{Error: "Message not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}

	// Delete the message
	result, err := h.db.Collection("messages").DeleteOne(
		context.Background(),
		bson.M{
			"_id":     messageID,
			"mailbox": mailboxID,
			"uid":     uid,
		},
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}

	// Update user storage quota
	if result.DeletedCount > 0 {
		_, err = h.db.Collection("users").UpdateOne(
			context.Background(),
			bson.M{"_id": userID},
			bson.M{"$inc": bson.M{"storageUsed": -message.Size}},
		)
		// Log error but don't fail the request
		if err != nil {
			// TODO: Add proper logging
		}
	}

	c.JSON(http.StatusOK, models.APISuccess{
		Success: result.DeletedCount > 0,
	})
}

// GetAttachment retrieves a message attachment
func (h *MessageHandler) GetAttachment(c *gin.Context) {
	userID, err := utils.ParseObjectID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Invalid user ID"})
		return
	}

	mailboxID, err := utils.ParseObjectID(c.Param("mailboxId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Invalid mailbox ID"})
		return
	}

	messageID, uid, err := utils.ParseMessageID(c.Param("messageId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Invalid message ID"})
		return
	}

	attachmentID := c.Param("attachmentId")

	// Find the message
	var message models.Message
	err = h.db.Collection("messages").FindOne(
		context.Background(),
		bson.M{
			"_id":     messageID,
			"mailbox": mailboxID,
			"uid":     uid,
			"user":    userID,
		},
		options.FindOne().SetProjection(bson.M{
			"attachments": 1,
			"map":         1,
		}),
	).Decode(&message)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, models.APIError{Error: "Message not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}

	// Find the attachment
	var attachment *models.Attachment
	for _, att := range message.Attachments {
		if att.ID == attachmentID {
			attachment = &att
			break
		}
	}

	if attachment == nil {
		c.JSON(http.StatusNotFound, models.APIError{Error: "Attachment not found"})
		return
	}

	// TODO: Implement actual file serving from GridFS
	// For now, return attachment metadata
	c.JSON(http.StatusOK, gin.H{
		"id":          attachment.ID,
		"filename":    attachment.Filename,
		"contentType": attachment.ContentType,
		"size":        attachment.Size,
		"disposition": attachment.Disposition,
	})
}

// SearchMessages searches messages for a user
func (h *MessageHandler) SearchMessages(c *gin.Context) {
	userID, err := utils.ParseObjectID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Invalid user ID"})
		return
	}

	query := c.Query("query")
	if query == "" {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Query parameter is required"})
		return
	}

	limit := utils.ParseIntParam(c.Query("limit"), 20)
	page := utils.ParseIntParam(c.Query("page"), 1)

	if limit > 250 {
		limit = 250
	}
	if limit < 1 {
		limit = 1
	}

	// Check if user exists
	count, err := h.db.Collection("users").CountDocuments(context.Background(), bson.M{"_id": userID})
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}
	if count == 0 {
		c.JSON(http.StatusNotFound, models.APIError{Error: "User not found"})
		return
	}

	// Build search filter
	filter := bson.M{
		"user":       userID,
		"searchable": true,
		"$text":      bson.M{"$search": query},
	}

	// Count total matching messages
	total, err := h.db.Collection("messages").CountDocuments(context.Background(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}

	// Calculate skip
	skip := (page - 1) * limit

	// Find messages
	opts := options.Find().
		SetLimit(int64(limit)).
		SetSkip(int64(skip)).
		SetSort(bson.D{{Key: "_id", Value: -1}})

	cursor, err := h.db.Collection("messages").Find(context.Background(), filter, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}
	defer cursor.Close(context.Background())

	var messages []models.Message
	if err := cursor.All(context.Background(), &messages); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}

	// Convert to response format
	results := make([]gin.H, len(messages))
	for i, msg := range messages {
		// Get from address
		var from models.EmailAddress
		if len(msg.MimeTree.ParsedHeader.From) > 0 {
			from = msg.MimeTree.ParsedHeader.From[0]
		} else if len(msg.MimeTree.ParsedHeader.Sender) > 0 {
			from = msg.MimeTree.ParsedHeader.Sender[0]
		} else if msg.Meta.From != "" {
			from = models.EmailAddress{Name: "", Address: msg.Meta.From}
		}

		results[i] = gin.H{
			"id":          utils.FormatMessageID(msg.ID, msg.UID),
			"mailbox":     msg.Mailbox.Hex(),
			"thread":      msg.Thread.Hex(),
			"from":        from,
			"subject":     msg.Subject,
			"date":        msg.Date,
			"intro":       msg.Intro,
			"attachments": msg.HasAttach,
			"seen":        msg.GetSeen(),
			"deleted":     msg.GetDeleted(),
			"flagged":     msg.Flagged,
			"draft":       msg.Draft,
		}
	}

	// Build pagination URLs
	baseURL := fmt.Sprintf("/api/users/%s/search", userID.Hex())
	var prevUrl, nextUrl *string

	if page > 1 {
		prev := fmt.Sprintf("%s?query=%s&page=%d&limit=%d", baseURL, query, page-1, limit)
		prevUrl = &prev
	}

	if int64(page*limit) < total {
		next := fmt.Sprintf("%s?query=%s&page=%d&limit=%d", baseURL, query, page+1, limit)
		nextUrl = &next
	}

	response := gin.H{
		"success": true,
		"query":   query,
		"total":   total,
		"page":    page,
		"results": results,
	}

	if prevUrl != nil {
		response["prev"] = *prevUrl
	}
	if nextUrl != nil {
		response["next"] = *nextUrl
	}

	c.JSON(http.StatusOK, response)
}

// moveMessage moves a message from one mailbox to another
func (h *MessageHandler) moveMessage(userID, sourceMailboxID, destMailboxID, messageID primitive.ObjectID, uid int64) error {
	// TODO: Implement proper message moving with UID management
	// For now, just update the mailbox field
	_, err := h.db.Collection("messages").UpdateOne(
		context.Background(),
		bson.M{
			"_id":     messageID,
			"mailbox": sourceMailboxID,
			"uid":     uid,
			"user":    userID,
		},
		bson.M{"$set": bson.M{"mailbox": destMailboxID}},
	)
	return err
}