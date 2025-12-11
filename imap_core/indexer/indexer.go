package indexer

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/quotedprintable"
	"regexp"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/gridfs"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// EmailIndexer handles indexing of RFC822 emails with MongoDB storage
type EmailIndexer struct {
	database     *mongo.Database
	gridFSBucket *gridfs.Bucket
	logger       Logger
}

// Logger interface for custom logging
type Logger interface {
	Info(msg string, fields ...interface{})
	Debug(msg string, fields ...interface{})
	Error(msg string, fields ...interface{})
}

// DefaultLogger provides basic logging
type DefaultLogger struct{}

func (l *DefaultLogger) Info(msg string, fields ...interface{}) { log.Printf("[INFO] "+msg, fields...) }
func (l *DefaultLogger) Debug(msg string, fields ...interface{}) {
	log.Printf("[DEBUG] "+msg, fields...)
}
func (l *DefaultLogger) Error(msg string, fields ...interface{}) {
	log.Printf("[ERROR] "+msg, fields...)
}

// EmailDocument represents the MongoDB document structure for emails
type EmailDocument struct {
	ID            primitive.ObjectID `bson:"_id,omitempty"`
	MessageID     string             `bson:"messageId"`
	Subject       string             `bson:"subject"`
	From          []Address          `bson:"from"`
	To            []Address          `bson:"to"`
	Cc            []Address          `bson:"cc"`
	Bcc           []Address          `bson:"bcc"`
	Date          time.Time          `bson:"date"`
	Size          int64              `bson:"size"`
	Headers       map[string]string  `bson:"headers"`
	TextContent   string             `bson:"textContent"`
	HTMLContent   []string           `bson:"htmlContent"`
	Attachments   []AttachmentInfo   `bson:"attachments"`
	MimeTree      *MIMENode          `bson:"mimeTree"`
	Envelope      []interface{}      `bson:"envelope"`
	BodyStructure []interface{}      `bson:"bodyStructure"`
	CreatedAt     time.Time          `bson:"createdAt"`
	UpdatedAt     time.Time          `bson:"updatedAt"`
}

// AttachmentInfo represents attachment metadata
type AttachmentInfo struct {
	ID               primitive.ObjectID `bson:"id"`
	FileName         string             `bson:"fileName"`
	ContentType      string             `bson:"contentType"`
	Disposition      string             `bson:"disposition"`
	TransferEncoding string             `bson:"transferEncoding"`
	Related          bool               `bson:"related"`
	SizeKB           int64              `bson:"sizeKb"`
	ContentID        string             `bson:"contentId,omitempty"`
}

// ProcessedContent represents extracted email content
type ProcessedContent struct {
	Attachments []AttachmentInfo `json:"attachments"`
	Text        string           `json:"text"`
	HTML        []string         `json:"html"`
}

// NewEmailIndexer creates a new email indexer instance
func NewEmailIndexer(database *mongo.Database, logger Logger) *EmailIndexer {
	if logger == nil {
		logger = &DefaultLogger{}
	}

	bucket, err := gridfs.NewBucket(database, options.GridFSBucket().SetName("attachments"))
	if err != nil {
		logger.Error("Failed to create GridFS bucket: %v", err)
		return nil
	}

	return &EmailIndexer{
		database:     database,
		gridFSBucket: bucket,
		logger:       logger,
	}
}

