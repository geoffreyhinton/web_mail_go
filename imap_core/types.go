package imapcore

import (
	"bufio"
	"crypto/tls"
	"net"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// SessionState represents the current state of an IMAP session
type SessionState int

// Session states
const (
	StateNotAuthenticated SessionState = iota
	StateAuthenticated
	StateSelected
	StateLogout
)

// IMAPServerOptions configuration for IMAP server
type IMAPServerOptions struct {
	Logger         Logger
	Database       *mongo.Database
	Host           string
	Port           int
	TLSConfig      *tls.Config
	MaxStorage     int64
	Debug          bool
	AuthTimeout    time.Duration
	Secure         bool
	IgnoreSTARTTLS bool
}

// Logger interface for logging
type Logger interface {
	Info(format string, args ...interface{})
	Error(format string, args ...interface{})
	Debug(format string, args ...interface{})
}

// IMAPServer represents an IMAP server
type IMAPServer struct {
	options  *IMAPServerOptions
	listener net.Listener
	sessions map[string]*Session
	mutex    sync.RWMutex
	quit     chan struct{}
	done     chan struct{}
}

// Session represents an IMAP client session
type Session struct {
	ID            string
	conn          net.Conn
	reader        *bufio.Reader
	writer        *bufio.Writer
	scanner       *bufio.Scanner
	server        *IMAPServer
	authenticated bool
	user          *User
	selectedBox   *Mailbox
	state         SessionState
	capabilities  []string
	tls           bool
}

// User represents a user account
type User struct {
	ID       primitive.ObjectID `bson:"_id,omitempty"`
	Username string             `bson:"username"`
	Email    string             `bson:"email"`
	Password string             `bson:"password"`
	Quota    int64              `bson:"quota"`
	Used     int64              `bson:"used"`
}

// Mailbox represents a mailbox/folder
type Mailbox struct {
	ID          primitive.ObjectID `bson:"_id,omitempty"`
	User        primitive.ObjectID `bson:"user"`
	Path        string             `bson:"path"`
	Subscribed  bool               `bson:"subscribed"`
	SpecialUse  string             `bson:"specialUse,omitempty"`
	UIDValidity int64              `bson:"uidValidity"`
	UIDNext     int64              `bson:"uidNext"`
	Flags       []string           `bson:"flags,omitempty"`
	CreatedAt   time.Time          `bson:"createdAt"`
	ModifiedAt  time.Time          `bson:"modifiedAt"`
	UIDList     []int64            `bson:"-"` // Runtime field for UID list
}

// Message represents an email message
type Message struct {
	ID          primitive.ObjectID  `bson:"_id,omitempty"`
	Mailbox     primitive.ObjectID  `bson:"mailbox"`
	UID         int64               `bson:"uid"`
	MessageID   string              `bson:"messageId"`
	From        string              `bson:"from"`
	To          []string            `bson:"to"`
	CC          []string            `bson:"cc,omitempty"`
	BCC         []string            `bson:"bcc,omitempty"`
	Subject     string              `bson:"subject"`
	Date        time.Time           `bson:"date"`
	Size        int64               `bson:"size"`
	BodyText    string              `bson:"bodyText,omitempty"`
	BodyHTML    string              `bson:"bodyHTML,omitempty"`
	Headers     map[string][]string `bson:"headers,omitempty"`
	Attachments []Attachment        `bson:"attachments,omitempty"`
	InReplyTo   string              `bson:"inReplyTo,omitempty"`
	References  []string            `bson:"references,omitempty"`

	// IMAP flags
	Seen     bool `bson:"seen"`
	Answered bool `bson:"answered"`
	Flagged  bool `bson:"flagged"`
	Deleted  bool `bson:"deleted"`
	Draft    bool `bson:"draft"`
	Recent   bool `bson:"recent"`

	CreatedAt time.Time `bson:"createdAt"`
	UpdatedAt time.Time `bson:"updatedAt"`
}

// Attachment represents an email attachment
type Attachment struct {
	ID          string `bson:"id"`
	Filename    string `bson:"filename"`
	ContentType string `bson:"contentType"`
	Size        int64  `bson:"size"`
	GridFSID    string `bson:"gridfsId,omitempty"`
}
