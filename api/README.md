# Mail API

A comprehensive mail API built with Go, Gin framework, and MongoDB, based on the Wild Duck mail server architecture.

## Features

- **User Management**: Complete CRUD operations for users with authentication
- **Address Management**: Email address management with main address support
- **Mailbox Management**: Create and manage IMAP mailboxes (INBOX, Sent, Drafts, etc.)
- **Message Management**: Full message CRUD with attachment support
- **Search**: Full-text search across messages
- **Pagination**: Efficient pagination for all list endpoints
- **Storage Quota**: Track and manage user storage quotas

## API Endpoints

### Users
- `GET /api/users` - List users with pagination and search
- `POST /api/users` - Create a new user
- `GET /api/users/:id` - Get user details
- `PUT /api/users/:id` - Update user information
- `DELETE /api/users/:id` - Delete user
- `POST /api/users/:id/quota/reset` - Reset user storage quota

### Addresses
- `GET /api/addresses` - List all email addresses
- `GET /api/users/:id/addresses` - Get user's email addresses
- `POST /api/users/:id/addresses` - Add address to user
- `GET /api/users/:id/addresses/:addressId` - Get specific address
- `PUT /api/users/:id/addresses/:addressId` - Update address (set as main)
- `DELETE /api/users/:id/addresses/:addressId` - Delete address

### Mailboxes
- `GET /api/users/:id/mailboxes` - Get user's mailboxes
- `POST /api/users/:id/mailboxes` - Create new mailbox
- `GET /api/users/:id/mailboxes/:mailboxId` - Get mailbox details
- `PUT /api/users/:id/mailboxes/:mailboxId` - Update mailbox
- `DELETE /api/users/:id/mailboxes/:mailboxId` - Delete mailbox

### Messages
- `GET /api/users/:id/mailboxes/:mailboxId/messages` - List messages
- `GET /api/users/:id/mailboxes/:mailboxId/messages/:messageId` - Get message
- `PUT /api/users/:id/mailboxes/:mailboxId/messages/:messageId` - Update message flags
- `DELETE /api/users/:id/mailboxes/:mailboxId/messages/:messageId` - Delete message
- `GET /api/users/:id/mailboxes/:mailboxId/messages/:messageId/attachments/:attachmentId` - Get attachment
- `GET /api/users/:id/search` - Search user's messages

### Health Check
- `GET /health` - API health check

## Installation

1. Clone the repository:
```bash
git clone https://github.com/geoffreyhinton/mail_go.git
cd mail_go/api
```

2. Install dependencies:
```bash
go mod tidy
```

3. Set environment variables:
```bash
export PORT=8080
export MONGO_URL=mongodb://localhost:27017
export DB_NAME=wildmail
```

4. Run the server:
```bash
go run main.go
```

## Configuration

The API can be configured using environment variables:

- `PORT` - Server port (default: 8080)
- `MONGO_URL` - MongoDB connection URL (default: mongodb://localhost:27017)
- `DB_NAME` - Database name (default: wildmail)

## Database Schema

### Collections

#### users
```json
{
  "_id": "ObjectId",
  "username": "string",
  "password": "string (hashed)",
  "address": "string (main email)",
  "language": "string",
  "retention": "number",
  "quota": "number",
  "storageUsed": "number",
  "recipients": "number",
  "forwards": "number",
  "activated": "boolean",
  "disabled": "boolean",
  "created": "date",
  "updated": "date"
}
```

#### addresses
```json
{
  "_id": "ObjectId",
  "user": "ObjectId",
  "address": "string",
  "main": "boolean",
  "created": "date"
}
```

#### mailboxes
```json
{
  "_id": "ObjectId",
  "user": "ObjectId",
  "path": "string",
  "name": "string",
  "specialUse": "string",
  "retention": "number",
  "subscribed": "boolean",
  "modifyIndex": "number",
  "uidNext": "number",
  "uidValidity": "number",
  "created": "date",
  "updated": "date"
}
```

#### messages
```json
{
  "_id": "ObjectId",
  "user": "ObjectId",
  "mailbox": "ObjectId",
  "uid": "number",
  "size": "number",
  "flags": ["string"],
  "subject": "string",
  "messageId": "string",
  "date": "date",
  "from": {"name": "string", "address": "string"},
  "to": [{"name": "string", "address": "string"}],
  "html": ["string"],
  "text": "string",
  "attachments": [...],
  "unseen": "boolean",
  "undeleted": "boolean",
  "flagged": "boolean",
  "draft": "boolean",
  "created": "date"
}
```

## Example Usage

### Create a User
```bash
curl -X POST http://localhost:8080/api/users \
  -H "Content-Type: application/json" \
  -d '{
    "username": "john",
    "password": "password123",
    "address": "john@example.com",
    "quota": 1073741824
  }'
```

### List Messages
```bash
curl "http://localhost:8080/api/users/507f1f77bcf86cd799439011/mailboxes/507f1f77bcf86cd799439012/messages?limit=10&page=1"
```

### Search Messages
```bash
curl "http://localhost:8080/api/users/507f1f77bcf86cd799439011/search?query=important&limit=20"
```

## Development

### Project Structure
```
api/
├── handlers/          # HTTP request handlers
│   ├── user_handler.go
│   ├── address_handler.go
│   ├── mailbox_handler.go
│   └── message_handler.go
├── models/            # Data models
│   └── models.go
├── middleware/        # HTTP middleware
│   └── middleware.go
├── utils/             # Utility functions
│   └── utils.go
├── main.go           # Main application entry point
└── README.md         # This file
```

### Adding New Features

1. Define models in `models/models.go`
2. Create handler in appropriate `handlers/*_handler.go`
3. Add routes in `main.go`
4. Add validation and error handling

### Testing

Run tests with:
```bash
go test ./...
```

## TODO

- [ ] Implement GridFS for attachment storage
- [ ] Add Redis caching for better performance
- [ ] Implement proper logging
- [ ] Add rate limiting
- [ ] Add authentication middleware
- [ ] Implement server-sent events for real-time updates
- [ ] Add comprehensive test suite
- [ ] Add Docker support
- [ ] Implement message threading
- [ ] Add IMAP server integration

## License

MIT License - see LICENSE file for details.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request