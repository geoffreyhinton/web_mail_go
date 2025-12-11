package handlers

import (
	"context"
	"net/http"
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

type MailboxHandler struct {
	db *mongo.Database
}

func NewMailboxHandler(db *mongo.Database) *MailboxHandler {
	return &MailboxHandler{db: db}
}

// GetUserMailboxes retrieves all mailboxes for a specific user
func (h *MailboxHandler) GetUserMailboxes(c *gin.Context) {
	userID, err := utils.ParseObjectID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Invalid user ID"})
		return
	}

	counters := utils.ParseBoolParam(c.Query("counters"))

	// Check if user exists
	var user models.User
	err = h.db.Collection("users").FindOne(context.Background(), bson.M{"_id": userID}).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, models.APIError{Error: "User not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}

	// Find mailboxes for the user
	cursor, err := h.db.Collection("mailboxes").Find(
		context.Background(),
		bson.M{"user": userID},
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}
	defer cursor.Close(context.Background())

	var mailboxes []models.Mailbox
	if err := cursor.All(context.Background(), &mailboxes); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}

	// Sort mailboxes (INBOX first, then alphabetically)
	sortedMailboxes := make([]models.Mailbox, 0, len(mailboxes))
	var inbox *models.Mailbox
	
	for i := range mailboxes {
		if mailboxes[i].Path == "INBOX" {
			inbox = &mailboxes[i]
		} else {
			sortedMailboxes = append(sortedMailboxes, mailboxes[i])
		}
	}
	
	if inbox != nil {
		sortedMailboxes = append([]models.Mailbox{*inbox}, sortedMailboxes...)
	}

	// Convert to response format
	results := make([]gin.H, len(sortedMailboxes))
	for i, mb := range sortedMailboxes {
		pathParts := strings.Split(mb.Path, "/")
		name := pathParts[len(pathParts)-1]

		result := gin.H{
			"id":          mb.ID.Hex(),
			"name":        name,
			"path":        mb.Path,
			"specialUse":  mb.SpecialUse,
			"modifyIndex": mb.ModifyIndex,
			"subscribed":  mb.Subscribed,
		}

		if counters {
			// Get message counts
			total, err := h.getMailboxMessageCount(mb.ID, nil)
			if err == nil {
				result["total"] = total
			}
			
			unseen, err := h.getMailboxMessageCount(mb.ID, bson.M{"unseen": true})
			if err == nil {
				result["unseen"] = unseen
			}
		}

		results[i] = result
	}

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"mailboxes": results,
	})
}