// IndexEmail processes and indexes an RFC822 email message
func (ei *EmailIndexer) IndexEmail(ctx context.Context, rfc822 []byte, messageID string) (*EmailDocument, error) {
	// Parse the MIME tree
	mimeTree, err := ParseMIME(rfc822)
	if err != nil {
		return nil, fmt.Errorf("failed to parse MIME tree: %w", err)
	}

	// Generate envelope and body structure
	envelope := ei.CreateEnvelope(mimeTree)
	bodyStructure := CreateBodyStructure(mimeTree, &BodyStructureOptions{
		UpperCaseKeys: true,
	})

	// Process content and attachments
	processedContent, err := ei.ProcessContent(ctx, messageID, mimeTree)
	if err != nil {
		return nil, fmt.Errorf("failed to process content: %w", err)
	}

	// Create email document
	doc := &EmailDocument{
		MessageID:     ei.extractMessageID(mimeTree),
		Subject:       ei.extractSubject(mimeTree),
		From:          ei.extractAddresses(mimeTree, "from"),
		To:            ei.extractAddresses(mimeTree, "to"),
		Cc:            ei.extractAddresses(mimeTree, "cc"),
		Bcc:           ei.extractAddresses(mimeTree, "bcc"),
		Date:          ei.extractDate(mimeTree),
		Size:          int64(len(rfc822)),
		Headers:       ei.extractHeaders(mimeTree),
		TextContent:   processedContent.Text,
		HTMLContent:   processedContent.HTML,
		Attachments:   processedContent.Attachments,
		MimeTree:      mimeTree,
		Envelope:      envelope,
		BodyStructure: bodyStructure.([]interface{}),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	// Insert into MongoDB
	result, err := ei.database.Collection("emails").InsertOne(ctx, doc)
	if err != nil {
		return nil, fmt.Errorf("failed to insert email document: %w", err)
	}

	doc.ID = result.InsertedID.(primitive.ObjectID)
	ei.logger.Info("Successfully indexed email", "messageId", doc.MessageID, "id", doc.ID.Hex())

	return doc, nil
}

// ProcessContent extracts and processes email content, storing attachments in GridFS
func (ei *EmailIndexer) ProcessContent(ctx context.Context, messageID string, mimeTree *MIMENode) (*ProcessedContent, error) {
	result := &ProcessedContent{
		Attachments: make([]AttachmentInfo, 0),
		HTML:        make([]string, 0),
	}

	var htmlContent []string
	var textContent []string
	cidMap := make(map[string]AttachmentInfo)

	err := ei.walkMimeTree(ctx, mimeTree, messageID, false, false, &htmlContent, &textContent, &result.Attachments, cidMap)
	if err != nil {
		return nil, err
	}

	// Process CID links in HTML content
	for i, html := range htmlContent {
		htmlContent[i] = ei.updateCIDLinks(html, messageID, cidMap)
	}

	// Process CID links in text content
	for i, text := range textContent {
		textContent[i] = ei.updateCIDLinks(text, messageID, cidMap)
	}

	result.HTML = htmlContent
	result.Text = strings.Join(textContent, "\n")

	return result, nil
}

// walkMimeTree recursively processes MIME tree nodes
func (ei *EmailIndexer) walkMimeTree(ctx context.Context, node *MIMENode, messageID string, alternative, related bool, htmlContent, textContent *[]string, attachments *[]AttachmentInfo, cidMap map[string]AttachmentInfo) error {
	if node == nil {
		return nil
	}

	// Process embedded message
	if node.Message != nil {
		return ei.walkMimeTree(ctx, node.Message, messageID, alternative, related, htmlContent, textContent, attachments, cidMap)
	}

	// Get content type information
	contentType := ei.getContentType(node)
	disposition := ei.getDisposition(node)
	transferEncoding := ei.getTransferEncoding(node)

	// Update multipart flags
	if contentType.Type == "multipart" {
		if contentType.Subtype == "alternative" {
			alternative = true
		}
		if contentType.Subtype == "related" {
			related = true
		}
	}

	// Process text content
	isInlineText := (contentType.Type == "text" &&
		(contentType.Subtype == "plain" || contentType.Subtype == "html") &&
		(disposition == "" || disposition == "inline"))

	if isInlineText && len(node.Body) > 0 {
		content, err := ei.decodeContent(node.Body, transferEncoding, ei.getCharset(node))
		if err != nil {
			ei.logger.Error("Failed to decode content: %v", err)
		} else {
			if contentType.Subtype == "html" {
				*htmlContent = append(*htmlContent, content)
				if !alternative {
					// Convert HTML to text for non-alternative parts
					textVersion := ei.htmlToText(content)
					*textContent = append(*textContent, textVersion)
				}
			} else {
				*textContent = append(*textContent, content)
				if !alternative {
					// Convert text to simple HTML for non-alternative parts
					htmlVersion := ei.textToHTML(content)
					*htmlContent = append(*htmlContent, htmlVersion)
				}
			}
		}
	}

	// Process attachments
	isMultipart := contentType.Type == "multipart"
	if !isMultipart && len(node.Body) > 0 && (!isInlineText || node.Size > 300*1024) {
		attachmentID := primitive.NewObjectID()

		fileName := ei.getFileName(node)
		contentID := ei.getContentID(node)

		if fileName == "" {
			// Generate random filename with appropriate extension
			hash := md5.Sum([]byte(fmt.Sprintf("%s-%d", messageID, time.Now().UnixNano())))
			extension := ei.detectExtension(contentType.Value)
			fileName = fmt.Sprintf("%x.%s", hash[:4], extension)
		}

		// Store attachment in GridFS
		err := ei.storeAttachment(ctx, attachmentID, node.Body, fileName, contentType.Value, disposition, transferEncoding, messageID)
		if err != nil {
			return fmt.Errorf("failed to store attachment: %w", err)
		}

		// Create attachment info
		attachmentInfo := AttachmentInfo{
			ID:               attachmentID,
			FileName:         fileName,
			ContentType:      contentType.Value,
			Disposition:      disposition,
			TransferEncoding: transferEncoding,
			Related:          related,
			SizeKB:           int64((node.Size + 1023) / 1024), // Round up to KB
			ContentID:        contentID,
		}

		// Add to CID map if content ID exists
		if contentID != "" {
			cidMap[contentID] = attachmentInfo
		}

		// Add to attachments list if it's a real attachment
		if !isInlineText && !(contentType.Value == "message/rfc822" && (disposition == "" || disposition == "inline")) {
			*attachments = append(*attachments, attachmentInfo)
		}

		// Clear body and set attachment reference
		node.Body = nil
		// In a full implementation, you'd set node.AttachmentId = attachmentID
	}

	// Process child nodes
	for _, child := range node.ChildNodes {
		err := ei.walkMimeTree(ctx, child, messageID, alternative, related, htmlContent, textContent, attachments, cidMap)
		if err != nil {
			return err
		}
	}

	return nil
}

// storeAttachment stores attachment data in GridFS
func (ei *EmailIndexer) storeAttachment(ctx context.Context, attachmentID primitive.ObjectID, data []byte, fileName, contentType, disposition, transferEncoding, messageID string) error {
	uploadStream, err := ei.gridFSBucket.OpenUploadStreamWithID(
		attachmentID,
		fileName,
		options.GridFSUpload().SetMetadata(bson.M{
			"messages":         []string{messageID},
			"fileName":         fileName,
			"contentType":      contentType,
			"disposition":      disposition,
			"transferEncoding": transferEncoding,
		}),
	)
	if err != nil {
		return fmt.Errorf("failed to open upload stream: %w", err)
	}
	defer uploadStream.Close()

	_, err = uploadStream.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write attachment data: %w", err)
	}

	return nil
}

