package indexer

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Mock logger for testing
type MockLogger struct {
	InfoMessages  []string
	DebugMessages []string
	ErrorMessages []string
}

func (l *MockLogger) Info(msg string, fields ...interface{}) {
	l.InfoMessages = append(l.InfoMessages, msg)
}
func (l *MockLogger) Debug(msg string, fields ...interface{}) {
	l.DebugMessages = append(l.DebugMessages, msg)
}
func (l *MockLogger) Error(msg string, fields ...interface{}) {
	l.ErrorMessages = append(l.ErrorMessages, msg)
}

// Mock MongoDB structures for testing
type MockDatabase struct {
	collections  map[string]*MockCollection
	insertedDocs map[string][]interface{}
}

type MockCollection struct {
	name     string
	database *MockDatabase
}

type MockInsertOneResult struct {
	insertedID interface{}
}

func (r *MockInsertOneResult) InsertedID() interface{} {
	return r.insertedID
}

type MockGridFSBucket struct {
	uploadedFiles map[string][]byte
}

func NewMockDatabase() *MockDatabase {
	return &MockDatabase{
		collections:  make(map[string]*MockCollection),
		insertedDocs: make(map[string][]interface{}),
	}
}

func (db *MockDatabase) Collection(name string) *MockCollection {
	if _, exists := db.collections[name]; !exists {
		db.collections[name] = &MockCollection{name: name, database: db}
		db.insertedDocs[name] = make([]interface{}, 0)
	}
	return db.collections[name]
}

func (c *MockCollection) InsertOne(ctx context.Context, document interface{}) (*MockInsertOneResult, error) {
	// Generate a mock ObjectID
	id := primitive.NewObjectID()
	c.database.insertedDocs[c.name] = append(c.database.insertedDocs[c.name], document)
	return &MockInsertOneResult{insertedID: id}, nil
}

func NewMockGridFSBucket() *MockGridFSBucket {
	return &MockGridFSBucket{
		uploadedFiles: make(map[string][]byte),
	}
}

func TestDefaultLogger(t *testing.T) {
	logger := &DefaultLogger{}

	// These should not panic
	logger.Info("Test info message")
	logger.Debug("Test debug message")
	logger.Error("Test error message")
}

func TestNewEmailIndexer(t *testing.T) {
	// Note: NewEmailIndexer with nil database causes a panic in gridfs.NewBucket
	// This is expected behavior - the function requires a valid MongoDB database
	// In production, this should never be called with nil database
	
	// Test logger handling when nil logger is passed
	// We can't test with nil database due to the panic, but we can test the logger logic
	t.Log("NewEmailIndexer requires a valid MongoDB database - testing with nil database would cause panic")
}

func TestIndexerGetContentType(t *testing.T) {
	indexer := &EmailIndexer{logger: &MockLogger{}}

	// Test valid content-type
	node := &MIMENode{
		ParsedHeader: map[string]interface{}{
			"content-type": &ValueParams{
				Type:    "text",
				Subtype: "html",
				Value:   "text/html",
			},
		},
	}
	result := indexer.getContentType(node)
	if result.Type != "text" || result.Subtype != "html" {
		t.Errorf("Expected text/html, got %s/%s", result.Type, result.Subtype)
	}

	// Test missing content-type (should default to text/plain)
	node2 := &MIMENode{
		ParsedHeader: map[string]interface{}{},
	}
	result2 := indexer.getContentType(node2)
	if result2.Type != "text" || result2.Subtype != "plain" {
		t.Errorf("Expected text/plain for missing content-type, got %s/%s", result2.Type, result2.Subtype)
	}
}

