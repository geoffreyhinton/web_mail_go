package indexer

import (
	"strings"
	"testing"
)

func TestCreateBodyStructureSimpleText(t *testing.T) {
	// Create a simple text email MIME node
	node := &MIMENode{
		ParsedHeader: map[string]interface{}{
			"content-type": &ValueParams{
				Type:      "text",
				Subtype:   "plain",
				Value:     "text/plain",
				Params:    map[string]string{"charset": "utf-8"},
				HasParams: true,
			},
			"content-transfer-encoding": "7bit",
		},
		Body:      []byte("Hello World!\nThis is a test message."),
		Size:      35,
		LineCount: 2,
	}

	options := &BodyStructureOptions{
		UpperCaseKeys: true,
	}

	structure := CreateBodyStructure(node, options)

	// Verify structure is an array
	structArray, ok := structure.([]interface{})
	if !ok {
		t.Fatalf("Expected structure to be []interface{}, got %T", structure)
	}

	// Verify basic fields
	if len(structArray) < 8 {
		t.Fatalf("Expected at least 8 elements in structure, got %d", len(structArray))
	}

	// Check type and subtype
	if structArray[0] != "TEXT" {
		t.Errorf("Expected type TEXT, got %v", structArray[0])
	}
	if structArray[1] != "PLAIN" {
		t.Errorf("Expected subtype PLAIN, got %v", structArray[1])
	}

	// Check parameters
	params, ok := structArray[2].([]interface{})
	if !ok {
		t.Fatalf("Expected parameters to be []interface{}, got %T", structArray[2])
	}
	if len(params) != 2 || params[0] != "CHARSET" || params[1] != "utf-8" {
		t.Errorf("Expected charset parameter, got %v", params)
	}

	// Check encoding
	if structArray[5] != "7BIT" {
		t.Errorf("Expected encoding 7BIT, got %v", structArray[5])
	}

	// Check size
	if structArray[6] != 35 {
		t.Errorf("Expected size 35, got %v", structArray[6])
	}

	// Check line count
	if structArray[7] != 2 {
		t.Errorf("Expected line count 2, got %v", structArray[7])
	}
}

func TestCreateBodyStructureMultipart(t *testing.T) {
	// Create child nodes
	textChild := &MIMENode{
		ParsedHeader: map[string]interface{}{
			"content-type": &ValueParams{
				Type:    "text",
				Subtype: "plain",
				Value:   "text/plain",
				Params:  map[string]string{},
			},
		},
		Body:      []byte("Plain text content"),
		Size:      18,
		LineCount: 1,
	}

	htmlChild := &MIMENode{
		ParsedHeader: map[string]interface{}{
			"content-type": &ValueParams{
				Type:    "text",
				Subtype: "html",
				Value:   "text/html",
				Params:  map[string]string{},
			},
		},
		Body:      []byte("<html><body>HTML content</body></html>"),
		Size:      38,
		LineCount: 1,
	}

	// Create multipart node
	multipartNode := &MIMENode{
		ParsedHeader: map[string]interface{}{
			"content-type": &ValueParams{
				Type:      "multipart",
				Subtype:   "alternative",
				Value:     "multipart/alternative",
				Params:    map[string]string{"boundary": "test-boundary"},
				HasParams: true,
			},
		},
		Multipart:  "alternative",
		Boundary:   "test-boundary",
		ChildNodes: []*MIMENode{textChild, htmlChild},
	}

	structure := CreateBodyStructure(multipartNode, &BodyStructureOptions{})

	// Verify structure is an array
	structArray, ok := structure.([]interface{})
	if !ok {
		t.Fatalf("Expected structure to be []interface{}, got %T", structure)
	}

	// Should have at least 3 elements: child1, child2, subtype
	if len(structArray) < 3 {
		t.Fatalf("Expected at least 3 elements, got %d", len(structArray))
	}

	// Check first child (text/plain)
	child1, ok := structArray[0].([]interface{})
	if !ok {
		t.Fatalf("Expected first child to be []interface{}, got %T", structArray[0])
	}
	if child1[0] != "text" || child1[1] != "plain" {
		t.Errorf("Expected first child to be text/plain, got %v/%v", child1[0], child1[1])
	}

	// Check second child (text/html)
	child2, ok := structArray[1].([]interface{})
	if !ok {
		t.Fatalf("Expected second child to be []interface{}, got %T", structArray[1])
	}
	if child2[0] != "text" || child2[1] != "html" {
		t.Errorf("Expected second child to be text/html, got %v/%v", child2[0], child2[1])
	}

	// Check multipart subtype
	if structArray[2] != "alternative" {
		t.Errorf("Expected subtype 'alternative', got %v", structArray[2])
	}
}

