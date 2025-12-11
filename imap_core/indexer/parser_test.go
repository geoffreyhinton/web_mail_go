package indexer

import (
	"strings"
	"testing"
	"time"
)

func TestBasicEmailParsing(t *testing.T) {
	email := `From: sender@example.com
To: recipient@example.com
Subject: Basic Test Email
Date: Mon, 23 Nov 2024 10:30:00 +0000
Content-Type: text/plain; charset=utf-8

Hello World!
This is a basic test email.`

	tree, err := ParseMIME([]byte(email))
	if err != nil {
		t.Fatalf("Failed to parse email: %v", err)
	}

	if tree == nil {
		t.Fatal("Parsed tree is nil")
	}

	// Test headers
	if subject, ok := tree.ParsedHeader["subject"].(string); !ok || subject != "Basic Test Email" {
		t.Errorf("Expected subject 'Basic Test Email', got %v", tree.ParsedHeader["subject"])
	}

	// Test From address
	if from, ok := tree.ParsedHeader["from"].([]*Address); !ok || len(from) == 0 || from[0].Address != "sender@example.com" {
		t.Errorf("Expected from 'sender@example.com', got %v", tree.ParsedHeader["from"])
	}

	// Test To address
	if to, ok := tree.ParsedHeader["to"].([]*Address); !ok || len(to) == 0 || to[0].Address != "recipient@example.com" {
		t.Errorf("Expected to 'recipient@example.com', got %v", tree.ParsedHeader["to"])
	}

	// Test Content-Type
	if ct, ok := tree.ParsedHeader["content-type"].(*ValueParams); !ok || ct.Type != "text" || ct.Subtype != "plain" {
		t.Errorf("Expected content-type text/plain, got %v", tree.ParsedHeader["content-type"])
	}

	// Test body
	bodyStr := string(tree.Body)
	if !strings.Contains(bodyStr, "Hello World!") {
		t.Errorf("Body should contain 'Hello World!', got: %s", bodyStr)
	}

	// Test line count
	if tree.LineCount == 0 {
		t.Error("LineCount should be greater than 0")
	}

	// Test size
	if tree.Size == 0 {
		t.Error("Size should be greater than 0")
	}
}

func TestMultipartEmail(t *testing.T) {
	email := `From: sender@example.com
To: recipient@example.com
Subject: Multipart Test Email
Content-Type: multipart/alternative; boundary="test-boundary"

--test-boundary
Content-Type: text/plain; charset=utf-8

This is the plain text version.

--test-boundary
Content-Type: text/html; charset=utf-8

<html><body><p>This is the HTML version.</p></body></html>

--test-boundary--`

	tree, err := ParseMIME([]byte(email))
	if err != nil {
		t.Fatalf("Failed to parse multipart email: %v", err)
	}

	// Test multipart type
	if tree.Multipart != "alternative" {
		t.Errorf("Expected multipart type 'alternative', got '%s'", tree.Multipart)
	}

	// Test boundary
	if tree.Boundary != "test-boundary" {
		t.Errorf("Expected boundary 'test-boundary', got '%s'", tree.Boundary)
	}

	// Test child nodes
	if len(tree.ChildNodes) != 2 {
		t.Errorf("Expected 2 child nodes, got %d", len(tree.ChildNodes))
	}

	// Test first child (text/plain)
	firstChild := tree.ChildNodes[0]
	if ct, ok := firstChild.ParsedHeader["content-type"].(*ValueParams); !ok || ct.Type != "text" || ct.Subtype != "plain" {
		t.Errorf("First child should be text/plain, got %v", firstChild.ParsedHeader["content-type"])
	}

	plainBody := string(firstChild.Body)
	if !strings.Contains(plainBody, "plain text version") {
		t.Errorf("Plain text body should contain 'plain text version', got: %s", plainBody)
	}

	// Test second child (text/html)
	secondChild := tree.ChildNodes[1]
	if ct, ok := secondChild.ParsedHeader["content-type"].(*ValueParams); !ok || ct.Type != "text" || ct.Subtype != "html" {
		t.Errorf("Second child should be text/html, got %v", secondChild.ParsedHeader["content-type"])
	}

	htmlBody := string(secondChild.Body)
	if !strings.Contains(htmlBody, "HTML version") {
		t.Errorf("HTML body should contain 'HTML version', got: %s", htmlBody)
	}
}