func TestGetDisposition(t *testing.T) {
	indexer := &EmailIndexer{logger: &MockLogger{}}

	// Test attachment disposition
	node := &MIMENode{
		ParsedHeader: map[string]interface{}{
			"content-disposition": &ValueParams{
				Value: "ATTACHMENT",
			},
		},
	}
	result := indexer.getDisposition(node)
	if result != "attachment" {
		t.Errorf("Expected 'attachment', got '%s'", result)
	}

	// Test inline disposition as string
	node2 := &MIMENode{
		ParsedHeader: map[string]interface{}{
			"content-disposition": "INLINE",
		},
	}
	result2 := indexer.getDisposition(node2)
	if result2 != "inline" {
		t.Errorf("Expected 'inline', got '%s'", result2)
	}

	// Test no disposition
	node3 := &MIMENode{
		ParsedHeader: map[string]interface{}{},
	}
	result3 := indexer.getDisposition(node3)
	if result3 != "" {
		t.Errorf("Expected empty string for no disposition, got '%s'", result3)
	}
}

func TestGetTransferEncoding(t *testing.T) {
	indexer := &EmailIndexer{logger: &MockLogger{}}

	// Test base64 encoding
	node := &MIMENode{
		ParsedHeader: map[string]interface{}{
			"content-transfer-encoding": "BASE64",
		},
	}
	result := indexer.getTransferEncoding(node)
	if result != "base64" {
		t.Errorf("Expected 'base64', got '%s'", result)
	}

	// Test no encoding (should default to 7bit)
	node2 := &MIMENode{
		ParsedHeader: map[string]interface{}{},
	}
	result2 := indexer.getTransferEncoding(node2)
	if result2 != "7bit" {
		t.Errorf("Expected '7bit' for missing encoding, got '%s'", result2)
	}
}

func TestExtractMessageID(t *testing.T) {
	indexer := &EmailIndexer{logger: &MockLogger{}}

	// Test message-id with brackets
	node := &MIMENode{
		ParsedHeader: map[string]interface{}{
			"message-id": "<msg123@example.com>",
		},
	}
	result := indexer.extractMessageID(node)
	if result != "msg123@example.com" {
		t.Errorf("Expected 'msg123@example.com', got '%s'", result)
	}

	// Test message-id without brackets
	node2 := &MIMENode{
		ParsedHeader: map[string]interface{}{
			"message-id": "msg456@example.com",
		},
	}
	result2 := indexer.extractMessageID(node2)
	if result2 != "msg456@example.com" {
		t.Errorf("Expected 'msg456@example.com', got '%s'", result2)
	}

	// Test no message-id
	node3 := &MIMENode{
		ParsedHeader: map[string]interface{}{},
	}
	result3 := indexer.extractMessageID(node3)
	if result3 != "" {
		t.Errorf("Expected empty string for no message-id, got '%s'", result3)
	}
}

func TestExtractSubject(t *testing.T) {
	indexer := &EmailIndexer{logger: &MockLogger{}}

	// Test simple subject
	node := &MIMENode{
		ParsedHeader: map[string]interface{}{
			"subject": "Test Email Subject",
		},
	}
	result := indexer.extractSubject(node)
	if result != "Test Email Subject" {
		t.Errorf("Expected 'Test Email Subject', got '%s'", result)
	}

	// Test no subject
	node2 := &MIMENode{
		ParsedHeader: map[string]interface{}{},
	}
	result2 := indexer.extractSubject(node2)
	if result2 != "" {
		t.Errorf("Expected empty string for no subject, got '%s'", result2)
	}
}

func TestExtractDate(t *testing.T) {
	indexer := &EmailIndexer{logger: &MockLogger{}}

	// Test RFC1123Z format
	node := &MIMENode{
		ParsedHeader: map[string]interface{}{
			"date": "Mon, 23 Nov 2024 10:30:00 +0000",
		},
	}
	result := indexer.extractDate(node)
	expected := time.Date(2024, 11, 23, 10, 30, 0, 0, time.UTC)
	if !result.Equal(expected) {
		t.Errorf("Expected %v, got %v", expected, result)
	}

	// Test no date header (should return current time)
	node2 := &MIMENode{
		ParsedHeader: map[string]interface{}{},
	}
	result2 := indexer.extractDate(node2)
	if time.Since(result2) > time.Minute {
		t.Error("Expected current time when no date header present")
	}
}

