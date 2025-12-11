package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/geoffreyhinton/mail_go/api/models"
	"github.com/geoffreyhinton/mail_go/api/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type UserHandler struct {
	db *mongo.Database
}

func NewUserHandler(db *mongo.Database) *UserHandler {
	return &UserHandler{db: db}
}

// GetUsers retrieves a paginated list of users
func (h *UserHandler) GetUsers(c *gin.Context) {
	query := c.Query("query")
	limit := utils.ParseIntParam(c.Query("limit"), 20)
	page := utils.ParseIntParam(c.Query("page"), 1)

	if limit > 250 {
		limit = 250
	}
	if limit < 1 {
		limit = 1
	}

	// Build filter
	filter := bson.M{}
	if query != "" {
		filter["username"] = bson.M{
			"$regex":   query,
			"$options": "i",
		}
	}

	// Count total documents
	total, err := h.db.Collection("users").CountDocuments(context.Background(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}

	// Calculate skip
	skip := (page - 1) * limit

	// Find users
	opts := options.Find().
		SetLimit(int64(limit)).
		SetSkip(int64(skip)).
		SetSort(bson.D{{Key: "username", Value: 1}})

	cursor, err := h.db.Collection("users").Find(context.Background(), filter, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}
	defer cursor.Close(context.Background())

	var users []models.User
	if err := cursor.All(context.Background(), &users); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}

	// Convert to response format
	results := make([]gin.H, len(users))
	for i, user := range users {
		results[i] = gin.H{
			"id":       user.ID.Hex(),
			"username": user.Username,
			"address":  user.Address,
			"quota": gin.H{
				"allowed": user.Quota,
				"used":    user.StorageUsed,
			},
			"disabled": user.Disabled,
		}
	}

	// Build pagination URLs
	var prevUrl, nextUrl *string
	if page > 1 {
		prev := "/api/users?page=" + strconv.Itoa(page-1) + "&limit=" + strconv.Itoa(limit)
		if query != "" {
			prev += "&query=" + query
		}
		prevUrl = &prev
	}

	if int64((page)*limit) < total {
		next := "/api/users?page=" + strconv.Itoa(page+1) + "&limit=" + strconv.Itoa(limit)
		if query != "" {
			next += "&query=" + query
		}
		nextUrl = &next
	}

	response := models.PaginatedResponse{
		Success: true,
		Query:   query,
		Total:   total,
		Page:    page,
		Results: results,
	}

	if prevUrl != nil {
		response.Prev = *prevUrl
	}
	if nextUrl != nil {
		response.Next = *nextUrl
	}

	c.JSON(http.StatusOK, response)
}