func TestCreateBodyStructureAttachment(t *testing.T) {
	node := &MIMENode{
		ParsedHeader: map[string]interface{}{
			"content-type": &ValueParams{
				Type:      "application",
				Subtype:   "pdf",
				Value:     "application/pdf",
				Params:    map[string]string{"name": "document.pdf"},
				HasParams: true,
			},
			"content-disposition": &ValueParams{
				Value:     "attachment",
				Params:    map[string]string{"filename": "document.pdf"},
				HasParams: true,
			},
			"content-transfer-encoding": "base64",
		},
		Body: []byte("JVBERi0xLjMKJcTl8uXrp..."), // Base64 PDF content
		Size: 25,
	}

	structure := CreateBodyStructure(node, &BodyStructureOptions{})

	structArray, ok := structure.([]interface{})
	if !ok {
		t.Fatalf("Expected structure to be []interface{}, got %T", structure)
	}

	// Check type and subtype
	if structArray[0] != "application" {
		t.Errorf("Expected type application, got %v", structArray[0])
	}
	if structArray[1] != "pdf" {
		t.Errorf("Expected subtype pdf, got %v", structArray[1])
	}

	// Check parameters include filename
	params, ok := structArray[2].([]interface{})
	if !ok {
		t.Fatalf("Expected parameters to be []interface{}, got %T", structArray[2])
	}

	// Find name parameter
	nameFound := false
	for i := 0; i < len(params)-1; i += 2 {
		if params[i] == "name" && params[i+1] == "document.pdf" {
			nameFound = true
			break
		}
	}
	if !nameFound {
		t.Errorf("Expected name parameter 'document.pdf' in %v", params)
	}

	// Check encoding
	if structArray[5] != "base64" {
		t.Errorf("Expected encoding base64, got %v", structArray[5])
	}

	// Check extension fields (disposition should be present)
	if len(structArray) >= 9 {
		disposition, ok := structArray[8].([]interface{})
		if !ok {
			t.Errorf("Expected disposition to be []interface{}, got %T", structArray[8])
		} else {
			if disposition[0] != "attachment" {
				t.Errorf("Expected disposition attachment, got %v", disposition[0])
			}
		}
	}
}