func TestNestedMultipartEmail(t *testing.T) {
	email := `From: sender@example.com
To: recipient@example.com
Subject: Nested Multipart Test
Content-Type: multipart/mixed; boundary="outer-boundary"

--outer-boundary
Content-Type: multipart/alternative; boundary="inner-boundary"

--inner-boundary
Content-Type: text/plain

Plain text content

--inner-boundary
Content-Type: text/html

<html>HTML content</html>

--inner-boundary--

--outer-boundary
Content-Type: application/pdf; name="document.pdf"
Content-Disposition: attachment; filename="document.pdf"

PDF content here

--outer-boundary--`

	tree, err := ParseMIME([]byte(email))
	if err != nil {
		t.Fatalf("Failed to parse nested multipart email: %v", err)
	}

	// Test outer multipart
	if tree.Multipart != "mixed" {
		t.Errorf("Expected outer multipart type 'mixed', got '%s'", tree.Multipart)
	}

	if len(tree.ChildNodes) != 2 {
		t.Errorf("Expected 2 outer child nodes, got %d", len(tree.ChildNodes))
	}

	// Test nested multipart
	nestedNode := tree.ChildNodes[0]
	if nestedNode.Multipart != "alternative" {
		t.Errorf("Expected nested multipart type 'alternative', got '%s'", nestedNode.Multipart)
	}

	if len(nestedNode.ChildNodes) != 2 {
		t.Errorf("Expected 2 nested child nodes, got %d", len(nestedNode.ChildNodes))
	}

	// Test attachment
	attachmentNode := tree.ChildNodes[1]
	if ct, ok := attachmentNode.ParsedHeader["content-type"].(*ValueParams); !ok || ct.Type != "application" || ct.Subtype != "pdf" {
		t.Errorf("Attachment should be application/pdf, got %v", attachmentNode.ParsedHeader["content-type"])
	}

	// Test attachment filename
	if ct, ok := attachmentNode.ParsedHeader["content-type"].(*ValueParams); ok {
		if filename, hasName := ct.Params["name"]; !hasName || filename != "document.pdf" {
			t.Errorf("Expected filename 'document.pdf', got '%s'", filename)
		}
	}
}

