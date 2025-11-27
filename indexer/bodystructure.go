package indexer

import (
	"fmt"
	"strconv"
	"strings"
)

// BodyStructure handles creation of IMAP BODYSTRUCTURE responses
type BodyStructure struct {
	tree          *MIMENode
	options       *BodyStructureOptions
	currentPath   string
	bodyStructure interface{}
}

// BodyStructureOptions contains configuration options for body structure generation
type BodyStructureOptions struct {
	ContentLanguageString bool // Convert single element array to string for Content-Language
	UpperCaseKeys         bool // Use only upper case key names
	SkipContentLocation   bool // Do not include Content-Location in the output
	Body                  bool // Skip extension fields (needed for BODY)
	AttachmentRFC822      bool // Treat message/rfc822 as attachment
}

// NewBodyStructure creates a new BodyStructure instance
func NewBodyStructure(tree *MIMENode, options *BodyStructureOptions) *BodyStructure {
	if options == nil {
		options = &BodyStructureOptions{}
	}

	bs := &BodyStructure{
		tree:        tree,
		options:     options,
		currentPath: "",
	}

	bs.bodyStructure = bs.createBodyStructure(tree, options)
	return bs
}

// Create returns the generated body structure
func (bs *BodyStructure) Create() interface{} {
	return bs.bodyStructure
}

// createBodyStructure generates an object that can be serialized into a BODYSTRUCTURE string
func (bs *BodyStructure) createBodyStructure(node *MIMENode, options *BodyStructureOptions) interface{} {
	if node == nil {
		return []interface{}{}
	}

	contentType := bs.getContentType(node)

	switch contentType.Type {
	case "multipart":
		return bs.processMultipartNode(node, options)
	case "text":
		return bs.processTextNode(node, options)
	case "message":
		if contentType.Subtype == "rfc822" {
			if !options.AttachmentRFC822 {
				return bs.processRFC822Node(node, options)
			}
			return bs.processAttachmentNode(node, options)
		}
		fallthrough
	default:
		return bs.processAttachmentNode(node, options)
	}
}

// getContentType safely extracts content type information from a node
func (bs *BodyStructure) getContentType(node *MIMENode) *ValueParams {
	if ctHeader, exists := node.ParsedHeader["content-type"]; exists {
		if ct, ok := ctHeader.(*ValueParams); ok {
			return ct
		}
	}

	// Default content type
	return &ValueParams{
		Type:    "text",
		Subtype: "plain",
		Value:   "text/plain",
		Params:  make(map[string]string),
	}
}

// getBasicFields generates a list of basic fields any non-multipart part should have
func (bs *BodyStructure) getBasicFields(node *MIMENode, options *BodyStructureOptions) []interface{} {
	contentType := bs.getContentType(node)

	bodyType := contentType.Type
	bodySubtype := contentType.Subtype

	if bodyType == "" {
		bodyType = "text"
	}
	if bodySubtype == "" {
		bodySubtype = "plain"
	}

	contentTransfer := "7bit"
	if cte, exists := node.ParsedHeader["content-transfer-encoding"]; exists {
		if cteStr, ok := cte.(string); ok {
			contentTransfer = cteStr
		}
	}

	// Handle case conversion
	if options.UpperCaseKeys {
		bodyType = strings.ToUpper(bodyType)
		bodySubtype = strings.ToUpper(bodySubtype)
		contentTransfer = strings.ToUpper(contentTransfer)
	}

	// Build parameter list
	var paramList interface{}
	if contentType.HasParams && len(contentType.Params) > 0 {
		params := make([]interface{}, 0)
		for key, value := range contentType.Params {
			keyName := key
			if options.UpperCaseKeys {
				keyName = strings.ToUpper(key)
			}
			params = append(params, keyName, value)
		}
		paramList = params
	}

	// Content-ID
	var contentID interface{}
	if cid, exists := node.ParsedHeader["content-id"]; exists {
		contentID = cid
	}

	// Content-Description
	var contentDesc interface{}
	if desc, exists := node.ParsedHeader["content-description"]; exists {
		contentDesc = desc
	}

	return []interface{}{
		bodyType,        // body type
		bodySubtype,     // body subtype
		paramList,       // body parameter parenthesized list
		contentID,       // body id
		contentDesc,     // body description
		contentTransfer, // body encoding
		node.Size,       // body size
	}
}