// Helper methods for content extraction
func (ei *EmailIndexer) getContentType(node *MIMENode) *ValueParams {
	if ct, exists := node.ParsedHeader["content-type"]; exists {
		if ctValue, ok := ct.(*ValueParams); ok {
			return ctValue
		}
	}
	return &ValueParams{Type: "text", Subtype: "plain", Value: "text/plain"}
}

func (ei *EmailIndexer) getDisposition(node *MIMENode) string {
	if disp, exists := node.ParsedHeader["content-disposition"]; exists {
		if dispValue, ok := disp.(*ValueParams); ok {
			return strings.ToLower(dispValue.Value)
		}
		if dispStr, ok := disp.(string); ok {
			return strings.ToLower(dispStr)
		}
	}
	return ""
}

func (ei *EmailIndexer) getTransferEncoding(node *MIMENode) string {
	if te, exists := node.ParsedHeader["content-transfer-encoding"]; exists {
		if teStr, ok := te.(string); ok {
			return strings.ToLower(teStr)
		}
	}
	return "7bit"
}

func (ei *EmailIndexer) getCharset(node *MIMENode) string {
	contentType := ei.getContentType(node)
	if charset, exists := contentType.Params["charset"]; exists {
		return charset
	}
	return "utf-8"
}

func (ei *EmailIndexer) getFileName(node *MIMENode) string {
	// Check content-disposition first
	if disp, exists := node.ParsedHeader["content-disposition"]; exists {
		if dispValue, ok := disp.(*ValueParams); ok {
			if filename, hasFilename := dispValue.Params["filename"]; hasFilename {
				return ei.decodeHeaderValue(filename)
			}
		}
	}

	// Check content-type
	contentType := ei.getContentType(node)
	if name, hasName := contentType.Params["name"]; hasName {
		return ei.decodeHeaderValue(name)
	}

	return ""
}