func TestCreateBodyStructureRFC822(t *testing.T) {
	// Create embedded message
	embeddedMsg := &MIMENode{
		ParsedHeader: map[string]interface{}{
			"from": []*Address{
				{Name: "John Doe", Address: "john@example.com"},
			},
			"to": []*Address{
				{Address: "jane@example.com"},
			},
			"subject": "Embedded Message",
			"date":    "Mon, 23 Nov 2024 10:00:00 +0000",
			"content-type": &ValueParams{
				Type:    "text",
				Subtype: "plain",
				Value:   "text/plain",
				Params:  map[string]string{},
			},
		},
		Body:      []byte("This is an embedded message."),
		Size:      28,
		LineCount: 1,
	}

	// Create RFC822 container
	rfc822Node := &MIMENode{
		ParsedHeader: map[string]interface{}{
			"content-type": &ValueParams{
				Type:    "message",
				Subtype: "rfc822",
				Value:   "message/rfc822",
				Params:  map[string]string{},
			},
		},
		Message:   embeddedMsg,
		Size:      200,
		LineCount: 10,
	}

	structure := CreateBodyStructure(rfc822Node, &BodyStructureOptions{})

	structArray, ok := structure.([]interface{})
	if !ok {
		t.Fatalf("Expected structure to be []interface{}, got %T", structure)
	}

	// Check type and subtype
	if structArray[0] != "message" {
		t.Errorf("Expected type message, got %v", structArray[0])
	}
	if structArray[1] != "rfc822" {
		t.Errorf("Expected subtype rfc822, got %v", structArray[1])
	}

	// Check envelope (should be at index 7)
	if len(structArray) < 8 {
		t.Fatalf("Expected at least 8 elements for RFC822 structure, got %d", len(structArray))
	}

	envelope, ok := structArray[7].([]interface{})
	if !ok {
		t.Fatalf("Expected envelope to be []interface{}, got %T", structArray[7])
	}

	// Envelope should have 10 fields
	if len(envelope) != 10 {
		t.Errorf("Expected envelope to have 10 fields, got %d", len(envelope))
	}

	// Check subject in envelope
	if envelope[1] != "Embedded Message" {
		t.Errorf("Expected envelope subject 'Embedded Message', got %v", envelope[1])
	}

	// Check embedded body structure (should be at index 8)
	if len(structArray) < 9 {
		t.Fatalf("Expected at least 9 elements for RFC822 structure with body, got %d", len(structArray))
	}

	embeddedStruct, ok := structArray[8].([]interface{})
	if !ok {
		t.Fatalf("Expected embedded structure to be []interface{}, got %T", structArray[8])
	}

	// Embedded structure should be text/plain
	if embeddedStruct[0] != "text" || embeddedStruct[1] != "plain" {
		t.Errorf("Expected embedded structure to be text/plain, got %v/%v", embeddedStruct[0], embeddedStruct[1])
	}
}

func TestBodyStructureOptions(t *testing.T) {
	node := &MIMENode{
		ParsedHeader: map[string]interface{}{
			"content-type": &ValueParams{
				Type:      "text",
				Subtype:   "plain",
				Value:     "text/plain",
				Params:    map[string]string{"charset": "utf-8"},
				HasParams: true,
			},
			"content-language": "en",
			"content-location": "http://example.com/msg.txt",
		},
		Body:      []byte("Test content"),
		Size:      12,
		LineCount: 1,
	}

	// Test with UpperCaseKeys
	options := &BodyStructureOptions{
		UpperCaseKeys: true,
	}
	structure := CreateBodyStructure(node, options)
	structArray := structure.([]interface{})

	if structArray[0] != "TEXT" {
		t.Errorf("Expected uppercase TEXT, got %v", structArray[0])
	}
	if structArray[1] != "PLAIN" {
		t.Errorf("Expected uppercase PLAIN, got %v", structArray[1])
	}

	// Test with Body option (skip extensions)
	options = &BodyStructureOptions{
		Body: true,
	}
	structure = CreateBodyStructure(node, options)
	structArray = structure.([]interface{})

	// Should have fewer fields when Body=true (no extensions)
	if len(structArray) > 8 {
		t.Errorf("Expected 8 or fewer fields with Body=true, got %d", len(structArray))
	}

	// Test with SkipContentLocation
	options = &BodyStructureOptions{
		SkipContentLocation: true,
	}
	structure = CreateBodyStructure(node, options)
	structArray = structure.([]interface{})

	// Extension fields should be present but location should be omitted
	// This is harder to test without detailed field inspection
}