// getExtensionFields generates a list of extension fields any non-multipart part should have
func (bs *BodyStructure) getExtensionFields(node *MIMENode, options *BodyStructureOptions) []interface{} {
	// Content-MD5
	var contentMD5 interface{}
	if md5, exists := node.ParsedHeader["content-md5"]; exists {
		contentMD5 = md5
	}

	// Content-Disposition
	var disposition interface{}
	if disp, exists := node.ParsedHeader["content-disposition"]; exists {
		if dispParams, ok := disp.(*ValueParams); ok {
			dispValue := dispParams.Value
			if options.UpperCaseKeys {
				dispValue = strings.ToUpper(dispValue)
			}

			var dispParamList interface{}
			if dispParams.HasParams && len(dispParams.Params) > 0 {
				params := make([]interface{}, 0)
				for key, value := range dispParams.Params {
					keyName := key
					if options.UpperCaseKeys {
						keyName = strings.ToUpper(key)
					}
					params = append(params, keyName, value)
				}
				dispParamList = params
			}

			disposition = []interface{}{dispValue, dispParamList}
		}
	}

	// Content-Language
	var language interface{}
	if lang, exists := node.ParsedHeader["content-language"]; exists {
		if langStr, ok := lang.(string); ok {
			// Clean up language string
			langStr = strings.ReplaceAll(langStr, " ", ",")
			langStr = strings.ReplaceAll(langStr, ",,", ",")
			langStr = strings.Trim(langStr, ",")

			if langStr != "" {
				langList := strings.Split(langStr, ",")
				if len(langList) == 1 && options.ContentLanguageString {
					language = langList[0]
				} else {
					language = langList
				}
			}
		}
	}

	result := []interface{}{
		contentMD5,  // body MD5
		disposition, // body disposition
		language,    // body language
	}

	// Content-Location (optional based on settings)
	if !options.SkipContentLocation {
		var contentLocation interface{}
		if loc, exists := node.ParsedHeader["content-location"]; exists {
			contentLocation = loc
		}
		result = append(result, contentLocation)
	}

	return result
}

// processMultipartNode processes a node with content-type=multipart/*
func (bs *BodyStructure) processMultipartNode(node *MIMENode, options *BodyStructureOptions) []interface{} {
	result := make([]interface{}, 0)

	// Add child structures
	if len(node.ChildNodes) > 0 {
		for _, child := range node.ChildNodes {
			result = append(result, bs.createBodyStructure(child, options))
		}
	} else {
		result = append(result, []interface{}{})
	}

	// Add multipart subtype
	subtype := node.Multipart
	if subtype == "" {
		subtype = "mixed" // default
	}
	if options.UpperCaseKeys {
		subtype = strings.ToUpper(subtype)
	}
	result = append(result, subtype)

	// Add multipart parameters
	contentType := bs.getContentType(node)
	var paramList interface{}
	if contentType.HasParams && len(contentType.Params) > 0 {
		params := make([]interface{}, 0)
		for key, value := range contentType.Params {
			keyName := key
			if options.UpperCaseKeys {
				keyName = strings.ToUpper(key)
			}
			params = append(params, keyName, value)
		}
		paramList = params
	}
	result = append(result, paramList)

	// Add extension fields for multipart (skip MD5)
	if !options.Body {
		extFields := bs.getExtensionFields(node, options)
		result = append(result, extFields[1:]...) // Skip MD5 (first field)
	}

	return result
}

// processTextNode processes a node with content-type=text/*
func (bs *BodyStructure) processTextNode(node *MIMENode, options *BodyStructureOptions) []interface{} {
	result := bs.getBasicFields(node, options)

	// Add line count for text parts
	result = append(result, node.LineCount)

	// Add extension fields
	if !options.Body {
		result = append(result, bs.getExtensionFields(node, options)...)
	}

	return result
}

// processAttachmentNode processes a non-text, non-multipart node
func (bs *BodyStructure) processAttachmentNode(node *MIMENode, options *BodyStructureOptions) []interface{} {
	result := bs.getBasicFields(node, options)

	// Add extension fields
	if !options.Body {
		result = append(result, bs.getExtensionFields(node, options)...)
	}

	return result
}

