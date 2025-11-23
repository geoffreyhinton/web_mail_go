package indexer

import (
	"fmt"
	"net/mail"
	"regexp"
	"strings"
)

// MIMENode represents a node in the MIME tree
type MIMENode struct {
	RootNode       bool                   `json:"rootNode,omitempty"`
	ChildNodes     []*MIMENode            `json:"childNodes,omitempty"`
	Header         []string               `json:"header,omitempty"`
	ParsedHeader   map[string]interface{} `json:"parsedHeader"`
	Body           []byte                 `json:"body,omitempty"`
	Multipart      string                 `json:"multipart,omitempty"`
	Boundary       string                 `json:"boundary,omitempty"`
	ParentBoundary string                 `json:"parentBoundary,omitempty"`
	LineCount      int                    `json:"lineCount,omitempty"`
	Size           int                    `json:"size,omitempty"`
	Message        *MIMENode              `json:"message,omitempty"`

	// Internal fields for parsing
	state      string
	parentNode *MIMENode
}

// ValueParams represents a parsed header value with parameters
type ValueParams struct {
	Value     string            `json:"value"`
	Type      string            `json:"type"`
	Subtype   string            `json:"subtype"`
	Params    map[string]string `json:"params"`
	HasParams bool              `json:"hasParams,omitempty"`
}

// Address represents an email address
type Address struct {
	Name    string `json:"name,omitempty"`
	Address string `json:"address"`
}

// MIMEParser handles parsing of RFC822 messages
type MIMEParser struct {
	rfc822  string
	pos     int
	br      string
	rawBody string
	tree    *MIMENode
	node    *MIMENode
}

// NewMIMEParser creates a new parser instance
func NewMIMEParser(rfc822 []byte) *MIMEParser {
	parser := &MIMEParser{
		rfc822: string(rfc822),
		pos:    0,
		tree: &MIMENode{
			RootNode:     true,
			ChildNodes:   make([]*MIMENode, 0),
			ParsedHeader: make(map[string]interface{}),
		},
	}
	parser.node = parser.createNode(parser.tree)
	return parser
}

// Parse processes the message line by line
func (p *MIMEParser) Parse() error {
	var prevBr string

	for p.br != "" || p.pos < len(p.rfc822) {
		line := p.readLine()

		switch p.node.state {
		case "header":
			if p.rawBody != "" {
				p.rawBody += prevBr + line
			}

			if line == "" {
				p.processNodeHeader()
				p.processContentType()
				p.node.state = "body"
			} else {
				p.node.Header = append(p.node.Header, line)
			}

		case "body":
			p.rawBody += prevBr + line

			if p.node.ParentBoundary != "" &&
				(line == "--"+p.node.ParentBoundary ||
					line == "--"+p.node.ParentBoundary+"--") {

				if contentType, ok := p.node.ParsedHeader["content-type"].(*ValueParams); ok {
					if contentType.Value == "message/rfc822" {
						if len(p.node.Body) > 0 {
							subParser := NewMIMEParser(p.node.Body)
							subParser.Parse()
							p.node.Message = subParser.GetResult()
						}
					}
				}

				if line == "--"+p.node.ParentBoundary {
					p.node = p.createNode(p.node.parentNode)
				} else {
					p.node = p.node.parentNode
				}
			} else if p.node.Boundary != "" && line == "--"+p.node.Boundary {
				p.node = p.createNode(p.node)
			} else {
				if len(p.node.Body) > 0 {
					p.node.Body = append(p.node.Body, []byte(prevBr+line)...)
				} else {
					p.node.Body = []byte(line)
				}
			}

		default:
			return fmt.Errorf("unexpected state: %s", p.node.state)
		}

		prevBr = p.br
		if p.br == "" {
			break
		}
	}

	return nil
}

// readLine reads a line from the message body
func (p *MIMEParser) readLine() string {
	if p.pos >= len(p.rfc822) {
		p.br = ""
		return ""
	}

	re := regexp.MustCompile(`(.*?)(\r?\n|$)`)
	remaining := p.rfc822[p.pos:]
	matches := re.FindStringSubmatch(remaining)

	if matches != nil {
		p.br = matches[2]
		if p.br == "" && p.pos+len(matches[0]) < len(p.rfc822) {
			p.br = "\n"
		}
		p.pos += len(matches[0])
		return matches[1]
	}

	p.br = ""
	return ""
}

// createNode creates a new node with default values
func (p *MIMEParser) createNode(parentNode *MIMENode) *MIMENode {
	node := &MIMENode{
		state:          "header",
		ChildNodes:     make([]*MIMENode, 0),
		Header:         make([]string, 0),
		ParsedHeader:   make(map[string]interface{}),
		Body:           make([]byte, 0),
		ParentBoundary: parentNode.Boundary,
		parentNode:     parentNode,
	}
	parentNode.ChildNodes = append(parentNode.ChildNodes, node)
	return node
}

