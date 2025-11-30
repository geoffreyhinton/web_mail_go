package imapcore

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"time"
)

// ServerOptions alias for compatibility
type ServerOptions = IMAPServerOptions

// IMAPResponse represents a response to an IMAP command
type IMAPResponse struct {
	Tag     string
	Status  string
	Command string
	Text    string
	Items   []interface{}
}

// NewIMAPServer creates a new IMAP server instance
func NewIMAPServer(options *ServerOptions) (*IMAPServer, error) {
	if options.Logger == nil {
		return nil, fmt.Errorf("logger is required")
	}
	if options.Database == nil {
		return nil, fmt.Errorf("database is required")
	}

	server := &IMAPServer{
		options:  options,
		sessions: make(map[string]*Session),
		quit:     make(chan struct{}),
		done:     make(chan struct{}),
	}

	return server, nil
}

// Start begins listening for IMAP connections
func (s *IMAPServer) Start() error {
	addr := fmt.Sprintf("%s:%d", s.options.Host, s.options.Port)

	var err error
	if s.options.Secure && s.options.TLSConfig != nil {
		s.listener, err = tls.Listen("tcp", addr, s.options.TLSConfig)
	} else {
		s.listener, err = net.Listen("tcp", addr)
	}

	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	s.options.Logger.Info("IMAP server listening on %s", addr)

	for {
		select {
		case <-s.quit:
			return nil
		default:
		}

		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.quit:
				return nil
			default:
				s.options.Logger.Error("Failed to accept connection: %v", err)
				continue
			}
		}

		go s.handleConnection(conn)
	}
}