// processRFC822Node processes a node with content-type=message/rfc822
func (bs *BodyStructure) processRFC822Node(node *MIMENode, options *BodyStructureOptions) []interface{} {
	result := bs.getBasicFields(node, options)

	// Add envelope structure
	envelope := bs.createEnvelope(node.Message)
	result = append(result, envelope)

	// Add body structure of the embedded message
	if node.Message != nil {
		embeddedStructure := bs.createBodyStructure(node.Message, options)
		result = append(result, embeddedStructure)
	} else {
		result = append(result, []interface{}{})
	}

	// Add line count
	result = append(result, node.LineCount)

	// Add extension fields
	if !options.Body {
		result = append(result, bs.getExtensionFields(node, options)...)
	}

	return result
}

// createEnvelope creates an IMAP envelope structure from parsed headers
func (bs *BodyStructure) createEnvelope(node *MIMENode) []interface{} {
	if node == nil {
		return []interface{}{nil, nil, nil, nil, nil, nil, nil, nil, nil, nil}
	}

	envelope := make([]interface{}, 10)

	// Date
	if date, exists := node.ParsedHeader["date"]; exists {
		envelope[0] = date
	}

	// Subject
	if subject, exists := node.ParsedHeader["subject"]; exists {
		envelope[1] = subject
	}

	// From
	envelope[2] = bs.formatAddresses(node.ParsedHeader["from"])

	// Sender
	envelope[3] = bs.formatAddresses(node.ParsedHeader["sender"])

	// Reply-To
	envelope[4] = bs.formatAddresses(node.ParsedHeader["reply-to"])

	// To
	envelope[5] = bs.formatAddresses(node.ParsedHeader["to"])

	// CC
	envelope[6] = bs.formatAddresses(node.ParsedHeader["cc"])

	// BCC
	envelope[7] = bs.formatAddresses(node.ParsedHeader["bcc"])

	// In-Reply-To
	if inReplyTo, exists := node.ParsedHeader["in-reply-to"]; exists {
		envelope[8] = inReplyTo
	}

	// Message-ID
	if messageID, exists := node.ParsedHeader["message-id"]; exists {
		envelope[9] = messageID
	}

	return envelope
}

// formatAddresses converts address structures to IMAP format
func (bs *BodyStructure) formatAddresses(addrs interface{}) interface{} {
	if addrs == nil {
		return nil
	}

	if addresses, ok := addrs.([]*Address); ok && len(addresses) > 0 {
		result := make([]interface{}, len(addresses))
		for i, addr := range addresses {
			// Split email address into parts
			parts := strings.Split(addr.Address, "@")
			var mailbox, host string
			if len(parts) == 2 {
				mailbox = parts[0]
				host = parts[1]
			} else {
				mailbox = addr.Address
			}

			result[i] = []interface{}{
				addr.Name, // personal name
				nil,       // SMTP source route (obsolete)
				mailbox,   // mailbox name
				host,      // domain name
			}
		}
		return result
	}

	return nil
}

// flatten converts all sub-arrays into one level array
func (bs *BodyStructure) flatten(arr interface{}) []interface{} {
	result := make([]interface{}, 0)

	if slice, ok := arr.([]interface{}); ok {
		for _, item := range slice {
			if subSlice, isSlice := item.([]interface{}); isSlice {
				result = append(result, bs.flatten(subSlice)...)
			} else {
				result = append(result, item)
			}
		}
	} else {
		result = append(result, arr)
	}

	return result
}

// CreateBodyStructure is a convenience function to create BODYSTRUCTURE from a MIME tree
func CreateBodyStructure(tree *MIMENode, options *BodyStructureOptions) interface{} {
	bs := NewBodyStructure(tree, options)
	return bs.Create()
}

// SerializeBodyStructure converts the body structure to a string representation
func SerializeBodyStructure(structure interface{}) string {
	return serializeValue(structure)
}

func serializeValue(value interface{}) string {
	switch v := value.(type) {
	case nil:
		return "NIL"
	case string:
		return fmt.Sprintf("\"%s\"", strings.ReplaceAll(v, "\"", "\\\""))
	case int:
		return strconv.Itoa(v)
	case []interface{}:
		if len(v) == 0 {
			return "NIL"
		}
		parts := make([]string, len(v))
		for i, item := range v {
			parts[i] = serializeValue(item)
		}
		return "(" + strings.Join(parts, " ") + ")"
	default:
		return fmt.Sprintf("\"%v\"", v)
	}
}