// CreateUser creates a new user
func (h *UserHandler) CreateUser(c *gin.Context) {
	var req struct {
		Username   string `json:"username" binding:"required,min=3,max=30"`
		Password   string `json:"password" binding:"required,min=6,max=256"`
		Address    string `json:"address"`
		Language   string `json:"language,omitempty"`
		Retention  int64  `json:"retention,omitempty"`
		Quota      int64  `json:"quota,omitempty"`
		Recipients int64  `json:"recipients,omitempty"`
		Forwards   int64  `json:"forwards,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{Error: err.Error()})
		return
	}

	// Validate username format
	if !utils.ValidateUsername(req.Username) {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Invalid username format"})
		return
	}

	// Check if username already exists
	count, err := h.db.Collection("users").CountDocuments(context.Background(), bson.M{"username": req.Username})
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}
	if count > 0 {
		c.JSON(http.StatusConflict, models.APIError{Error: "Username already exists"})
		return
	}

	// Validate email if provided
	var address string
	if req.Address != "" {
		address = utils.NormalizeAddress(req.Address)
		if !utils.ValidateEmail(address) {
			c.JSON(http.StatusBadRequest, models.APIError{Error: "Invalid email address"})
			return
		}
		
		// Check if address already exists
		count, err := h.db.Collection("addresses").CountDocuments(context.Background(), bson.M{"address": address})
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
			return
		}
		if count > 0 {
			c.JSON(http.StatusConflict, models.APIError{Error: "Email address already exists"})
			return
		}
	} else {
		// Generate default email address
		address = req.Username + "@localhost"
	}

	// Hash password
	hashedPassword, err := utils.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{Error: "Failed to hash password"})
		return
	}

	// Create user
	now := utils.GetCurrentTime()
	user := models.User{
		Username:    req.Username,
		Password:    hashedPassword,
		Address:     address,
		Language:    req.Language,
		Retention:   req.Retention,
		Quota:       req.Quota,
		Recipients:  req.Recipients,
		Forwards:    req.Forwards,
		Activated:   true,
		Disabled:    false,
		StorageUsed: 0,
		Created:     now,
		Updated:     now,
	}

	result, err := h.db.Collection("users").InsertOne(context.Background(), user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}

	userID := result.InsertedID.(primitive.ObjectID)

	// Create address entry
	addressDoc := models.Address{
		User:    userID,
		Address: address,
		Main:    true,
		Created: now,
	}
	
	_, err = h.db.Collection("addresses").InsertOne(context.Background(), addressDoc)
	if err != nil {
		// Rollback user creation if address creation fails
		h.db.Collection("users").DeleteOne(context.Background(), bson.M{"_id": userID})
		c.JSON(http.StatusInternalServerError, models.APIError{Error: "Failed to create address"})
		return
	}

	// Create default mailboxes
	err = h.createDefaultMailboxes(userID)
	if err != nil {
		// Log error but don't fail user creation
		// TODO: Add proper logging
	}

	c.JSON(http.StatusCreated, models.APISuccess{
		Success: true,
		ID:      userID.Hex(),
	})
}

// GetUser retrieves a single user by ID
func (h *UserHandler) GetUser(c *gin.Context) {
	userID, err := utils.ParseObjectID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Invalid user ID"})
		return
	}

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

	// TODO: Get Redis counters for rate limiting
	// For now, return static values
	response := models.UserResponse{
		ID:        user.ID,
		Username:  user.Username,
		Address:   user.Address,
		Language:  user.Language,
		Retention: user.Retention,
		Limits: models.UserLimits{
			Quota: models.UserQuota{
				Allowed: user.Quota,
				Used:    user.StorageUsed,
			},
			Recipients: map[string]interface{}{
				"allowed": user.Recipients,
				"used":    0,
				"ttl":     false,
			},
			Forwards: map[string]interface{}{
				"allowed": user.Forwards,
				"used":    0,
				"ttl":     false,
			},
		},
		Activated: user.Activated,
		Disabled:  user.Disabled,
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": response})
}

// UpdateUser updates user information
func (h *UserHandler) UpdateUser(c *gin.Context) {
	userID, err := utils.ParseObjectID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Invalid user ID"})
		return
	}

	var req struct {
		Password   *string `json:"password,omitempty"`
		Language   *string `json:"language,omitempty"`
		Retention  *int64  `json:"retention,omitempty"`
		Quota      *int64  `json:"quota,omitempty"`
		Recipients *int64  `json:"recipients,omitempty"`
		Forwards   *int64  `json:"forwards,omitempty"`
		Disabled   *bool   `json:"disabled,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{Error: err.Error()})
		return
	}

	update := bson.M{"$set": bson.M{"updated": utils.GetCurrentTime()}}
	
	if req.Password != nil {
		if !utils.ValidatePassword(*req.Password) {
			c.JSON(http.StatusBadRequest, models.APIError{Error: "Invalid password"})
			return
		}
		hashedPassword, err := utils.HashPassword(*req.Password)
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.APIError{Error: "Failed to hash password"})
			return
		}
		update["$set"].(bson.M)["password"] = hashedPassword
	}
	
	if req.Language != nil {
		update["$set"].(bson.M)["language"] = *req.Language
	}
	
	if req.Retention != nil {
		update["$set"].(bson.M)["retention"] = *req.Retention
	}
	
	if req.Quota != nil {
		update["$set"].(bson.M)["quota"] = *req.Quota
	}
	
	if req.Recipients != nil {
		update["$set"].(bson.M)["recipients"] = *req.Recipients
	}
	
	if req.Forwards != nil {
		update["$set"].(bson.M)["forwards"] = *req.Forwards
	}
	
	if req.Disabled != nil {
		update["$set"].(bson.M)["disabled"] = *req.Disabled
	}

	result, err := h.db.Collection("users").UpdateOne(
		context.Background(),
		bson.M{"_id": userID},
		update,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}

	if result.MatchedCount == 0 {
		c.JSON(http.StatusNotFound, models.APIError{Error: "User not found"})
		return
	}

	c.JSON(http.StatusOK, models.APISuccess{Success: true})
}