func TestCreateEnvelope(t *testing.T) {
	indexer := &EmailIndexer{logger: &MockLogger{}}

	node := &MIMENode{
		ParsedHeader: map[string]interface{}{
			"date":       "Mon, 23 Nov 2024 10:30:00 +0000",
			"subject":    "Test Subject",
			"message-id": "<test@example.com>",
			"from": []*Address{
				{Name: "John Doe", Address: "john@example.com"},
			},
			"to": []*Address{
				{Address: "jane@example.com"},
			},
		},
	}

	envelope := indexer.CreateEnvelope(node)

	if len(envelope) != 10 {
		t.Fatalf("Expected envelope to have 10 fields, got %d", len(envelope))
	}

	// Check date
	if envelope[0] != "Mon, 23 Nov 2024 10:30:00 +0000" {
		t.Errorf("Expected date in envelope[0], got %v", envelope[0])
	}

	// Check subject
	if envelope[1] != "Test Subject" {
		t.Errorf("Expected subject in envelope[1], got %v", envelope[1])
	}
}

func TestProcessContent_SimpleText(t *testing.T) {
	indexer := &EmailIndexer{logger: &MockLogger{}}

	// Create a simple text email node
	node := &MIMENode{
		ParsedHeader: map[string]interface{}{
			"content-type": &ValueParams{
				Type:    "text",
				Subtype: "plain",
				Value:   "text/plain",
				Params:  map[string]string{"charset": "utf-8"},
			},
		},
		Body: []byte("Hello World!\nThis is a test email."),
		Size: 35,
	}

	ctx := context.Background()
	result, err := indexer.ProcessContent(ctx, "test-msg", node)

	if err != nil {
		t.Fatalf("ProcessContent failed: %v", err)
	}

	if result == nil {
		t.Fatal("ProcessContent returned nil result")
	}

	// Should have extracted text content
	if !strings.Contains(result.Text, "Hello World!") {
		t.Errorf("Expected text content to contain 'Hello World!', got: %s", result.Text)
	}

	// Should have HTML version too (converted from text)
	if len(result.HTML) == 0 {
		t.Error("Expected HTML content to be generated from text")
	} else if !strings.Contains(result.HTML[0], "Hello World!") {
		t.Errorf("Expected HTML content to contain 'Hello World!', got: %s", result.HTML[0])
	}

	// Should have no attachments for simple text
	if len(result.Attachments) > 0 {
		t.Errorf("Expected no attachments for simple text, got %d", len(result.Attachments))
	}
}

func TestHtmlToText(t *testing.T) {
	indexer := &EmailIndexer{logger: &MockLogger{}}

	// Test simple HTML
	html := "<html><body><h1>Title</h1><p>Paragraph</p></body></html>"
	result := indexer.htmlToText(html)
	expected := "TitleParagraph"
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}

	// Test plain text
	text := "Just plain text"
	result2 := indexer.htmlToText(text)
	if result2 != text {
		t.Errorf("Expected '%s', got '%s'", text, result2)
	}

	// Test empty string
	result3 := indexer.htmlToText("")
	if result3 != "" {
		t.Errorf("Expected empty string, got '%s'", result3)
	}
}

func TestTextToHTML(t *testing.T) {
	indexer := &EmailIndexer{logger: &MockLogger{}}

	// Test simple text
	text := "Hello World"
	result := indexer.textToHTML(text)
	expected := "<pre>Hello World</pre>"
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}

	// Test text with HTML characters
	text2 := "Test <tag> & \"quotes\""
	result2 := indexer.textToHTML(text2)
	expected2 := "<pre>Test &lt;tag&gt; &amp; &quot;quotes&quot;</pre>"
	if result2 != expected2 {
		t.Errorf("Expected '%s', got '%s'", expected2, result2)
	}
}

