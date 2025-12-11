package handlers

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/geoffreyhinton/mail_go/api/models"
	"github.com/geoffreyhinton/mail_go/api/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type AddressHandler struct {
	db *mongo.Database
}

func NewAddressHandler(db *mongo.Database) *AddressHandler {
	return &AddressHandler{db: db}
}

// GetAddresses retrieves a paginated list of all addresses
func (h *AddressHandler) GetAddresses(c *gin.Context) {
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
		escapedQuery := utils.EscapeRegexSpecialChars(query)
		filter["address"] = bson.M{
			"$regex":   escapedQuery,
			"$options": "i",
		}
	}

	// Count total documents
	total, err := h.db.Collection("addresses").CountDocuments(context.Background(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}

	// Calculate skip
	skip := (page - 1) * limit

	// Find addresses
	opts := options.Find().
		SetLimit(int64(limit)).
		SetSkip(int64(skip)).
		SetSort(bson.D{{Key: "address", Value: 1}})

	cursor, err := h.db.Collection("addresses").Find(context.Background(), filter, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}
	defer cursor.Close(context.Background())

	var addresses []models.Address
	if err := cursor.All(context.Background(), &addresses); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}

	// Convert to response format
	results := make([]gin.H, len(addresses))
	for i, addr := range addresses {
		results[i] = gin.H{
			"id":      addr.ID.Hex(),
			"address": addr.Address,
			"user":    addr.User.Hex(),
		}
	}

	// Build pagination URLs
	var prevUrl, nextUrl *string
	if page > 1 {
		prev := "/api/addresses?page=" + strconv.Itoa(page-1) + "&limit=" + strconv.Itoa(limit)
		if query != "" {
			prev += "&query=" + query
		}
		prevUrl = &prev
	}

	if int64((page)*limit) < total {
		next := "/api/addresses?page=" + strconv.Itoa(page+1) + "&limit=" + strconv.Itoa(limit)
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

// GetUserAddresses retrieves all addresses for a specific user
func (h *AddressHandler) GetUserAddresses(c *gin.Context) {
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

	// Find addresses for the user
	cursor, err := h.db.Collection("addresses").Find(
		context.Background(),
		bson.M{"user": userID},
		options.Find().SetSort(bson.D{{Key: "address", Value: 1}}),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}
	defer cursor.Close(context.Background())

	var addresses []models.Address
	if err := cursor.All(context.Background(), &addresses); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}

	// Convert to response format
	results := make([]gin.H, len(addresses))
	for i, addr := range addresses {
		results[i] = gin.H{
			"id":      addr.ID.Hex(),
			"address": addr.Address,
			"main":    addr.Address == user.Address,
			"created": addr.Created,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"addresses": results,
	})
}

// CreateUserAddress creates a new address for a user
func (h *AddressHandler) CreateUserAddress(c *gin.Context) {
	userID, err := utils.ParseObjectID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Invalid user ID"})
		return
	}

	var req struct {
		Address string `json:"address" binding:"required,email"`
		Main    bool   `json:"main,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{Error: err.Error()})
		return
	}

	address := utils.NormalizeAddress(req.Address)

	// Validate email format
	if !utils.ValidateEmail(address) {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Invalid email address"})
		return
	}

	// Check for + in address
	if len(address) > 0 && address[0] == '+' {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Address cannot contain +"})
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

	// Check if address already exists
	var existingAddr models.Address
	err = h.db.Collection("addresses").FindOne(context.Background(), bson.M{"address": address}).Decode(&existingAddr)
	if err == nil {
		c.JSON(http.StatusConflict, models.APIError{Error: "Email address already exists"})
		return
	} else if err != mongo.ErrNoDocuments {
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}

	// Create address document
	now := utils.GetCurrentTime()
	addressDoc := models.Address{
		User:    userID,
		Address: address,
		Created: now,
	}

	result, err := h.db.Collection("addresses").InsertOne(context.Background(), addressDoc)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}

	insertedID := result.InsertedID.(primitive.ObjectID)

	// Update user's main address if requested or if user has no main address
	if req.Main || user.Address == "" {
		_, err = h.db.Collection("users").UpdateOne(
			context.Background(),
			bson.M{"_id": userID},
			bson.M{"$set": bson.M{
				"address": address,
				"updated": now,
			}},
		)
		if err != nil {
			// Log error but don't fail the request
			// TODO: Add proper logging
		}
	}

	c.JSON(http.StatusCreated, models.APISuccess{
		Success: true,
		ID:      insertedID.Hex(),
	})
}

// GetUserAddress retrieves a specific address for a user
func (h *AddressHandler) GetUserAddress(c *gin.Context) {
	userID, err := utils.ParseObjectID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Invalid user ID"})
		return
	}

	addressID, err := utils.ParseObjectID(c.Param("addressId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Invalid address ID"})
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

	// Find the address
	var address models.Address
	err = h.db.Collection("addresses").FindOne(
		context.Background(),
		bson.M{"_id": addressID, "user": userID},
	).Decode(&address)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, models.APIError{Error: "Address not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"id":      address.ID.Hex(),
		"address": address.Address,
		"main":    address.Address == user.Address,
		"created": address.Created,
	})
}

// UpdateUserAddress updates a user's address (mainly to set as main address)
func (h *AddressHandler) UpdateUserAddress(c *gin.Context) {
	userID, err := utils.ParseObjectID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Invalid user ID"})
		return
	}

	addressID, err := utils.ParseObjectID(c.Param("addressId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Invalid address ID"})
		return
	}

	var req struct {
		Main bool `json:"main" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{Error: err.Error()})
		return
	}

	if !req.Main {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Cannot unset main status"})
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

	// Find the address
	var address models.Address
	err = h.db.Collection("addresses").FindOne(
		context.Background(),
		bson.M{"_id": addressID, "user": userID},
	).Decode(&address)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, models.APIError{Error: "Invalid or unknown address"})
			return
		}
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}

	// Check if this address is already the main address
	if address.Address == user.Address {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Selected address is already the main email address"})
		return
	}

	// Update user's main address
	result, err := h.db.Collection("users").UpdateOne(
		context.Background(),
		bson.M{"_id": userID},
		bson.M{"$set": bson.M{
			"address": address.Address,
			"updated": utils.GetCurrentTime(),
		}},
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, models.APISuccess{
		Success: result.ModifiedCount > 0,
	})
}

// DeleteUserAddress deletes a user's address
func (h *AddressHandler) DeleteUserAddress(c *gin.Context) {
	userID, err := utils.ParseObjectID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Invalid user ID"})
		return
	}

	addressID, err := utils.ParseObjectID(c.Param("addressId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Invalid address ID"})
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

	// Find the address
	var address models.Address
	err = h.db.Collection("addresses").FindOne(
		context.Background(),
		bson.M{"_id": addressID, "user": userID},
	).Decode(&address)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, models.APIError{Error: "Invalid or unknown address"})
			return
		}
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}

	// Check if trying to delete the main address
	if address.Address == user.Address {
		c.JSON(http.StatusBadRequest, models.APIError{Error: "Cannot delete main address. Set a new main address first"})
		return
	}

	// Delete the address
	result, err := h.db.Collection("addresses").DeleteOne(
		context.Background(),
		bson.M{"_id": addressID},
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIError{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, models.APISuccess{
		Success: result.DeletedCount > 0,
	})
}