// DeleteUser deletes a user
func (h *UserHandler) DeleteUser(c *gin.Context) {
	userID, err := utils.ParseObjectID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Invalid user ID"})
		return
	}

	// TODO: Implement cascade deletion of user data (addresses, mailboxes, messages)
	result, err := h.db.Collection("users").DeleteOne(context.Background(), bson.M{"_id": userID})
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}

	if result.DeletedCount == 0 {
		c.JSON(http.StatusNotFound, models.APIError{Error: "User not found"})
		return
	}

	c.JSON(http.StatusOK, models.APISuccess{Success: true})
}

// ResetUserQuota recalculates and resets user storage quota
func (h *UserHandler) ResetUserQuota(c *gin.Context) {
	userID, err := utils.ParseObjectID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Invalid user ID"})
		return
	}

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

	// Calculate total storage used by aggregating message sizes
	pipeline := []bson.M{
		{"$match": bson.M{"user": userID}},
		{"$group": bson.M{
			"_id":         "$user",
			"storageUsed": bson.M{"$sum": "$size"},
		}},
	}

	cursor, err := h.db.Collection("messages").Aggregate(context.Background(), pipeline)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}
	defer cursor.Close(context.Background())

	var storageUsed int64 = 0
	if cursor.Next(context.Background()) {
		var result struct {
			StorageUsed int64 `bson:"storageUsed"`
		}
		if err := cursor.Decode(&result); err == nil {
			storageUsed = result.StorageUsed
		}
	}

	// Update user's storage used
	_, err = h.db.Collection("users").UpdateOne(
		context.Background(),
		bson.M{"_id": userID},
		bson.M{"$set": bson.M{
			"storageUsed": storageUsed,
			"updated":     utils.GetCurrentTime(),
		}},
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"storageUsed": storageUsed,
	})
}

// createDefaultMailboxes creates default mailboxes for a new user
func (h *UserHandler) createDefaultMailboxes(userID primitive.ObjectID) error {
	defaultMailboxes := []struct {
		Path       string
		SpecialUse string
	}{
		{"INBOX", "\\Inbox"},
		{"Sent", "\\Sent"},
		{"Drafts", "\\Drafts"},
		{"Trash", "\\Trash"},
		{"Junk", "\\Junk"},
	}

	now := utils.GetCurrentTime()
	
	for _, mb := range defaultMailboxes {
		mailbox := models.Mailbox{
			User:        userID,
			Path:        mb.Path,
			Name:        mb.Path,
			SpecialUse:  mb.SpecialUse,
			Subscribed:  true,
			ModifyIndex: 1,
			UIDNext:     1,
			UIDValidity: time.Now().Unix(),
			Created:     now,
			Updated:     now,
		}
		
		_, err := h.db.Collection("mailboxes").InsertOne(context.Background(), mailbox)
		if err != nil {
			return err
		}
	}
	
	return nil
}