# RFC822 Email Parser

A high-performance Go package for parsing RFC822 email messages with complete MIME support.

## Features

- **RFC822 Compliant**: Full support for RFC822 email message format
- **MIME Parsing**: Complete multipart MIME structure parsing
- **Header Processing**: Automatic header parsing with folding support
- **Address Parsing**: Email address extraction and validation
- **Content-Type Handling**: Parameter parsing for Content-Type headers
- **Attachment Support**: Automatic detection and parsing of attachments
- **Performance Optimized**: ~117μs per simple email, ~175μs per multipart
- **High Test Coverage**: 85.4% test coverage with comprehensive test suite

## Installation

```bash
go get github.com/geoffreyhinton/mail_go/indexer
```

## Quick Start

```go
package main

import (
    "fmt"
    "log"
    
    "github.com/geoffreyhinton/mail_go/indexer"
)

func main() {
    email := `From: sender@example.com
To: recipient@example.com
Subject: Hello World
Content-Type: text/plain

Hello from Go email parser!`

    tree, err := indexer.ParseMIME([]byte(email))
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Subject: %s\n", tree.ParsedHeader["subject"])
    fmt.Printf("From: %v\n", tree.ParsedHeader["from"])
    fmt.Printf("Body: %s\n", string(tree.Body))
}
```

## API Reference

### Main Functions

#### `ParseMIME(rfc822 []byte) (*MIMENode, error)`

Parses an RFC822 email message and returns the MIME tree structure.

**Parameters:**
- `rfc822`: Raw email content as byte slice

**Returns:**
- `*MIMENode`: Parsed MIME tree structure
- `error`: Parse error if any

### Data Structures

#### `MIMENode`

Represents a node in the MIME tree structure:

```go
type MIMENode struct {
    RootNode       bool                   // True if this is the root node
    ChildNodes     []*MIMENode           // Child MIME parts
    Header         []string              // Raw header lines
    ParsedHeader   map[string]interface{} // Parsed headers
    Body           []byte                // Message body content
    Multipart      string                // Multipart type (e.g., "mixed", "alternative")
    Boundary       string                // Multipart boundary string
    ParentBoundary string                // Parent's boundary string
    LineCount      int                   // Number of lines in body
    Size           int                   // Size in bytes
    Message        *MIMENode             // Embedded message (for message/rfc822)
}
```

#### `Address`

Represents an email address:

```go
type Address struct {
    Name    string // Display name (optional)
    Address string // Email address
}
```

#### `ValueParams`

Represents a parsed header value with parameters:

```go
type ValueParams struct {
    Value     string            // Main value
    Type      string            // Primary type (for Content-Type)
    Subtype   string            // Sub-type (for Content-Type)
    Params    map[string]string // Parameters
    HasParams bool              // Whether parameters exist
}
```

## Usage Examples

### Basic Email Parsing

```go
email := `From: john@example.com
Subject: Test Email
Content-Type: text/plain

Hello World!`

tree, _ := indexer.ParseMIME([]byte(email))

// Access headers
subject := tree.ParsedHeader["subject"].(string)
from := tree.ParsedHeader["from"].([]*indexer.Address)[0]
contentType := tree.ParsedHeader["content-type"].(*indexer.ValueParams)

fmt.Printf("Subject: %s\n", subject)
fmt.Printf("From: %s <%s>\n", from.Name, from.Address)
fmt.Printf("Type: %s/%s\n", contentType.Type, contentType.Subtype)
```

### Multipart Email Processing

```go
multipartEmail := `Content-Type: multipart/alternative; boundary="bound"

--bound
Content-Type: text/plain

Plain text version

--bound
Content-Type: text/html

<html><body>HTML version</body></html>

--bound--`

tree, _ := indexer.ParseMIME([]byte(multipartEmail))

fmt.Printf("Multipart type: %s\n", tree.Multipart)
fmt.Printf("Parts count: %d\n", len(tree.ChildNodes))

for i, child := range tree.ChildNodes {
    ct := child.ParsedHeader["content-type"].(*indexer.ValueParams)
    fmt.Printf("Part %d: %s/%s\n", i+1, ct.Type, ct.Subtype)
}
```

### Attachment Detection

```go
for _, child := range tree.ChildNodes {
    ct := child.ParsedHeader["content-type"].(*indexer.ValueParams)
    
    // Check if it's an attachment
    if ct.Type == "application" || ct.Type == "image" {
        filename := ""
        if name, exists := ct.Params["name"]; exists {
            filename = name
        }
        
        fmt.Printf("Attachment: %s/%s (%s)\n", ct.Type, ct.Subtype, filename)
    }
}
```

### Header Processing

```go
// Access different header types
subject := tree.ParsedHeader["subject"].(string)
date := tree.ParsedHeader["date"].(string)

// Email addresses
from := tree.ParsedHeader["from"].([]*indexer.Address)
to := tree.ParsedHeader["to"].([]*indexer.Address)

// Content parameters
ct := tree.ParsedHeader["content-type"].(*indexer.ValueParams)
charset := ct.Params["charset"] // e.g., "utf-8"
```

## Performance

The parser is optimized for high-performance email processing:

- **Simple emails**: ~117 microseconds per email
- **Multipart emails**: ~175 microseconds per multipart email
- **Memory efficient**: Minimal allocations during parsing
- **Scalable**: Tested with thousands of concurrent parses

### Benchmark Results

```
BenchmarkSimpleEmailParsing-4      11782    117508 ns/op    46459 B/op    530 allocs/op
BenchmarkMultipartEmailParsing-4    6607    175079 ns/op    95267 B/op   1018 allocs/op
```

## Testing

The package includes comprehensive tests covering:

- Basic email parsing
- Multipart structure handling
- Nested multipart emails
- Address parsing validation
- Header folding support
- Content-Type parameter parsing
- Performance benchmarks
- Edge cases and malformed emails

Run tests:

```bash
go test -v
go test -bench=. -benchmem
go test -cover
```

**Current test coverage: 85.4%**

## Supported Features

### RFC822 Compliance

- ✅ Header parsing with folding support
- ✅ Email address parsing and validation
- ✅ Date header parsing
- ✅ Message-ID handling
- ✅ Subject line processing

### MIME Support

- ✅ Multipart message parsing
- ✅ Nested multipart structures
- ✅ Content-Type parameter parsing
- ✅ Content-Disposition handling
- ✅ Transfer encoding recognition
- ✅ Boundary detection and processing

### Content Types

- ✅ `text/plain` and `text/html`
- ✅ `multipart/mixed`, `multipart/alternative`, `multipart/related`
- ✅ `application/*` attachments
- ✅ `image/*` attachments
- ✅ `message/rfc822` embedded messages

### Special Features

- ✅ Header value parameter parsing
- ✅ Quoted parameter value handling
- ✅ Multiple address parsing
- ✅ Attachment filename extraction
- ✅ Line counting and size calculation

## Error Handling

The parser handles various email format issues gracefully:

- Malformed headers (skipped with warnings)
- Missing required headers (defaults provided)
- Invalid boundaries (fallback parsing)
- Encoding issues (best-effort decoding)

## Integration

This parser integrates seamlessly with:

- IMAP servers (body structure generation)
- Email indexing systems (MongoDB integration available)
- Content analysis pipelines
- Email archival systems
- Message filtering systems

## License

MIT License - see LICENSE file for details.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Add comprehensive tests
4. Ensure all tests pass
5. Submit a pull request

## Related Packages

- `bodystructure.go`: IMAP BODYSTRUCTURE generation
- `indexer.go`: MongoDB email indexing
- Integration examples available in the repository