func TestSerializeBodyStructure(t *testing.T) {
	testCases := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{
			name:     "nil value",
			input:    nil,
			expected: "NIL",
		},
		{
			name:     "string value",
			input:    "test",
			expected: `"test"`,
		},
		{
			name:     "string with quotes",
			input:    `text with "quotes"`,
			expected: `"text with \"quotes\""`,
		},
		{
			name:     "integer value",
			input:    42,
			expected: "42",
		},
		{
			name:     "empty array",
			input:    []interface{}{},
			expected: "NIL",
		},
		{
			name:     "simple array",
			input:    []interface{}{"TEXT", "PLAIN"},
			expected: `("TEXT" "PLAIN")`,
		},
		{
			name:     "nested array",
			input:    []interface{}{"TEXT", "PLAIN", []interface{}{"CHARSET", "utf-8"}},
			expected: `("TEXT" "PLAIN" ("CHARSET" "utf-8"))`,
		},
		{
			name:     "mixed types",
			input:    []interface{}{"TEXT", 123, nil, []interface{}{"param"}},
			expected: `("TEXT" 123 NIL ("param"))`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := SerializeBodyStructure(tc.input)
			if result != tc.expected {
				t.Errorf("Expected %s, got %s", tc.expected, result)
			}
		})
	}
}

func TestGetContentType(t *testing.T) {
	bs := &BodyStructure{}

	// Test with valid content-type
	node := &MIMENode{
		ParsedHeader: map[string]interface{}{
			"content-type": &ValueParams{
				Type:    "text",
				Subtype: "html",
				Value:   "text/html",
			},
		},
	}

	ct := bs.getContentType(node)
	if ct.Type != "text" || ct.Subtype != "html" {
		t.Errorf("Expected text/html, got %s/%s", ct.Type, ct.Subtype)
	}

	// Test with missing content-type (should default)
	node = &MIMENode{
		ParsedHeader: map[string]interface{}{},
	}

	ct = bs.getContentType(node)
	if ct.Type != "text" || ct.Subtype != "plain" {
		t.Errorf("Expected default text/plain, got %s/%s", ct.Type, ct.Subtype)
	}

	// Test with invalid content-type format
	node = &MIMENode{
		ParsedHeader: map[string]interface{}{
			"content-type": "invalid-format",
		},
	}

	ct = bs.getContentType(node)
	if ct.Type != "text" || ct.Subtype != "plain" {
		t.Errorf("Expected fallback text/plain, got %s/%s", ct.Type, ct.Subtype)
	}
}

func TestFormatAddresses(t *testing.T) {
	bs := &BodyStructure{}

	// Test with nil addresses
	result := bs.formatAddresses(nil)
	if result != nil {
		t.Errorf("Expected nil for nil addresses, got %v", result)
	}

	// Test with valid addresses
	addresses := []*Address{
		{Name: "John Doe", Address: "john@example.com"},
		{Address: "jane@test.org"},
	}

	result = bs.formatAddresses(addresses)
	resultArray, ok := result.([]interface{})
	if !ok {
		t.Fatalf("Expected []interface{}, got %T", result)
	}

	if len(resultArray) != 2 {
		t.Fatalf("Expected 2 addresses, got %d", len(resultArray))
	}

	// Check first address
	addr1, ok := resultArray[0].([]interface{})
	if !ok {
		t.Fatalf("Expected address to be []interface{}, got %T", resultArray[0])
	}

	if len(addr1) != 4 {
		t.Errorf("Expected 4 fields in address, got %d", len(addr1))
	}

	if addr1[0] != "John Doe" {
		t.Errorf("Expected name 'John Doe', got %v", addr1[0])
	}
	if addr1[2] != "john" {
		t.Errorf("Expected mailbox 'john', got %v", addr1[2])
	}
	if addr1[3] != "example.com" {
		t.Errorf("Expected domain 'example.com', got %v", addr1[3])
	}

	// Check second address (no name)
	addr2, ok := resultArray[1].([]interface{})
	if !ok {
		t.Fatalf("Expected address to be []interface{}, got %T", resultArray[1])
	}

	if addr2[0] != "" {
		t.Errorf("Expected empty name, got %v", addr2[0])
	}
	if addr2[2] != "jane" {
		t.Errorf("Expected mailbox 'jane', got %v", addr2[2])
	}
	if addr2[3] != "test.org" {
		t.Errorf("Expected domain 'test.org', got %v", addr2[3])
	}
}