func TestAddressParsing(t *testing.T) {
	testCases := []struct {
		name     string
		header   string
		field    string
		expected []Address
	}{
		{
			name:   "Simple address",
			header: "From: john@example.com",
			field:  "from",
			expected: []Address{
				{Address: "john@example.com"},
			},
		},
		{
			name:   "Address with name",
			header: "From: John Doe <john@example.com>",
			field:  "from",
			expected: []Address{
				{Name: "John Doe", Address: "john@example.com"},
			},
		},
		{
			name:   "Multiple addresses",
			header: "To: john@example.com, jane@example.com",
			field:  "to",
			expected: []Address{
				{Address: "john@example.com"},
				{Address: "jane@example.com"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			email := tc.header + "\nSubject: Test\n\nBody"
			tree, err := ParseMIME([]byte(email))
			if err != nil {
				t.Fatalf("Failed to parse email: %v", err)
			}

			addresses, ok := tree.ParsedHeader[tc.field].([]*Address)
			if !ok {
				t.Fatalf("Expected address slice, got %T", tree.ParsedHeader[tc.field])
			}

			if len(addresses) != len(tc.expected) {
				t.Fatalf("Expected %d addresses, got %d", len(tc.expected), len(addresses))
			}

			for i, addr := range addresses {
				if addr.Name != tc.expected[i].Name || addr.Address != tc.expected[i].Address {
					t.Errorf("Address %d: expected %+v, got %+v", i, tc.expected[i], *addr)
				}
			}
		})
	}
}

func TestHeaderFolding(t *testing.T) {
	email := `From: sender@example.com
To: recipient@example.com
Subject: This is a very long subject line
 that continues on the next line
 and even another line
Content-Type: text/html;
 charset=utf-8;
 boundary="test"

Body content`

	tree, err := ParseMIME([]byte(email))
	if err != nil {
		t.Fatalf("Failed to parse email with folded headers: %v", err)
	}

	// Test folded subject
	subject, ok := tree.ParsedHeader["subject"].(string)
	if !ok {
		t.Fatalf("Expected string subject, got %T", tree.ParsedHeader["subject"])
	}

	expectedSubject := "This is a very long subject line that continues on the next line and even another line"
	if subject != expectedSubject {
		t.Errorf("Expected folded subject '%s', got '%s'", expectedSubject, subject)
	}

	// Test folded content-type
	ct, ok := tree.ParsedHeader["content-type"].(*ValueParams)
	if !ok {
		t.Fatalf("Expected ValueParams content-type, got %T", tree.ParsedHeader["content-type"])
	}

	if ct.Type != "text" || ct.Subtype != "html" {
		t.Errorf("Expected text/html, got %s/%s", ct.Type, ct.Subtype)
	}

	if charset, hasCharset := ct.Params["charset"]; !hasCharset || charset != "utf-8" {
		t.Errorf("Expected charset utf-8, got %s", charset)
	}
}

func TestContentTypeParameters(t *testing.T) {
	email := `From: sender@example.com
To: recipient@example.com
Subject: Content-Type Parameters Test
Content-Type: text/plain; charset="utf-8"; format=flowed; delsp=yes

Body content`

	tree, err := ParseMIME([]byte(email))
	if err != nil {
		t.Fatalf("Failed to parse email: %v", err)
	}

	ct, ok := tree.ParsedHeader["content-type"].(*ValueParams)
	if !ok {
		t.Fatalf("Expected ValueParams content-type, got %T", tree.ParsedHeader["content-type"])
	}

	// Test main type/subtype
	if ct.Type != "text" || ct.Subtype != "plain" {
		t.Errorf("Expected text/plain, got %s/%s", ct.Type, ct.Subtype)
	}

	// Test parameters
	expectedParams := map[string]string{
		"charset": "utf-8",
		"format":  "flowed",
		"delsp":   "yes",
	}

	if !ct.HasParams {
		t.Error("Expected HasParams to be true")
	}

	for key, expectedValue := range expectedParams {
		if actualValue, exists := ct.Params[key]; !exists || actualValue != expectedValue {
			t.Errorf("Expected parameter %s=%s, got %s=%s", key, expectedValue, key, actualValue)
		}
	}
}

func TestValueParamsParser(t *testing.T) {
	parser := &MIMEParser{}

	testCases := []struct {
		name     string
		input    string
		expected ValueParams
	}{
		{
			name:  "Simple content type",
			input: "text/plain",
			expected: ValueParams{
				Value:     "text/plain",
				Type:      "text",
				Subtype:   "plain",
				Params:    map[string]string{},
				HasParams: false,
			},
		},
		{
			name:  "Content type with charset",
			input: "text/html; charset=utf-8",
			expected: ValueParams{
				Value:     "text/html",
				Type:      "text",
				Subtype:   "html",
				Params:    map[string]string{"charset": "utf-8"},
				HasParams: true,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := parser.parseValueParams(tc.input)

			if result.Value != tc.expected.Value {
				t.Errorf("Value: expected '%s', got '%s'", tc.expected.Value, result.Value)
			}

			if result.Type != tc.expected.Type {
				t.Errorf("Type: expected '%s', got '%s'", tc.expected.Type, result.Type)
			}

			if result.Subtype != tc.expected.Subtype {
				t.Errorf("Subtype: expected '%s', got '%s'", tc.expected.Subtype, result.Subtype)
			}

			if result.HasParams != tc.expected.HasParams {
				t.Errorf("HasParams: expected %t, got %t", tc.expected.HasParams, result.HasParams)
			}

			for key, expectedValue := range tc.expected.Params {
				if actualValue, exists := result.Params[key]; !exists || actualValue != expectedValue {
					t.Errorf("Param %s: expected '%s', got '%s'", key, expectedValue, actualValue)
				}
			}
		})
	}
}

func TestParserPerformance(t *testing.T) {
	// Create a moderately complex email
	email := `From: sender@example.com
To: recipient1@example.com, recipient2@example.com
Subject: Performance Test Email
Content-Type: multipart/mixed; boundary="perf-boundary"

--perf-boundary
Content-Type: text/plain; charset=utf-8

This is a performance test email.

--perf-boundary
Content-Type: text/html; charset=utf-8

<html><body><h1>Performance Test</h1></body></html>

--perf-boundary--`

	emailBytes := []byte(email)
	iterations := 100

	start := time.Now()
	for i := 0; i < iterations; i++ {
		tree, err := ParseMIME(emailBytes)
		if err != nil {
			t.Fatalf("Parse failed on iteration %d: %v", i, err)
		}
		if tree == nil {
			t.Fatalf("Nil tree on iteration %d", i)
		}
	}
	duration := time.Since(start)

	avgDuration := duration / time.Duration(iterations)
	t.Logf("Parsed %d emails in %v (avg: %v per email)", iterations, duration, avgDuration)

	// Performance should be reasonable
	if avgDuration > 10*time.Millisecond {
		t.Errorf("Performance slower than expected: %v per email", avgDuration)
	}
}

// Benchmark tests
func BenchmarkSimpleEmailParsing(b *testing.B) {
	email := []byte(`From: sender@example.com
To: recipient@example.com
Subject: Benchmark Test
Content-Type: text/plain

Simple email body for benchmarking.`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ParseMIME(email)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMultipartEmailParsing(b *testing.B) {
	email := []byte(`From: sender@example.com
To: recipient@example.com
Subject: Multipart Benchmark
Content-Type: multipart/alternative; boundary="bench"

--bench
Content-Type: text/plain

Plain text part

--bench
Content-Type: text/html

<html><body>HTML part</body></html>

--bench--`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ParseMIME(email)
		if err != nil {
			b.Fatal(err)
		}
	}
}