// processNodeHeader processes header lines and splits them to key-value pairs
func (p *MIMEParser) processNodeHeader() {
	// Process folded headers
	for i := len(p.node.Header) - 1; i >= 0; i-- {
		if i > 0 && regexp.MustCompile(`^\s`).MatchString(p.node.Header[i]) {
			p.node.Header[i-1] = p.node.Header[i-1] + "\r\n" + p.node.Header[i]
			p.node.Header = append(p.node.Header[:i], p.node.Header[i+1:]...)
		} else {
			parts := strings.SplitN(p.node.Header[i], ":", 2)
			if len(parts) == 2 {
				key := strings.ToLower(strings.TrimSpace(parts[0]))
				value := strings.TrimSpace(parts[1])

				// Validate key format
				if regexp.MustCompile(`^[a-zA-Z0-9\-\*]`).MatchString(key) && len(key) < 100 {
					// Clean up folded headers
					value = regexp.MustCompile(`\s*\r?\n\s*`).ReplaceAllString(value, " ")

					if existing, exists := p.node.ParsedHeader[key]; exists {
						if arr, isArray := existing.([]string); isArray {
							p.node.ParsedHeader[key] = append([]string{value}, arr...)
						} else {
							p.node.ParsedHeader[key] = []string{value, existing.(string)}
						}
					} else {
						p.node.ParsedHeader[key] = value
					}
				}
			}
		}
	}

	// Ensure Content-Type presence
	if _, exists := p.node.ParsedHeader["content-type"]; !exists {
		p.node.ParsedHeader["content-type"] = "text/plain"
	}

	// Parse content-type and content-disposition
	for _, key := range []string{"content-type", "content-disposition"} {
		if headerValue, exists := p.node.ParsedHeader[key]; exists {
			var value string
			if arr, isArray := headerValue.([]string); isArray {
				value = arr[len(arr)-1] // Get last value
			} else {
				value = headerValue.(string)
			}
			p.node.ParsedHeader[key] = p.parseValueParams(value)
		}
	}

	// Ensure single values for specific fields
	singleValueFields := []string{
		"content-transfer-encoding", "content-id", "content-description",
		"content-language", "content-md5", "content-location",
	}
	for _, key := range singleValueFields {
		if headerValue, exists := p.node.ParsedHeader[key]; exists {
			if arr, isArray := headerValue.([]string); isArray {
				p.node.ParsedHeader[key] = arr[len(arr)-1]
			}
		}
	}

	// Parse address fields
	addressFields := []string{"from", "sender", "reply-to", "to", "cc", "bcc"}
	for _, key := range addressFields {
		if headerValue, exists := p.node.ParsedHeader[key]; exists {
			addresses := make([]*Address, 0)
			var values []string

			if arr, isArray := headerValue.([]string); isArray {
				values = arr
			} else {
				values = []string{headerValue.(string)}
			}

			for _, value := range values {
				if value != "" {
					parsed := p.parseAddresses(value)
					addresses = append(addresses, parsed...)
				}
			}

			if len(addresses) > 0 {
				p.node.ParsedHeader[key] = addresses
			}
		}
	}
}

// parseValueParams splits a header value into structured data
func (p *MIMEParser) parseValueParams(headerValue string) *ValueParams {
	data := &ValueParams{
		Params: make(map[string]string),
	}

	parts := strings.Split(headerValue, ";")
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if i == 0 {
			data.Value = part
			typeParts := strings.Split(part, "/")
			if len(typeParts) >= 2 {
				data.Type = strings.ToLower(typeParts[0])
				data.Subtype = strings.Join(typeParts[1:], "/")
			} else {
				data.Type = strings.ToLower(part)
			}
		} else {
			paramParts := strings.SplitN(part, "=", 2)
			if len(paramParts) == 2 {
				key := strings.ToLower(strings.TrimSpace(paramParts[0]))
				value := strings.Trim(strings.TrimSpace(paramParts[1]), `"'`)

				if regexp.MustCompile(`^[a-zA-Z0-9\-\*]`).MatchString(key) && len(key) < 100 {
					data.Params[key] = value
					data.HasParams = true
				}
			}
		}
	}

	return data
}

// parseAddresses parses email addresses from a header value
func (p *MIMEParser) parseAddresses(value string) []*Address {
	addresses := make([]*Address, 0)

	addrs, err := mail.ParseAddressList(value)
	if err != nil {
		// Fallback for malformed addresses
		return addresses
	}

	for _, addr := range addrs {
		addresses = append(addresses, &Address{
			Name:    addr.Name,
			Address: addr.Address,
		})
	}

	return addresses
}

// processContentType checks Content-Type value for multipart handling
func (p *MIMEParser) processContentType() {
	contentType, exists := p.node.ParsedHeader["content-type"]
	if !exists {
		return
	}

	ct, ok := contentType.(*ValueParams)
	if !ok {
		return
	}

	if ct.Type == "multipart" {
		if boundary, hasBoundary := ct.Params["boundary"]; hasBoundary {
			p.node.Multipart = ct.Subtype
			p.node.Boundary = boundary
		}
	}
}

// FinalizeTree joins body arrays and removes unnecessary fields
func (p *MIMEParser) FinalizeTree() {
	p.finalizeNode(p.tree)
}

func (p *MIMEParser) finalizeNode(node *MIMENode) {
	if len(node.Body) > 0 {
		// Count lines in body
		node.LineCount = strings.Count(string(node.Body), "\n") + 1

		// Ensure proper line endings
		bodyStr := string(node.Body)
		bodyStr = regexp.MustCompile(`\r?\n`).ReplaceAllString(bodyStr, "\r\n")
		node.Body = []byte(bodyStr)
		node.Size = len(node.Body)
	}

	for _, child := range node.ChildNodes {
		p.finalizeNode(child)
	}

	// Clean up fields that aren't needed in final output
	if len(node.ChildNodes) == 0 {
		node.ChildNodes = nil
	}
}

// GetResult returns the parsed result
func (p *MIMEParser) GetResult() *MIMENode {
	if len(p.tree.ChildNodes) > 0 {
		return p.tree.ChildNodes[0]
	}
	return nil
}

// ParseMIME parses an RFC822 message and returns the MIME tree
func ParseMIME(rfc822 []byte) (*MIMENode, error) {
	parser := NewMIMEParser(rfc822)
	err := parser.Parse()
	if err != nil {
		return nil, err
	}

	parser.FinalizeTree()
	return parser.GetResult(), nil
}