func TestComplexMultipartStructure(t *testing.T) {
	// Create a complex nested structure: multipart/mixed containing multipart/alternative

	// Inner alternative parts
	textPart := &MIMENode{
		ParsedHeader: map[string]interface{}{
			"content-type": &ValueParams{Type: "text", Subtype: "plain", Value: "text/plain", Params: map[string]string{}},
		},
		Body: []byte("Plain text"), Size: 10, LineCount: 1,
	}

	htmlPart := &MIMENode{
		ParsedHeader: map[string]interface{}{
			"content-type": &ValueParams{Type: "text", Subtype: "html", Value: "text/html", Params: map[string]string{}},
		},
		Body: []byte("<html>HTML</html>"), Size: 18, LineCount: 1,
	}

	// Alternative container
	altPart := &MIMENode{
		ParsedHeader: map[string]interface{}{
			"content-type": &ValueParams{Type: "multipart", Subtype: "alternative", Value: "multipart/alternative", Params: map[string]string{"boundary": "alt"}},
		},
		Multipart: "alternative", Boundary: "alt", ChildNodes: []*MIMENode{textPart, htmlPart},
	}

	// Attachment
	attachPart := &MIMENode{
		ParsedHeader: map[string]interface{}{
			"content-type": &ValueParams{Type: "application", Subtype: "pdf", Value: "application/pdf", Params: map[string]string{"name": "doc.pdf"}},
		},
		Body: []byte("PDF content"), Size: 11,
	}

	// Root mixed container
	rootPart := &MIMENode{
		ParsedHeader: map[string]interface{}{
			"content-type": &ValueParams{Type: "multipart", Subtype: "mixed", Value: "multipart/mixed", Params: map[string]string{"boundary": "mixed"}},
		},
		Multipart: "mixed", Boundary: "mixed", ChildNodes: []*MIMENode{altPart, attachPart},
	}

	structure := CreateBodyStructure(rootPart, &BodyStructureOptions{})
	structArray := structure.([]interface{})

	// Should have: [alternativePart, attachmentPart, "mixed", params, extensions...]
	if len(structArray) < 3 {
		t.Fatalf("Expected at least 3 elements in mixed structure, got %d", len(structArray))
	}

	// Check first part (alternative)
	altStruct, ok := structArray[0].([]interface{})
	if !ok {
		t.Fatalf("Expected alternative part to be []interface{}, got %T", structArray[0])
	}

	// Alternative should have: [textPart, htmlPart, "alternative", params...]
	if len(altStruct) < 3 {
		t.Fatalf("Expected at least 3 elements in alternative structure, got %d", len(altStruct))
	}

	if altStruct[2] != "alternative" {
		t.Errorf("Expected alternative subtype, got %v", altStruct[2])
	}

	// Check attachment part
	attachStruct, ok := structArray[1].([]interface{})
	if !ok {
		t.Fatalf("Expected attachment part to be []interface{}, got %T", structArray[1])
	}

	if attachStruct[0] != "application" || attachStruct[1] != "pdf" {
		t.Errorf("Expected attachment to be application/pdf, got %v/%v", attachStruct[0], attachStruct[1])
	}

	// Check root subtype
	if structArray[2] != "mixed" {
		t.Errorf("Expected mixed subtype, got %v", structArray[2])
	}
}