func TestDecodeContent(t *testing.T) {
	indexer := &EmailIndexer{logger: &MockLogger{}}

	// Test 7bit encoding
	data := []byte("Hello World!")
	result, err := indexer.decodeContent(data, "7bit", "utf-8")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result != "Hello World!" {
		t.Errorf("Expected 'Hello World!', got '%s'", result)
	}

	// Test base64 encoding
	data2 := []byte("SGVsbG8gV29ybGQh") // "Hello World!" in base64
	result2, err2 := indexer.decodeContent(data2, "base64", "utf-8")
	if err2 != nil {
		t.Fatalf("Unexpected error: %v", err2)
	}
	if result2 != "Hello World!" {
		t.Errorf("Expected 'Hello World!' from base64, got '%s'", result2)
	}

	// Test invalid base64 (should return error)
	data3 := []byte("InvalidBase64!!!")
	_, err3 := indexer.decodeContent(data3, "base64", "utf-8")
	if err3 == nil {
		t.Error("Expected error for invalid base64 data")
	}
}

// Benchmark tests
func BenchmarkGetContentType(b *testing.B) {
	indexer := &EmailIndexer{logger: &MockLogger{}}
	node := &MIMENode{
		ParsedHeader: map[string]interface{}{
			"content-type": &ValueParams{
				Type:    "text",
				Subtype: "html",
				Value:   "text/html",
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		indexer.getContentType(node)
	}
}

func BenchmarkHtmlToText(b *testing.B) {
	indexer := &EmailIndexer{logger: &MockLogger{}}
	html := "<html><body><h1>Title</h1><p>This is a paragraph with <strong>bold</strong> text.</p></body></html>"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		indexer.htmlToText(html)
	}
}

// Integration tests for IndexEmail function
func TestIndexEmail_SimpleTextEmail(t *testing.T) {
	// Create indexer with mock logger
	mockLogger := &MockLogger{}

	// Create a real indexer but we'll mock the database operations
	indexer := &EmailIndexer{
		logger: mockLogger,
	}

	// Simple RFC822 email
	rfc822Data := []byte(`From: sender@example.com
To: recipient@example.com
Subject: Test Email
Date: Mon, 23 Nov 2024 10:30:00 +0000
Message-ID: <test123@example.com>
Content-Type: text/plain; charset=utf-8

Hello World!
This is a test email.`)

	ctx := context.Background()

	// Since we can't easily mock MongoDB operations without major refactoring,
	// let's test what we can - the parsing and processing logic
	// We'll test the ProcessContent method which is called by IndexEmail

	// Parse the email first to get MIMENode
	mimeTree, err := ParseMIME(rfc822Data)
	if err != nil {
		t.Fatalf("Failed to parse email: %v", err)
	}

	// Test ProcessContent which is the core of IndexEmail
	result, err := indexer.ProcessContent(ctx, "test123@example.com", mimeTree)
	if err != nil {
		t.Fatalf("ProcessContent failed: %v", err)
	}

	// Verify the processed content
	if result == nil {
		t.Fatal("ProcessContent returned nil result")
	}

	// Check text content
	if !strings.Contains(result.Text, "Hello World!") {
		t.Errorf("Expected text to contain 'Hello World!', got: %s", result.Text)
	}
	if !strings.Contains(result.Text, "This is a test email.") {
		t.Errorf("Expected text to contain 'This is a test email.', got: %s", result.Text)
	}

	// Check HTML content (should be generated from text)
	if len(result.HTML) == 0 {
		t.Error("Expected HTML content to be generated")
	} else {
		if !strings.Contains(result.HTML[0], "Hello World!") {
			t.Errorf("Expected HTML to contain 'Hello World!', got: %s", result.HTML[0])
		}
		// Should be wrapped in <pre> tags
		if !strings.Contains(result.HTML[0], "<pre>") {
			t.Error("Expected HTML to be wrapped in <pre> tags")
		}
	}

	// Should have no attachments for simple text
	if len(result.Attachments) > 0 {
		t.Errorf("Expected no attachments, got %d", len(result.Attachments))
	}
}

func TestIndexEmail_HTMLEmail(t *testing.T) {
	mockLogger := &MockLogger{}
	indexer := &EmailIndexer{
		logger: mockLogger,
	}

	// HTML email
	rfc822Data := []byte(`From: sender@example.com
To: recipient@example.com
Subject: HTML Test Email
Date: Mon, 23 Nov 2024 10:30:00 +0000
Message-ID: <html123@example.com>
Content-Type: text/html; charset=utf-8

<html>
<body>
<h1>Welcome</h1>
<p>This is an <strong>HTML</strong> email with <em>formatting</em>.</p>
<ul>
<li>Item 1</li>
<li>Item 2</li>
</ul>
</body>
</html>`)

	ctx := context.Background()

	// Parse the email
	mimeTree, err := ParseMIME(rfc822Data)
	if err != nil {
		t.Fatalf("Failed to parse HTML email: %v", err)
	}

	// Test ProcessContent
	result, err := indexer.ProcessContent(ctx, "html123@example.com", mimeTree)
	if err != nil {
		t.Fatalf("ProcessContent failed for HTML email: %v", err)
	}

	// Verify the processed content
	if result == nil {
		t.Fatal("ProcessContent returned nil result for HTML email")
	}

	// Check HTML content
	if len(result.HTML) == 0 {
		t.Error("Expected HTML content to be present")
	} else {
		htmlContent := result.HTML[0]
		if !strings.Contains(htmlContent, "<h1>Welcome</h1>") {
			t.Errorf("Expected HTML to contain header, got: %s", htmlContent)
		}
		if !strings.Contains(htmlContent, "<strong>HTML</strong>") {
			t.Errorf("Expected HTML to contain bold text, got: %s", htmlContent)
		}
	}

	// Check that text version was extracted
	if !strings.Contains(result.Text, "Welcome") {
		t.Errorf("Expected text to contain 'Welcome', got: %s", result.Text)
	}
	if !strings.Contains(result.Text, "HTML") {
		t.Errorf("Expected text to contain 'HTML', got: %s", result.Text)
	}
}

func TestIndexEmail_MultipartEmail(t *testing.T) {
	mockLogger := &MockLogger{}
	indexer := &EmailIndexer{
		logger: mockLogger,
	}

	// Multipart email with text and HTML versions
	rfc822Data := []byte(`From: sender@example.com
To: recipient@example.com
Subject: Multipart Test Email
Date: Mon, 23 Nov 2024 10:30:00 +0000
Message-ID: <multi123@example.com>
Content-Type: multipart/alternative; boundary="boundary123"

--boundary123
Content-Type: text/plain; charset=utf-8

This is the plain text version.
Simple and clean.

--boundary123
Content-Type: text/html; charset=utf-8

<html>
<body>
<h1>HTML Version</h1>
<p>This is the <strong>HTML</strong> version with formatting.</p>
</body>
</html>

--boundary123--`)

	ctx := context.Background()

	// Parse the multipart email
	mimeTree, err := ParseMIME(rfc822Data)
	if err != nil {
		t.Fatalf("Failed to parse multipart email: %v", err)
	}

	// Test ProcessContent
	result, err := indexer.ProcessContent(ctx, "multi123@example.com", mimeTree)
	if err != nil {
		t.Fatalf("ProcessContent failed for multipart email: %v", err)
	}

	// Verify the processed content
	if result == nil {
		t.Fatal("ProcessContent returned nil result for multipart email")
	}

	// Should have both text and HTML content
	if !strings.Contains(result.Text, "plain text version") {
		t.Errorf("Expected text content, got: %s", result.Text)
	}

	if len(result.HTML) == 0 {
		t.Error("Expected HTML content to be present in multipart email")
	} else {
		htmlContent := result.HTML[0]
		if !strings.Contains(htmlContent, "<h1>HTML Version</h1>") {
			t.Errorf("Expected HTML content, got: %s", htmlContent)
		}
		if !strings.Contains(htmlContent, "<strong>HTML</strong>") {
			t.Errorf("Expected HTML formatting, got: %s", htmlContent)
		}
	}
}

func TestIndexEmail_EmailWithAttachment(t *testing.T) {
	// Skip this test as it requires GridFS operations which need a real MongoDB connection
	t.Skip("Test requires MongoDB GridFS operations - skipping in unit tests")
	
	mockLogger := &MockLogger{}
	indexer := &EmailIndexer{
		logger: mockLogger,
	}

	// Email with attachment (base64 encoded)
	rfc822Data := []byte(`From: sender@example.com
To: recipient@example.com
Subject: Email with Attachment
Date: Mon, 23 Nov 2024 10:30:00 +0000
Message-ID: <attach123@example.com>
Content-Type: multipart/mixed; boundary="mixedboundary"

--mixedboundary
Content-Type: text/plain; charset=utf-8

This email has an attachment.

--mixedboundary
Content-Type: application/pdf
Content-Disposition: attachment; filename="document.pdf"
Content-Transfer-Encoding: base64

JVBERi0xLjQKMSAwIG9iago8PAovVHlwZSAvQ2F0YWxvZwo+PgplbmRvYmoKMiAwIG9iago8PAovVHlwZSAvUGFnZXMKL0tpZHMgWzMgMCBSXQovQ291bnQgMQo+PgplbmRvYmoK

--mixedboundary--`)

	ctx := context.Background()

	// Parse the email with attachment
	mimeTree, err := ParseMIME(rfc822Data)
	if err != nil {
		t.Fatalf("Failed to parse email with attachment: %v", err)
	}

	// Test ProcessContent
	result, err := indexer.ProcessContent(ctx, "attach123@example.com", mimeTree)
	if err != nil {
		t.Fatalf("ProcessContent failed for email with attachment: %v", err)
	}

	// Verify the processed content
	if result == nil {
		t.Fatal("ProcessContent returned nil result for email with attachment")
	}

	// Should have text content
	if !strings.Contains(result.Text, "has an attachment") {
		t.Errorf("Expected text content, got: %s", result.Text)
	}

	// Should have detected attachment
	if len(result.Attachments) == 0 {
		t.Error("Expected attachment to be detected")
	} else {
		attachment := result.Attachments[0]
		if attachment.FileName != "document.pdf" {
			t.Errorf("Expected filename 'document.pdf', got: %s", attachment.FileName)
		}
		if attachment.ContentType != "application/pdf" {
			t.Errorf("Expected content type 'application/pdf', got: %s", attachment.ContentType)
		}
		if attachment.SizeKB == 0 {
			t.Error("Expected attachment size > 0")
		}
	}
}

func TestIndexEmail_EmailHeaderExtraction(t *testing.T) {
	mockLogger := &MockLogger{}
	indexer := &EmailIndexer{
		logger: mockLogger,
	}

	// Email with comprehensive headers
	rfc822Data := []byte(`From: "John Doe" <john.doe@example.com>
To: "Jane Smith" <jane@example.com>, bob@example.com
Cc: charlie@example.com
Bcc: secret@example.com
Subject: =?UTF-8?Q?Test_Subject_with_Encoding?=
Date: Mon, 23 Nov 2024 15:45:30 +0200
Message-ID: <headers123@example.com>
In-Reply-To: <original@example.com>
References: <thread1@example.com> <thread2@example.com>
Content-Type: text/plain; charset=utf-8

Email with comprehensive headers for testing extraction.`)

	// Parse the email
	mimeTree, err := ParseMIME(rfc822Data)
	if err != nil {
		t.Fatalf("Failed to parse email with headers: %v", err)
	}

	// Test extraction methods directly
	messageID := indexer.extractMessageID(mimeTree)
	if messageID != "headers123@example.com" {
		t.Errorf("Expected message ID 'headers123@example.com', got: %s", messageID)
	}

	subject := indexer.extractSubject(mimeTree)
	// Note: The subject might be decoded depending on implementation
	if !strings.Contains(subject, "Test") && !strings.Contains(subject, "Subject") {
		t.Errorf("Expected subject to contain 'Test' or 'Subject', got: %s", subject)
	}

	fromAddrs := indexer.extractAddresses(mimeTree, "from")
	if len(fromAddrs) == 0 {
		t.Error("Expected from addresses to be extracted")
	} else {
		if fromAddrs[0].Address != "john.doe@example.com" {
			t.Errorf("Expected from address 'john.doe@example.com', got: %s", fromAddrs[0].Address)
		}
		if fromAddrs[0].Name != "John Doe" {
			t.Errorf("Expected from name 'John Doe', got: %s", fromAddrs[0].Name)
		}
	}

	toAddrs := indexer.extractAddresses(mimeTree, "to")
	if len(toAddrs) != 2 {
		t.Errorf("Expected 2 to addresses, got: %d", len(toAddrs))
	}

	date := indexer.extractDate(mimeTree)
	expectedDate := time.Date(2024, 11, 23, 13, 45, 30, 0, time.UTC) // Adjusted for +0200 timezone
	if date.UTC().Format(time.RFC3339) != expectedDate.Format(time.RFC3339) {
		t.Errorf("Expected date %v, got %v", expectedDate, date.UTC())
	}
}

func TestIndexEmail_EnvelopeCreation(t *testing.T) {
	mockLogger := &MockLogger{}
	indexer := &EmailIndexer{
		logger: mockLogger,
	}

	// Create a test email node
	node := &MIMENode{
		ParsedHeader: map[string]interface{}{
			"date":       "Mon, 23 Nov 2024 10:30:00 +0000",
			"subject":    "Test IMAP Envelope",
			"message-id": "<envelope123@example.com>",
			"from": []*Address{
				{Name: "John Sender", Address: "john@sender.com"},
			},
			"to": []*Address{
				{Name: "Jane Recipient", Address: "jane@recipient.com"},
			},
			"cc": []*Address{
				{Address: "cc@example.com"},
			},
		},
	}

	envelope := indexer.CreateEnvelope(node)

	// IMAP envelope should have exactly 10 fields
	if len(envelope) != 10 {
		t.Fatalf("Expected envelope to have 10 fields, got %d", len(envelope))
	}

	// Check envelope structure (based on RFC 3501)
	// [0] = date, [1] = subject, [2] = from, [3] = sender, [4] = reply-to,
	// [5] = to, [6] = cc, [7] = bcc, [8] = in-reply-to, [9] = message-id

	if envelope[0] != "Mon, 23 Nov 2024 10:30:00 +0000" {
		t.Errorf("Expected date at position 0, got: %v", envelope[0])
	}

	if envelope[1] != "Test IMAP Envelope" {
		t.Errorf("Expected subject at position 1, got: %v", envelope[1])
	}

	if envelope[9] != "<envelope123@example.com>" {
		t.Errorf("Expected message-id at position 9, got: %v", envelope[9])
	}

	// Check from addresses (should be array of address structures)
	fromField := envelope[2]
	if fromField == nil {
		t.Error("Expected from field to be present")
	}

	// Check to addresses
	toField := envelope[5]
	if toField == nil {
		t.Error("Expected to field to be present")
	}
}

// Benchmark test for IndexEmail processing
func BenchmarkProcessContent(b *testing.B) {
	mockLogger := &MockLogger{}
	indexer := &EmailIndexer{
		logger: mockLogger,
	}

	// Create a test email
	rfc822Data := []byte(`From: sender@example.com
To: recipient@example.com
Subject: Benchmark Test
Date: Mon, 23 Nov 2024 10:30:00 +0000
Message-ID: <bench@example.com>
Content-Type: text/plain; charset=utf-8

This is a benchmark test email with some content.
It has multiple lines to test processing performance.
The content should be processed efficiently.`)

	mimeTree, _ := ParseMIME(rfc822Data)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := indexer.ProcessContent(ctx, "bench@example.com", mimeTree)
		if err != nil {
			b.Fatalf("ProcessContent failed: %v", err)
		}
	}
}