func (ei *EmailIndexer) getContentID(node *MIMENode) string {
	if cid, exists := node.ParsedHeader["content-id"]; exists {
		if cidStr, ok := cid.(string); ok {
			return strings.Trim(cidStr, "<>")
		}
	}
	return ""
}

// decodeContent decodes email content based on transfer encoding and charset
func (ei *EmailIndexer) decodeContent(data []byte, transferEncoding, charset string) (string, error) {
	var decoded []byte
	var err error

	switch transferEncoding {
	case "base64":
		decoded, err = base64.StdEncoding.DecodeString(string(data))
		if err != nil {
			return "", err
		}
	case "quoted-printable":
		reader := quotedprintable.NewReader(bytes.NewReader(data))
		decoded, err = io.ReadAll(reader)
		if err != nil {
			return "", err
		}
	default:
		decoded = data
	}

	// For simplicity, assume UTF-8. In production, you'd want proper charset conversion
	return string(decoded), nil
}

// decodeHeaderValue decodes MIME-encoded header values
func (ei *EmailIndexer) decodeHeaderValue(value string) string {
	// Simple implementation - in production use proper MIME word decoder
	return value
}

// detectExtension detects file extension from MIME type
func (ei *EmailIndexer) detectExtension(mimeType string) string {
	extensions, err := mime.ExtensionsByType(mimeType)
	if err != nil || len(extensions) == 0 {
		return "bin"
	}
	return strings.TrimPrefix(extensions[0], ".")
}

// updateCIDLinks updates CID links in content to attachment references
func (ei *EmailIndexer) updateCIDLinks(content, messageID string, cidMap map[string]AttachmentInfo) string {
	re := regexp.MustCompile(`\bcid:([^\s"']+)`)
	return re.ReplaceAllStringFunc(content, func(match string) string {
		cid := strings.TrimPrefix(match, "cid:")
		if attachment, exists := cidMap[cid]; exists {
			return fmt.Sprintf("attachment:%s/%s", messageID, attachment.ID.Hex())
		}
		return match
	})
}

// htmlToText converts HTML to plain text (simplified implementation)
func (ei *EmailIndexer) htmlToText(html string) string {
	// Remove HTML tags (very basic implementation)
	re := regexp.MustCompile(`<[^>]*>`)
	text := re.ReplaceAllString(html, "")

	// Decode HTML entities (basic implementation)
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&nbsp;", " ")
	text = strings.ReplaceAll(text, "&quot;", "\"")

	return strings.TrimSpace(text)
}

// textToHTML converts plain text to simple HTML
func (ei *EmailIndexer) textToHTML(text string) string {
	// Escape HTML characters
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")
	text = strings.ReplaceAll(text, "\"", "&quot;")

	// Convert line breaks to <br>
	text = strings.ReplaceAll(text, "\n", "<br>\n")

	return fmt.Sprintf("<pre>%s</pre>", text)
}

// Header extraction methods
func (ei *EmailIndexer) extractMessageID(mimeTree *MIMENode) string {
	if msgID, exists := mimeTree.ParsedHeader["message-id"]; exists {
		if msgIDStr, ok := msgID.(string); ok {
			return strings.Trim(msgIDStr, "<>")
		}
	}
	return ""
}

