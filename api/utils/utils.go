package utils

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ValidateEmail validates an email address
func ValidateEmail(email string) bool {
	_, err := mail.ParseAddress(email)
	return err == nil
}

// NormalizeAddress normalizes an email address to lowercase
func NormalizeAddress(address string) string {
	return strings.ToLower(strings.TrimSpace(address))
}

// HashPassword creates a bcrypt hash of the password
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// CheckPassword verifies a password against its hash
func CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// ParseObjectID parses a string to ObjectID
func ParseObjectID(s string) (primitive.ObjectID, error) {
	return primitive.ObjectIDFromHex(s)
}

// ValidateObjectID validates if string is a valid ObjectID
func ValidateObjectID(s string) bool {
	_, err := primitive.ObjectIDFromHex(s)
	return err == nil
}

// ParseMessageID parses message ID in format "objectid:uid"
func ParseMessageID(messageID string) (primitive.ObjectID, int64, error) {
	parts := strings.Split(messageID, ":")
	if len(parts) != 2 {
		return primitive.NilObjectID, 0, fmt.Errorf("invalid message ID format")
	}

	oid, err := primitive.ObjectIDFromHex(parts[0])
	if err != nil {
		return primitive.NilObjectID, 0, err
	}

	uid, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return primitive.NilObjectID, 0, err
	}

	return oid, uid, nil
}

// GenerateUID generates a random unique identifier
func GenerateUID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// ParseIntParam parses integer parameter with default value
func ParseIntParam(value string, defaultValue int) int {
	if value == "" {
		return defaultValue
	}
	if parsed, err := strconv.Atoi(value); err == nil {
		return parsed
	}
	return defaultValue
}

// ParseInt64Param parses int64 parameter with default value
func ParseInt64Param(value string, defaultValue int64) int64 {
	if value == "" {
		return defaultValue
	}
	if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
		return parsed
	}
	return defaultValue
}

// ParseBoolParam parses boolean parameter
func ParseBoolParam(value string) bool {
	switch strings.ToLower(value) {
	case "true", "yes", "y", "1":
		return true
	default:
		return false
	}
}

// ValidateUsername validates username format
func ValidateUsername(username string) bool {
	// Username should be alphanumeric, 3-30 characters
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9]{3,30}$`, username)
	return matched
}

// ValidatePassword validates password requirements
func ValidatePassword(password string) bool {
	// Password should be at least 6 characters, max 256
	return len(password) >= 6 && len(password) <= 256
}

// GetCurrentTime returns current UTC time
func GetCurrentTime() time.Time {
	return time.Now().UTC()
}

// FormatMessageID formats message ID as "objectid:uid"
func FormatMessageID(id primitive.ObjectID, uid int64) string {
	return fmt.Sprintf("%s:%d", id.Hex(), uid)
}

// EscapeRegexSpecialChars escapes regex special characters in a string
func EscapeRegexSpecialChars(s string) string {
	// Escape regex special characters
	specialChars := []string{".", "+", "*", "?", "^", "$", "(", ")", "[", "]", "{", "}", "|", "\\", "-"}
	escaped := s
	for _, char := range specialChars {
		escaped = strings.ReplaceAll(escaped, char, "\\"+char)
	}
	return escaped
}

// Contains checks if slice contains a string
func Contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// RemoveFromSlice removes a string from slice
func RemoveFromSlice(slice []string, item string) []string {
	var result []string
	for _, s := range slice {
		if s != item {
			result = append(result, s)
		}
	}
	return result
}

// AddToSlice adds a string to slice if not present
func AddToSlice(slice []string, item string) []string {
	if !Contains(slice, item) {
		slice = append(slice, item)
	}
	return slice
}