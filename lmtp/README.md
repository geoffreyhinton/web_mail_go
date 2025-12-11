# LMTP Server

A Local Mail Transfer Protocol (LMTP) server implementation in Go, compatible with the Wild Duck mail server architecture.

## Features

- **LMTP Protocol Support**: Full LMTP implementation based on RFC 2033
- **MongoDB Integration**: Stores messages in MongoDB with proper indexing
- **Message Filtering**: Support for user-defined filters and spam detection
- **Per-Recipient Processing**: LMTP-specific per-recipient response handling
- **Mailbox Management**: Automatic mailbox detection and message routing
- **Storage Quota**: Tracks user storage usage
- **Address Processing**: Handles address normalization and plus addressing

## Architecture

The LMTP server consists of several key components:

- **Server**: Main LMTP server handling connections
- **Session**: Per-connection session management
- **Backend**: SMTP backend implementation for LMTP
- **Message Processing**: Email parsing and indexing
- **Filter Engine**: Message filtering and routing logic

## Configuration

Environment variables for configuration:

- `LMTP_HOST` - Server bind host (default: localhost)
- `LMTP_PORT` - Server port (default: 2003)
- `LMTP_ENABLED` - Enable/disable server (default: true)
- `MONGO_URL` - MongoDB connection URL
- `DB_NAME` - Database name
- `SPAM_HEADER` - Spam detection header (default: X-Spam-Flag)

## Message Flow

1. **Connection**: Client connects to LMTP server
2. **LHLO**: Client sends LMTP hello command
3. **MAIL FROM**: Sender address specification
4. **RCPT TO**: Recipient validation against database
5. **DATA**: Message content processing
6. **Filtering**: Apply user filters and spam detection
7. **Storage**: Store message in appropriate mailbox
8. **Response**: Per-recipient delivery status

## Recipient Validation

The server validates recipients by:

1. Normalizing email addresses (lowercase, trim)
2. Handling plus addressing (user+tag@domain → user@domain)
3. Looking up addresses in MongoDB
4. Verifying associated user exists and is active
5. Checking user permissions and quotas

## Message Processing

For each valid recipient:

1. **Parse Message**: Extract headers and structure
2. **Apply Filters**: Check user-defined filters
3. **Determine Mailbox**: Route to appropriate folder
4. **Set Flags**: Apply message flags based on filters
5. **Store Message**: Save to MongoDB with metadata
6. **Update Quotas**: Track storage usage

## Filter System

Supports various filter criteria:

- **Header Matching**: Filter based on email headers
- **Content Filtering**: Text-based content matching
- **Size Filtering**: Message size constraints
- **Attachment Detection**: Presence of attachments
- **Spam Detection**: Integration with spam headers

Filter actions include:

- **Mailbox Routing**: Move to specific folder
- **Flag Setting**: Mark as seen, flagged, etc.
- **Message Deletion**: Reject and delete
- **Spam Handling**: Route to Junk folder

## Installation & Usage

1. **Build the server**:
```bash
cd lmtp
go build -o lmtp-server main.go
```

2. **Start the server**:
```bash
./lmtp-server
```

3. **Test with client**:
```bash
go run test_lmtp.go
```

## Testing

The package includes test utilities:

- **LMTPClient**: Simple LMTP client for testing
- **Test Message Creator**: Generate test emails
- **Test Script**: Automated testing workflow

Example test usage:
```go
client, err := lmtp.NewLMTPClient("localhost", 2003)
if err != nil {
    log.Fatal(err)
}
defer client.Close()

message := lmtp.CreateTestMessage(
    "sender@example.com",
    "recipient@localhost", 
    "Test Subject",
    "Message body content",
)

err = client.SendMail(
    "sender@example.com",
    []string{"recipient@localhost"},
    message,
)
```

## Integration

The LMTP server integrates with:

- **Mail API**: Uses same MongoDB schema
- **IMAP Server**: Messages stored for IMAP access
- **User Management**: Validates against user database
- **Mailbox System**: Routes to existing mailboxes

## Database Schema

Uses the same MongoDB collections as the main mail API:

- **users**: User accounts and settings
- **addresses**: Email address mappings
- **mailboxes**: User mailbox folders  
- **messages**: Email message storage

## Error Handling

LMTP-specific error responses:

- **550**: Permanent failure (unknown recipient, disabled user)
- **450**: Temporary failure (database error, quota exceeded)
- **250**: Success (message accepted)

Per-recipient responses ensure accurate delivery status reporting.

## Performance Considerations

- **Connection Pooling**: Efficient MongoDB connections
- **Concurrent Processing**: Handle multiple LMTP connections
- **Memory Management**: Stream large messages efficiently
- **Index Optimization**: Proper database indexing for lookups

## Security Features

- **Address Validation**: Prevent relay abuse
- **User Authorization**: Verify recipient permissions
- **Resource Limits**: Message size and connection limits
- **Input Sanitization**: Protect against injection attacks

## Logging

Comprehensive logging includes:

- Connection events and errors
- Message processing steps
- Filter matching results
- Database operations
- Performance metrics

## Future Enhancements

- **TLS Support**: Encrypted LMTP connections
- **Authentication**: SMTP AUTH for relay
- **Rate Limiting**: Per-user delivery limits
- **Delivery Status**: Enhanced status reporting
- **Clustering**: Multi-server deployment support

## Troubleshooting

Common issues and solutions:

1. **Connection Refused**: Check server is running and port is open
2. **Unknown Recipient**: Verify user/address exists in database
3. **Database Errors**: Check MongoDB connection and permissions
4. **Message Rejected**: Review filter rules and spam settings
5. **Storage Issues**: Check disk space and quotas

## License

MIT License - see LICENSE file for details.