func TestBodyStructureIntegration(t *testing.T) {
	// Test full integration from email parsing to BODYSTRUCTURE generation
	email := `From: sender@example.com
To: recipient@example.com
Subject: Integration Test
Content-Type: multipart/mixed; boundary="test123"

--test123
Content-Type: text/plain; charset=utf-8

Hello World!

--test123
Content-Type: image/jpeg; name="photo.jpg"
Content-Disposition: attachment; filename="photo.jpg"

[binary image data]

--test123--`

	// Parse the email first
	tree, err := ParseMIME([]byte(email))
	if err != nil {
		t.Fatalf("Failed to parse email: %v", err)
	}

	// Generate BODYSTRUCTURE
	structure := CreateBodyStructure(tree, &BodyStructureOptions{UpperCaseKeys: true})
	structArray := structure.([]interface{})

	// Verify overall structure
	if len(structArray) < 3 {
		t.Fatalf("Expected multipart structure with at least 3 elements, got %d", len(structArray))
	}

	// Check text part
	textPart, ok := structArray[0].([]interface{})
	if !ok {
		t.Fatalf("Expected text part to be []interface{}, got %T", structArray[0])
	}
	if textPart[0] != "TEXT" || textPart[1] != "PLAIN" {
		t.Errorf("Expected TEXT/PLAIN, got %v/%v", textPart[0], textPart[1])
	}

	// Check image part
	imagePart, ok := structArray[1].([]interface{})
	if !ok {
		t.Fatalf("Expected image part to be []interface{}, got %T", structArray[1])
	}
	if imagePart[0] != "IMAGE" || imagePart[1] != "JPEG" {
		t.Errorf("Expected IMAGE/JPEG, got %v/%v", imagePart[0], imagePart[1])
	}

	// Check multipart subtype
	if structArray[2] != "MIXED" {
		t.Errorf("Expected MIXED subtype, got %v", structArray[2])
	}

	// Test serialization
	serialized := SerializeBodyStructure(structure)

	// Should be a valid IMAP BODYSTRUCTURE response
	if !strings.HasPrefix(serialized, "(") || !strings.HasSuffix(serialized, ")") {
		t.Errorf("Serialized structure should be parenthesized, got: %s", serialized)
	}

	// Should contain expected content types
	if !strings.Contains(serialized, `"TEXT"`) || !strings.Contains(serialized, `"PLAIN"`) {
		t.Errorf("Serialized structure should contain TEXT/PLAIN, got: %s", serialized)
	}
	if !strings.Contains(serialized, `"IMAGE"`) || !strings.Contains(serialized, `"JPEG"`) {
		t.Errorf("Serialized structure should contain IMAGE/JPEG, got: %s", serialized)
	}
	if !strings.Contains(serialized, `"MIXED"`) {
		t.Errorf("Serialized structure should contain MIXED, got: %s", serialized)
	}
}

// Benchmark tests for performance validation
func BenchmarkCreateBodyStructureSimple(b *testing.B) {
	node := &MIMENode{
		ParsedHeader: map[string]interface{}{
			"content-type": &ValueParams{
				Type: "text", Subtype: "plain", Value: "text/plain",
				Params: map[string]string{"charset": "utf-8"},
			},
		},
		Body: []byte("Test content"), Size: 12, LineCount: 1,
	}

	options := &BodyStructureOptions{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CreateBodyStructure(node, options)
	}
}

func BenchmarkCreateBodyStructureMultipart(b *testing.B) {
	textChild := &MIMENode{
		ParsedHeader: map[string]interface{}{
			"content-type": &ValueParams{Type: "text", Subtype: "plain", Value: "text/plain", Params: map[string]string{}},
		},
		Body: []byte("Plain"), Size: 5, LineCount: 1,
	}

	htmlChild := &MIMENode{
		ParsedHeader: map[string]interface{}{
			"content-type": &ValueParams{Type: "text", Subtype: "html", Value: "text/html", Params: map[string]string{}},
		},
		Body: []byte("<html></html>"), Size: 13, LineCount: 1,
	}

	multipartNode := &MIMENode{
		ParsedHeader: map[string]interface{}{
			"content-type": &ValueParams{Type: "multipart", Subtype: "alternative", Value: "multipart/alternative", Params: map[string]string{}},
		},
		Multipart: "alternative", ChildNodes: []*MIMENode{textChild, htmlChild},
	}

	options := &BodyStructureOptions{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CreateBodyStructure(multipartNode, options)
	}
}

func BenchmarkSerializeBodyStructure(b *testing.B) {
	structure := []interface{}{
		"TEXT", "PLAIN",
		[]interface{}{"CHARSET", "utf-8"},
		nil, nil, "7BIT", 123, 5,
		nil, nil, nil, nil,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		SerializeBodyStructure(structure)
	}
}