// Shutdown gracefully shuts down the server
func (s *IMAPServer) Shutdown(ctx context.Context) error {
	close(s.quit)

	if s.listener != nil {
		if err := s.listener.Close(); err != nil {
			s.options.Logger.Error("Error closing listener: %v", err)
		}
	}

	// Close all active sessions
	s.mutex.RLock()
	for _, session := range s.sessions {
		session.close()
	}
	s.mutex.RUnlock()

	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// handleConnection handles a new client connection
func (s *IMAPServer) handleConnection(conn net.Conn) {
	defer conn.Close()

	sessionID := generateSessionID()
	session := &Session{
		ID:           sessionID,
		conn:         conn,
		scanner:      bufio.NewScanner(conn),
		writer:       bufio.NewWriter(conn),
		server:       s,
		state:        StateNotAuthenticated,
		capabilities: s.getCapabilities(),
	}

	s.mutex.Lock()
	s.sessions[sessionID] = session
	s.mutex.Unlock()

	defer func() {
		s.mutex.Lock()
		delete(s.sessions, sessionID)
		s.mutex.Unlock()
	}()

	s.options.Logger.Info("[%s] New connection from %s", sessionID, conn.RemoteAddr())

	// Send greeting
	session.writeResponse("*", "OK", "IMAP4rev1 Server Ready")

	// Handle commands
	for session.scanner.Scan() {
		line := strings.TrimSpace(session.scanner.Text())
		if line == "" {
			continue
		}

		s.options.Logger.Debug("[%s] C: %s", sessionID, line)

		if err := session.processCommand(line); err != nil {
			s.options.Logger.Error("[%s] Command processing error: %v", sessionID, err)
			session.writeResponse("*", "BAD", "Internal server error")
			break
		}

		if session.state == StateLogout {
			break
		}
	}

	if err := session.scanner.Err(); err != nil {
		s.options.Logger.Error("[%s] Scanner error: %v", sessionID, err)
	}

	s.options.Logger.Info("[%s] Connection closed", sessionID)
}

// processCommand processes an IMAP command
func (s *Session) processCommand(line string) error {
	parts := strings.SplitN(line, " ", 3)
	if len(parts) < 2 {
		s.writeResponse("*", "BAD", "Invalid command format")
		return nil
	}

	tag := parts[0]
	command := strings.ToUpper(parts[1])
	var args string
	if len(parts) > 2 {
		args = parts[2]
	}

	s.server.options.Logger.Debug("[%s] Processing command: %s %s", s.ID, command, args)

	switch command {
	case "CAPABILITY":
		return s.handleCapability(tag)
	case "NOOP":
		return s.handleNoop(tag)
	case "LOGOUT":
		return s.handleLogout(tag)
	case "STARTTLS":
		return s.handleStartTLS(tag)
	case "LOGIN":
		return s.handleLogin(tag, args)
	case "AUTHENTICATE":
		return s.handleAuthenticate(tag, args)
	case "LIST":
		return s.handleList(tag, args)
	case "LSUB":
		return s.handleLsub(tag, args)
	case "SELECT":
		return s.handleSelect(tag, args)
	case "EXAMINE":
		return s.handleExamine(tag, args)
	case "CREATE":
		return s.handleCreate(tag, args)
	case "DELETE":
		return s.handleDelete(tag, args)
	case "RENAME":
		return s.handleRename(tag, args)
	case "SUBSCRIBE":
		return s.handleSubscribe(tag, args)
	case "UNSUBSCRIBE":
		return s.handleUnsubscribe(tag, args)
	case "STATUS":
		return s.handleStatus(tag, args)
	case "APPEND":
		return s.handleAppend(tag, args)
	case "FETCH":
		return s.handleFetch(tag, args)
	case "STORE":
		return s.handleStore(tag, args)
	case "COPY":
		return s.handleCopy(tag, args)
	case "MOVE":
		return s.handleMove(tag, args)
	case "SEARCH":
		return s.handleSearch(tag, args)
	case "EXPUNGE":
		return s.handleExpunge(tag)
	case "CLOSE":
		return s.handleClose(tag)
	case "GETQUOTAROOT":
		return s.handleGetQuotaRoot(tag, args)
	case "GETQUOTA":
		return s.handleGetQuota(tag, args)
	case "UID":
		// Handle UID commands
		if args == "" {
			return s.writeResponse(tag, "BAD", "UID command requires subcommand")
		}

		uidParts := strings.SplitN(args, " ", 2)
		uidCommand := strings.ToUpper(uidParts[0])
		uidArgs := ""
		if len(uidParts) > 1 {
			uidArgs = uidParts[1]
		}

		switch uidCommand {
		case "FETCH":
			return s.handleUIDFetch(tag, uidArgs)
		case "STORE":
			return s.handleUIDStore(tag, uidArgs)
		case "COPY":
			return s.handleUIDCopy(tag, uidArgs)
		case "SEARCH":
			return s.handleUIDSearch(tag, uidArgs)
		default:
			return s.writeResponse(tag, "BAD", "Unknown UID command")
		}
	default:
		s.writeResponse(tag, "BAD", fmt.Sprintf("Unknown command: %s", command))
	}

	return nil
}

// close closes the session
func (s *Session) close() {
	if s.conn != nil {
		s.conn.Close()
	}
}

// getCapabilities returns the server capabilities
func (s *IMAPServer) getCapabilities() []string {
	caps := []string{
		"IMAP4rev1",
		"LITERAL+",
		"SASL-IR",
		"LOGIN-REFERRALS",
		"ID",
		"ENABLE",
		"IDLE",
		"NAMESPACE",
		"MAILBOX-REFERRALS",
		"BINARY",
		"UNSELECT",
		"ESEARCH",
		"WITHIN",
		"CONTEXT=SEARCH",
		"LIST-EXTENDED",
		"CHILDREN",
		"LIST-STATUS",
		"QUOTA",
	}

	if !s.options.IgnoreSTARTTLS && s.options.TLSConfig != nil {
		caps = append(caps, "STARTTLS")
	}

	return caps
}

// generateSessionID generates a unique session ID
func generateSessionID() string {
	return fmt.Sprintf("sess_%d_%d", time.Now().UnixNano(), rand.Intn(10000))
}

// Utility function for random numbers (simplified)
var rand = struct {
	Intn func(int) int
}{
	Intn: func(n int) int {
		return int(time.Now().UnixNano()) % n
	},
}
