package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type User struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Username     string             `bson:"username" json:"username"`
	Password     string             `bson:"password" json:"-"`
	Address      string             `bson:"address" json:"address"`
	Language     string             `bson:"language,omitempty" json:"language,omitempty"`
	Retention    int64              `bson:"retention,omitempty" json:"retention,omitempty"`
	Quota        int64              `bson:"quota,omitempty" json:"quota,omitempty"`
	StorageUsed  int64              `bson:"storageUsed,omitempty" json:"storageUsed,omitempty"`
	Recipients   int64              `bson:"recipients,omitempty" json:"recipients,omitempty"`
	Forwards     int64              `bson:"forwards,omitempty" json:"forwards,omitempty"`
	Activated    bool               `bson:"activated" json:"activated"`
	Disabled     bool               `bson:"disabled" json:"disabled"`
	Created      time.Time          `bson:"created" json:"created"`
	Updated      time.Time          `bson:"updated" json:"updated"`
}

type Address struct {
	ID      primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	User    primitive.ObjectID `bson:"user" json:"user"`
	Address string             `bson:"address" json:"address"`
	Main    bool               `bson:"main,omitempty" json:"main,omitempty"`
	Created time.Time          `bson:"created" json:"created"`
}

type Mailbox struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	User        primitive.ObjectID `bson:"user" json:"user"`
	Path        string             `bson:"path" json:"path"`
	Name        string             `bson:"name" json:"name"`
	SpecialUse  string             `bson:"specialUse,omitempty" json:"specialUse,omitempty"`
	Retention   int64              `bson:"retention,omitempty" json:"retention,omitempty"`
	Subscribed  bool               `bson:"subscribed" json:"subscribed"`
	ModifyIndex int64              `bson:"modifyIndex" json:"modifyIndex"`
	UIDNext     int64              `bson:"uidNext" json:"uidNext"`
	UIDValidity int64              `bson:"uidValidity" json:"uidValidity"`
	Created     time.Time          `bson:"created" json:"created"`
	Updated     time.Time          `bson:"updated" json:"updated"`
}

type EmailAddress struct {
	Name    string `bson:"name" json:"name"`
	Address string `bson:"address" json:"address"`
}

type MessageMeta struct {
	From string `bson:"from,omitempty" json:"from,omitempty"`
	To   string `bson:"to,omitempty" json:"to,omitempty"`
}

type ParsedHeader struct {
	From     []EmailAddress `bson:"from,omitempty" json:"from,omitempty"`
	Sender   []EmailAddress `bson:"sender,omitempty" json:"sender,omitempty"`
	ReplyTo  []EmailAddress `bson:"reply-to,omitempty" json:"replyTo,omitempty"`
	To       []EmailAddress `bson:"to,omitempty" json:"to,omitempty"`
	CC       []EmailAddress `bson:"cc,omitempty" json:"cc,omitempty"`
	BCC      []EmailAddress `bson:"bcc,omitempty" json:"bcc,omitempty"`
	Subject  string         `bson:"subject,omitempty" json:"subject,omitempty"`
	Date     time.Time      `bson:"date,omitempty" json:"date,omitempty"`
	ListID   []EmailAddress `bson:"list-id,omitempty" json:"listId,omitempty"`
	ListUnsub []EmailAddress `bson:"list-unsubscribe,omitempty" json:"listUnsubscribe,omitempty"`
}

type MimeTree struct {
	ParsedHeader ParsedHeader `bson:"parsedHeader,omitempty" json:"parsedHeader,omitempty"`
}

type Attachment struct {
	ID           string `bson:"id" json:"id"`
	Filename     string `bson:"filename,omitempty" json:"filename,omitempty"`
	ContentType  string `bson:"contentType" json:"contentType"`
	Disposition  string `bson:"disposition,omitempty" json:"disposition,omitempty"`
	Size         int64  `bson:"size" json:"size"`
	Related      bool   `bson:"related,omitempty" json:"related,omitempty"`
	ContentId    string `bson:"contentId,omitempty" json:"contentId,omitempty"`
	Encoding     string `bson:"encoding,omitempty" json:"encoding,omitempty"`
}