func (ei *EmailIndexer) extractSubject(mimeTree *MIMENode) string {
	if subj, exists := mimeTree.ParsedHeader["subject"]; exists {
		if subjStr, ok := subj.(string); ok {
			return ei.decodeHeaderValue(subjStr)
		}
	}
	return ""
}

func (ei *EmailIndexer) extractAddresses(mimeTree *MIMENode, field string) []Address {
	if addrs, exists := mimeTree.ParsedHeader[field]; exists {
		if addrList, ok := addrs.([]*Address); ok {
			result := make([]Address, len(addrList))
			for i, addr := range addrList {
				result[i] = *addr
			}
			return result
		}
	}
	return nil
}

func (ei *EmailIndexer) extractDate(mimeTree *MIMENode) time.Time {
	if dateStr, exists := mimeTree.ParsedHeader["date"]; exists {
		if dateStrValue, ok := dateStr.(string); ok {
			// Parse RFC2822 date format
			t, err := time.Parse(time.RFC1123Z, dateStrValue)
			if err != nil {
				// Try alternative formats
				formats := []string{
					time.RFC1123,
					"Mon, 2 Jan 2006 15:04:05 -0700",
					"2 Jan 2006 15:04:05 -0700",
				}
				for _, format := range formats {
					if t, err = time.Parse(format, dateStrValue); err == nil {
						break
					}
				}
			}
			if err == nil {
				return t
			}
		}
	}
	return time.Now()
}

func (ei *EmailIndexer) extractHeaders(mimeTree *MIMENode) map[string]string {
	headers := make(map[string]string)
	for key, value := range mimeTree.ParsedHeader {
		if strValue, ok := value.(string); ok {
			headers[key] = strValue
		}
	}
	return headers
}

// CreateEnvelope creates IMAP envelope from MIME tree
func (ei *EmailIndexer) CreateEnvelope(mimeTree *MIMENode) []interface{} {
	envelope := make([]interface{}, 10)

	// Date
	envelope[0] = ei.getHeaderString(mimeTree, "date")

	// Subject
	envelope[1] = ei.extractSubject(mimeTree)

	// From, Sender, Reply-To, To, CC, BCC
	envelope[2] = ei.formatAddressesForEnvelope(mimeTree, "from")
	envelope[3] = ei.formatAddressesForEnvelope(mimeTree, "sender")
	envelope[4] = ei.formatAddressesForEnvelope(mimeTree, "reply-to")
	envelope[5] = ei.formatAddressesForEnvelope(mimeTree, "to")
	envelope[6] = ei.formatAddressesForEnvelope(mimeTree, "cc")
	envelope[7] = ei.formatAddressesForEnvelope(mimeTree, "bcc")

	// In-Reply-To, Message-ID
	envelope[8] = ei.getHeaderString(mimeTree, "in-reply-to")
	envelope[9] = ei.getHeaderString(mimeTree, "message-id")

	return envelope
}

func (ei *EmailIndexer) getHeaderString(mimeTree *MIMENode, header string) interface{} {
	if value, exists := mimeTree.ParsedHeader[header]; exists {
		if strValue, ok := value.(string); ok {
			return strValue
		}
	}
	return nil
}

func (ei *EmailIndexer) formatAddressesForEnvelope(mimeTree *MIMENode, field string) interface{} {
	addrs := ei.extractAddresses(mimeTree, field)
	if len(addrs) == 0 {
		return nil
	}

	result := make([]interface{}, len(addrs))
	for i, addr := range addrs {
		parts := strings.Split(addr.Address, "@")
		var user, domain string
		if len(parts) == 2 {
			user = parts[0]
			domain = parts[1]
		} else {
			user = addr.Address
		}

		result[i] = []interface{}{
			addr.Name, // personal name
			nil,       // SMTP source route (obsolete)
			user,      // mailbox name
			domain,    // domain name
		}
	}

	return result
}