// CreateMailbox creates a new mailbox for a user
func (h *MailboxHandler) CreateMailbox(c *gin.Context) {
	userID, err := utils.ParseObjectID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Invalid user ID"})
		return
	}

	var req struct {
		Path      string `json:"path" binding:"required"`
		Retention int64  `json:"retention,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{Error: err.Error()})
		return
	}

	// Validate path (no double slashes or trailing slash)
	path := strings.TrimSpace(req.Path)
	if strings.Contains(path, "//") || strings.HasSuffix(path, "/") {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Invalid mailbox path"})
		return
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

	// Check if mailbox already exists
	existingCount, err := h.db.Collection("mailboxes").CountDocuments(
		context.Background(),
		bson.M{"user": userID, "path": path},
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}
	if existingCount > 0 {
		c.JSON(http.StatusConflict, models.APIError{Error: "Mailbox already exists"})
		return
	}

	// Create mailbox
	now := utils.GetCurrentTime()
	pathParts := strings.Split(path, "/")
	name := pathParts[len(pathParts)-1]
	
	mailbox := models.Mailbox{
		User:        userID,
		Path:        path,
		Name:        name,
		Retention:   req.Retention,
		Subscribed:  true,
		ModifyIndex: 1,
		UIDNext:     1,
		UIDValidity: time.Now().Unix(),
		Created:     now,
		Updated:     now,
	}

	result, err := h.db.Collection("mailboxes").InsertOne(context.Background(), mailbox)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}

	c.JSON(http.StatusCreated, models.APISuccess{
		Success: true,
		ID:      result.InsertedID.(primitive.ObjectID).Hex(),
	})
}

// GetMailbox retrieves a specific mailbox
func (h *MailboxHandler) GetMailbox(c *gin.Context) {
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

	// Find the mailbox
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

	pathParts := strings.Split(mailbox.Path, "/")
	name := pathParts[len(pathParts)-1]

	// Get message counts
	total, err := h.getMailboxMessageCount(mailbox.ID, nil)
	if err != nil {
		total = 0 // Default to 0 on error
	}

	unseen, err := h.getMailboxMessageCount(mailbox.ID, bson.M{"unseen": true})
	if err != nil {
		unseen = 0 // Default to 0 on error
	}

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"id":          mailbox.ID.Hex(),
		"name":        name,
		"path":        mailbox.Path,
		"specialUse":  mailbox.SpecialUse,
		"modifyIndex": mailbox.ModifyIndex,
		"subscribed":  mailbox.Subscribed,
		"total":       total,
		"unseen":      unseen,
	})
}

// UpdateMailbox updates mailbox properties
func (h *MailboxHandler) UpdateMailbox(c *gin.Context) {
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

	var req struct {
		Path       *string `json:"path,omitempty"`
		Retention  *int64  `json:"retention,omitempty"`
		Subscribed *bool   `json:"subscribed,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{Error: err.Error()})
		return
	}

	// Build update document
	update := bson.M{"$set": bson.M{"updated": utils.GetCurrentTime()}}
	hasUpdate := false

	if req.Path != nil {
		path := strings.TrimSpace(*req.Path)
		if strings.Contains(path, "//") || strings.HasSuffix(path, "/") {
			c.JSON(http.StatusBadRequest, models.APIError{Error: "Invalid mailbox path"})
			return
		}
		
		pathParts := strings.Split(path, "/")
		name := pathParts[len(pathParts)-1]
		
		update["$set"].(bson.M)["path"] = path
		update["$set"].(bson.M)["name"] = name
		hasUpdate = true
	}

	if req.Retention != nil {
		update["$set"].(bson.M)["retention"] = *req.Retention
		hasUpdate = true
	}

	if req.Subscribed != nil {
		update["$set"].(bson.M)["subscribed"] = *req.Subscribed
		hasUpdate = true
	}

	if !hasUpdate {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Nothing was changed"})
		return
	}

	// Update the mailbox
	result, err := h.db.Collection("mailboxes").UpdateOne(
		context.Background(),
		bson.M{"_id": mailboxID, "user": userID},
		update,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, models.APIError{Error: "Mailbox not found"})
		return
	}

	c.JSON(http.StatusOK, models.APISuccess{Success: true})
}

// DeleteMailbox deletes a mailbox
func (h *MailboxHandler) DeleteMailbox(c *gin.Context) {
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

	// Find the mailbox first to check if it's INBOX
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

	// Prevent deletion of INBOX
	if mailbox.Path == "INBOX" {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Cannot delete INBOX"})
		return
	}

	// Check if mailbox has messages
	messageCount, err := h.getMailboxMessageCount(mailboxID, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}

	if messageCount > 0 {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Cannot delete mailbox with messages"})
		return
	}

	// Delete the mailbox
	result, err := h.db.Collection("mailboxes").DeleteOne(
		context.Background(),
		bson.M{"_id": mailboxID, "user": userID},
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, models.APISuccess{
		Success: result.DeletedCount > 0,
	})
}

// getMailboxMessageCount counts messages in a mailbox with optional filter
func (h *MailboxHandler) getMailboxMessageCount(mailboxID primitive.ObjectID, extraFilter bson.M) (int64, error) {
	filter := bson.M{"mailbox": mailboxID}
	
	// Merge additional filters if provided
	if extraFilter != nil {
		for k, v := range extraFilter {
			filter[k] = v
		}
	}
	
	return h.db.Collection("messages").CountDocuments(context.Background(), filter)
}