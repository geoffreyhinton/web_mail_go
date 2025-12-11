package lmtp

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"time"
)

// LMTPClient represents an LMTP client for testing
type LMTPClient struct {
	conn net.Conn
}

// NewLMTPClient creates a new LMTP client
func NewLMTPClient(host string, port int) (*LMTPClient, error) {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), 10*time.Second)
	if err != nil {
		return nil, err
	}

	client := &LMTPClient{conn: conn}
	
	// Read greeting
	greeting, err := client.readResponse()
	if err != nil {
		conn.Close()
		return nil, err
	}
	
	log.Printf("LMTP Greeting: %s", greeting)
	return client, nil
}

// Close closes the client connection
func (c *LMTPClient) Close() error {
	if c.conn != nil {
		c.writeCommand("QUIT")
		c.readResponse()
		return c.conn.Close()
	}
	return nil
}

// SendMail sends a mail via LMTP
func (c *LMTPClient) SendMail(from string, to []string, message []byte) error {
	// LHLO command
	err := c.writeCommand("LHLO localhost")
	if err != nil {
		return err
	}
	
	_, err = c.readResponse()
	if err != nil {
		return err
	}

	// MAIL FROM
	err = c.writeCommand(fmt.Sprintf("MAIL FROM:<%s>", from))
	if err != nil {
		return err
	}
	
	_, err = c.readResponse()
	if err != nil {
		return err
	}

	// RCPT TO commands
	for _, recipient := range to {
		err = c.writeCommand(fmt.Sprintf("RCPT TO:<%s>", recipient))
		if err != nil {
			return err
		}
		
		_, err = c.readResponse()
		if err != nil {
			return err
		}
	}

	// DATA command
	err = c.writeCommand("DATA")
	if err != nil {
		return err
	}
	
	_, err = c.readResponse()
	if err != nil {
		return err
	}

	// Send message data
	_, err = c.conn.Write(message)
	if err != nil {
		return err
	}

	// End data with CRLF.CRLF
	err = c.writeCommand("\r\n.")
	if err != nil {
		return err
	}

	// For LMTP, we get per-recipient responses
	for range to {
		_, err = c.readResponse()
		if err != nil {
			return err
		}
	}

	return nil
}

// writeCommand writes a command to the server
func (c *LMTPClient) writeCommand(command string) error {
	_, err := c.conn.Write([]byte(command + "\r\n"))
	return err
}

// readResponse reads a response from the server
func (c *LMTPClient) readResponse() (string, error) {
	buffer := make([]byte, 1024)
	n, err := c.conn.Read(buffer)
	if err != nil {
		return "", err
	}
	
	response := string(buffer[:n])
	log.Printf("LMTP Response: %s", response)
	return response, nil
}

// CreateTestMessage creates a test email message
func CreateTestMessage(from, to, subject, body string) []byte {
	var buf bytes.Buffer
	
	buf.WriteString(fmt.Sprintf("From: %s\r\n", from))
	buf.WriteString(fmt.Sprintf("To: %s\r\n", to))
	buf.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	buf.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().Format(time.RFC1123Z)))
	buf.WriteString("MIME-Version: 1.0\r\n")
	buf.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	buf.WriteString("\r\n")
	buf.WriteString(body)
	buf.WriteString("\r\n")
	
	return buf.Bytes()
}