type Message struct {
	ID          primitive.ObjectID     `bson:"_id,omitempty" json:"id"`
	User        primitive.ObjectID     `bson:"user" json:"user"`
	Mailbox     primitive.ObjectID     `bson:"mailbox" json:"mailbox"`
	UID         int64                  `bson:"uid" json:"uid"`
	ModSeq      int64                  `bson:"modseq" json:"modseq"`
	Size        int64                  `bson:"size" json:"size"`
	Flags       []string               `bson:"flags,omitempty" json:"flags,omitempty"`
	Thread      primitive.ObjectID     `bson:"thread,omitempty" json:"thread,omitempty"`
	Subject     string                 `bson:"subject,omitempty" json:"subject,omitempty"`
	MessageID   string                 `bson:"msgid,omitempty" json:"messageId,omitempty"`
	Date        time.Time              `bson:"hdate" json:"date"`
	Received    time.Time              `bson:"rdate,omitempty" json:"received,omitempty"`
	Intro       string                 `bson:"intro,omitempty" json:"intro,omitempty"`
	Meta        MessageMeta            `bson:"meta,omitempty" json:"meta,omitempty"`
	MimeTree    MimeTree               `bson:"mimeTree,omitempty" json:"mimeTree,omitempty"`
	HTML        []string               `bson:"html,omitempty" json:"html,omitempty"`
	Text        string                 `bson:"text,omitempty" json:"text,omitempty"`
	Attachments []Attachment           `bson:"attachments,omitempty" json:"attachments,omitempty"`
	AttachMap   map[string]string      `bson:"map,omitempty" json:"attachMap,omitempty"`
	HasAttach   bool                   `bson:"ha,omitempty" json:"hasAttachments,omitempty"`
	Unseen      bool                   `bson:"unseen" json:"-"`
	Undeleted   bool                   `bson:"undeleted" json:"-"`
	Flagged     bool                   `bson:"flagged" json:"flagged"`
	Draft       bool                   `bson:"draft" json:"draft"`
	Answered    bool                   `bson:"answered,omitempty" json:"answered,omitempty"`
	Forwarded   bool                   `bson:"forwarded,omitempty" json:"forwarded,omitempty"`
	Searchable  bool                   `bson:"searchable,omitempty" json:"-"`
	Exp         bool                   `bson:"exp,omitempty" json:"-"`
	Created     time.Time              `bson:"created" json:"created"`
}

// GetSeen returns whether the message is seen (inverse of unseen)
func (m *Message) GetSeen() bool {
	return !m.Unseen
}

// GetDeleted returns whether the message is deleted (inverse of undeleted)
func (m *Message) GetDeleted() bool {
	return !m.Undeleted
}

type UserQuota struct {
	Allowed int64 `json:"allowed"`
	Used    int64 `json:"used"`
}

type UserLimits struct {
	Quota      UserQuota              `json:"quota"`
	Recipients map[string]interface{} `json:"recipients"`
	Forwards   map[string]interface{} `json:"forwards"`
}

type UserResponse struct {
	ID        primitive.ObjectID `json:"id"`
	Username  string             `json:"username"`
	Address   string             `json:"address"`
	Language  string             `json:"language,omitempty"`
	Retention int64              `json:"retention,omitempty"`
	Limits    UserLimits         `json:"limits"`
	Activated bool               `json:"activated"`
	Disabled  bool               `json:"disabled"`
}

type PaginatedResponse struct {
	Success bool        `json:"success"`
	Query   string      `json:"query,omitempty"`
	Total   int64       `json:"total"`
	Page    int         `json:"page"`
	Prev    string      `json:"prev,omitempty"`
	Next    string      `json:"next,omitempty"`
	Results interface{} `json:"results"`
}

type APIError struct {
	Error string `json:"error"`
}

type APISuccess struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	ID      interface{} `json:"id,omitempty"`
}