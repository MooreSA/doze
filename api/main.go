// Package main implements a web API server for managing Claude Code sessions.
//
// Doze provides a remote interface to Claude Code with automatic idle detection
// and session management. The server spawns Claude Code processes, streams their
// output via Server-Sent Events (SSE), and handles bidirectional communication
// using Claude's stream-json format.
//
// Architecture:
//   - Session state is managed in-memory (single session for MVP)
//   - Claude Code runs as a child process
//   - Output is buffered in a ring buffer for reconnection
//   - SSE streams real-time output to connected clients
//   - Idle detection automatically stops inactive sessions
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Constants for configuration and magic strings
const (
	// Server defaults
	DefaultPort    = "2020"
	DefaultWebPath = "../web/dist"

	// Buffer sizes
	RingBufferSize       = 10 * 1024   // 10KB ring buffer for output
	ScannerInitialBuffer = 64 * 1024   // 64KB initial scanner buffer
	ScannerMaxBuffer     = 1024 * 1024 // 1MB max scanner buffer
	SSEClientBufferSize  = 100         // Number of events to buffer per SSE client
	StderrReadBufferSize = 1024        // Buffer size for stderr reads

	// Timeouts
	DefaultIdleTimeout      = 30 * time.Second // Idle timeout before stopping session
	GracefulShutdownTimeout = 10 * time.Second // Time to wait for SIGTERM before SIGKILL

	// Message types from Claude stream-json
	MessageTypeAssistant = "assistant"
	MessageTypeUser      = "user"
	MessageTypeResult    = "result"
	MessageTypeError     = "error"
	MessageTypeSystem    = "system"

	// Content block types
	ContentTypeText    = "text"
	ContentTypeToolUse = "tool_use"

	// SSE event types
	EventTypeOutput      = "output"
	EventTypeState       = "state"
	EventTypeError       = "error"
	EventTypeInfo        = "info"
	EventTypeFileChanges = "file_changes"
	EventTypeToolUse     = "tool_use"

	// Display limits
	StatusRecentOutputLimit = 500 // Characters of recent output to include in status endpoint

	// Security settings
	// Set to true for remote/production use (no permission prompts)
	// Set to false for local development (safer, allows manual approval)
	SkipPermissions = true
)

// SessionState represents the current state of a Claude Code session.
//
// State transitions follow this flow:
//
//	StateNone → StateStarting → StateWaiting ⇄ StateActive → StateShuttingDown → StateStopped
//	                               ↑              ↓
//	                               └──────────────┘
//
// - Process starts in StateWaiting (ready for first input, idle timer starts)
// - User message → StateActive (Claude processing)
// - Response complete → StateWaiting (ready for next input)
// - Idle timeout in StateWaiting → StateShuttingDown → StateStopped
type SessionState string

const (
	// StateNone indicates no session has been started yet.
	StateNone SessionState = "none"

	// StateStarting indicates the Claude Code process is being spawned.
	StateStarting SessionState = "starting"

	// StateActive indicates Claude is actively processing and generating output.
	StateActive SessionState = "active"

	// StateWaiting indicates Claude has finished responding and is waiting for user input.
	// The idle timer starts when entering this state.
	StateWaiting SessionState = "waiting"

	// StateShuttingDown indicates the session is in the process of shutting down due to idle timeout.
	// A SIGTERM has been sent to the Claude process.
	StateShuttingDown SessionState = "shutting_down"

	// StateStopped indicates the session has been stopped and the process has exited.
	// Can be resumed later by spawning a new process with --resume {session-id}.
	StateStopped SessionState = "stopped"
)

// SSEClient represents a connected Server-Sent Events (SSE) client.
//
// Each client receives real-time updates about Claude's output and state changes.
// The events channel is buffered to prevent slow clients from blocking broadcasts.
type SSEClient struct {
	id     string        // Unique identifier for this client (timestamp-based)
	events chan SSEEvent // Buffered channel for events to send to this client
	done   chan struct{} // Closed when the client disconnects
}

// SSEEvent is an event to send to SSE clients.
//
// Events are JSON-encoded and sent over the SSE connection. The Type field
// determines which other fields are populated:
//   - "output": Content contains Claude's output text
//   - "state": State contains the new SessionState
//   - "error": Content contains an error message
//   - "info": Content contains informational text
type SSEEvent struct {
	Type    string `json:"type"`              // Event type: "output", "state", "error", "info"
	Content string `json:"content,omitempty"` // Text content for output/error/info events
	State   string `json:"state,omitempty"`   // SessionState for state change events
}

// RingBuffer is a thread-safe circular buffer for keeping recent output.
//
// When the buffer fills up, new writes overwrite the oldest data. This allows
// reconnecting clients to see recent output without storing unbounded history.
// Typically sized at 10KB for the MVP.
type RingBuffer struct {
	data     []byte     // Fixed-size buffer for data
	size     int        // Total capacity of the buffer
	writePos int        // Current write position (wraps around)
	full     bool       // True once we've wrapped around at least once
	mu       sync.Mutex // Protects concurrent access
}

// NewRingBuffer creates a new ring buffer with the specified size in bytes.
func NewRingBuffer(size int) *RingBuffer {
	return &RingBuffer{
		data: make([]byte, size),
		size: size,
	}
}

// Write implements io.Writer, appending data to the ring buffer.
//
// If the buffer is full, old data is overwritten. Always returns len(p), nil
// to satisfy the io.Writer interface (never fails).
func (rb *RingBuffer) Write(p []byte) (n int, err error) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	// Write each byte, wrapping around when necessary
	for _, b := range p {
		rb.data[rb.writePos] = b
		rb.writePos = (rb.writePos + 1) % rb.size
		if rb.writePos == 0 {
			rb.full = true
		}
	}
	return len(p), nil
}

// String returns the current contents of the ring buffer as a string.
//
// If the buffer hasn't filled yet, returns data from start to writePos.
// If the buffer has wrapped, reconstructs the data in chronological order
// (oldest to newest), which means reading from writePos to end, then start to writePos.
func (rb *RingBuffer) String() string {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if !rb.full {
		// Buffer not yet full, return what we have
		return string(rb.data[:rb.writePos])
	}

	// Buffer has wrapped - reconstruct in order (oldest to newest)
	// Oldest data starts at writePos, wraps around to writePos-1
	result := make([]byte, rb.size)
	copy(result, rb.data[rb.writePos:])                       // Copy from writePos to end
	copy(result[rb.size-rb.writePos:], rb.data[:rb.writePos]) // Copy from start to writePos
	return string(result)
}

// Session holds all state for a Claude Code session.
//
// This includes the running process, communication channels, output buffering,
// connected SSE clients, and idle detection state. The session is protected by
// multiple locks for different concerns:
//   - mu: Protects session state and timestamps
//   - sseMu: Protects the sseClients map (separate to avoid deadlocks during broadcasts)
//
// For the MVP, only a single global session is supported.
type Session struct {
	mu sync.RWMutex // Protects State, ClaudeSessionID, RepoPath, timestamps, and process fields

	// State tracking
	State           SessionState // Current state of the session
	ClaudeSessionID string       // Session ID from Claude Code (used for --resume)
	RepoPath        string       // Working directory for the Claude process
	LastActivity    time.Time    // Last time user sent a message or Claude produced output
	LastOutputAt    time.Time    // Last time Claude produced output (for timeout detection)

	// Process management
	cmd    *exec.Cmd      // The running Claude Code process
	stdin  io.WriteCloser // Pipe to send messages to Claude
	stdout io.ReadCloser  // Pipe to read Claude's JSON output
	stderr io.ReadCloser  // Pipe to read Claude's error output

	// Output handling
	outputBuffer *RingBuffer           // Circular buffer of recent output for reconnecting clients
	sseClients   map[string]*SSEClient // Connected SSE clients by ID
	sseMu        sync.RWMutex          // Protects sseClients map

	// Idle detection
	idleTimer   *time.Timer   // Timer that fires when idle timeout is reached
	idleTimeout time.Duration // How long to wait before stopping session (default: 3 minutes)
}

// Global session (single session for MVP).
// Protected by sessionMu during start/stop operations.
var session *Session
var sessionMu sync.Mutex

// Removed sprite client - running locally on sprite now

// respondJSON sends a JSON response with the given status code.
// Logs any errors that occur during encoding.
func respondJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if statusCode != http.StatusOK {
		w.WriteHeader(statusCode)
	}
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("failed to encode json response",
			"status_code", statusCode,
			"error", err)
	}
}

// respondError sends a JSON error response with the given status code and message.
func respondError(w http.ResponseWriter, statusCode int, message string) {
	respondJSON(w, statusCode, map[string]interface{}{
		"error": message,
	})
}

func main() {
	// Get port from environment or default
	port := os.Getenv("PORT")
	if port == "" {
		port = DefaultPort
	}

	// Set up structured logging
	// Use text handler for development, can switch to JSON for production
	logHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo, // Change to LevelDebug for verbose logging
	})
	slog.SetDefault(slog.New(logHandler))

	slog.Info("doze api server starting", "port", port)

	// Initialize global session with defaults
	session = &Session{
		State:        StateNone,
		outputBuffer: NewRingBuffer(RingBufferSize),
		sseClients:   make(map[string]*SSEClient),
		idleTimeout:  DefaultIdleTimeout, // TODO: Make configurable via env/config
	}

	// API endpoints
	http.HandleFunc("/health", handleHealth)   // GET: Health check endpoint
	http.HandleFunc("/status", handleStatus)   // GET: Check session status
	http.HandleFunc("/start", handleStart)     // POST: Start a new Claude session
	http.HandleFunc("/stream", handleStream)   // GET: SSE stream of output and state
	http.HandleFunc("/message", handleMessage) // POST: Send a message to Claude
	http.HandleFunc("/diff", handleDiff)       // GET: Get git diff for a specific file

	// Serve web UI
	http.HandleFunc("/", handleIndex)

	// Set up graceful shutdown
	server := &http.Server{
		Addr:    ":" + port,
		Handler: http.DefaultServeMux,
	}

	// Channel to listen for interrupt signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	// Start server in a goroutine
	go func() {
		slog.Info("server listening", "port", port, "endpoints", []string{"/health", "/status", "/start", "/stream", "/message"})
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal
	sig := <-sigChan
	slog.Info("received shutdown signal", "signal", sig)

	// Gracefully stop Claude session if running
	sessionMu.Lock()
	if session.State == StateWaiting || session.State == StateActive {
		slog.Info("stopping claude session before shutdown")
		stopSession()
	}
	sessionMu.Unlock()

	// Shutdown HTTP server with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		slog.Error("server shutdown error", "error", err)
		os.Exit(1)
	}

	slog.Info("server shutdown complete")
}

// handleHealth returns a simple health check response.
//
// GET /health
//
// Always returns 200 OK with {"status": "ok"} if the server is running.
// Used by load balancers, Docker health checks, and monitoring tools.
func handleHealth(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

// handleStatus returns the current session status as JSON.
//
// GET /status
//
// Response includes:
//   - state: Current SessionState
//   - claude_session_id: Session ID for --resume (empty if not captured yet)
//   - repo_path: Working directory of the Claude process
//   - last_activity: Timestamp of last user message or Claude output
//   - idle_seconds: Seconds since last activity
//   - recent_output: Last 500 chars from the output buffer
func handleStatus(w http.ResponseWriter, r *http.Request) {
	session.mu.RLock()
	defer session.mu.RUnlock()

	status := map[string]interface{}{
		"state":             session.State,
		"claude_session_id": session.ClaudeSessionID,
		"repo_path":         session.RepoPath,
		"last_activity":     session.LastActivity,
		"idle_seconds":      0,
	}

	// Calculate idle time if session has been active
	if !session.LastActivity.IsZero() {
		status["idle_seconds"] = int(time.Since(session.LastActivity).Seconds())
	}

	// Include recent output for status endpoint
	recentOutput := session.outputBuffer.String()
	if len(recentOutput) > StatusRecentOutputLimit {
		recentOutput = recentOutput[len(recentOutput)-StatusRecentOutputLimit:]
	}
	status["recent_output"] = recentOutput

	respondJSON(w, http.StatusOK, status)
}

// handleStart starts a new Claude Code session.
//
// POST /start
// Request body (optional):
//
//	{
//	  "repo_path": "/path/to/repo"  // Optional, uses REPO_PATH env or default
//	}
//
// Response on success:
//
//	{
//	  "success": true,
//	  "state": "active",
//	  "repo_path": "/path/to/repo"
//	}
//
// Response on error:
//
//	{
//	  "error": "session already active",
//	  "state": "active"
//	}
func handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessionMu.Lock()
	defer sessionMu.Unlock()

	// Check if session already running
	if session.State != StateNone && session.State != StateStopped {
		respondJSON(w, http.StatusConflict, map[string]interface{}{
			"error": "session already active",
			"state": session.State,
		})
		return
	}

	// Parse request for optional repo path
	var req struct {
		RepoPath string `json:"repo_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("failed to decode start request body", "error", err)
		// Continue with empty req - repo path is optional
	}

	// Determine repo path: request > env > current directory
	repoPath := req.RepoPath
	if repoPath == "" {
		repoPath = os.Getenv("REPO_PATH")
	}
	if repoPath == "" {
		var err error
		repoPath, err = os.Getwd()
		if err != nil {
			slog.Error("failed to get current directory", "error", err)
			respondError(w, http.StatusInternalServerError, "failed to determine working directory")
			return
		}
	}

	// Expand tilde in path (e.g., ~/code -> /home/user/code)
	if strings.HasPrefix(repoPath, "~/") {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			repoPath = filepath.Join(homeDir, repoPath[2:])
		}
	}

	// Start Claude Code process
	if err := startClaudeProcess(repoPath); err != nil {
		slog.Error("failed to start claude process", "error", err)
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"success":   true,
		"state":     session.State,
		"repo_path": session.RepoPath,
	})
}

// getRepoURL returns the git repository URL to clone
// For MVP, hardcoded to doze repo. Future: make configurable.
// startClaudeProcess spawns a new Claude Code process with stream-json I/O.
//
// The process is started in the specified repoPath directory. Uses Claude's
// stream-json format for bidirectional communication:
//   - Input: JSON objects sent to stdin, one per line
//   - Output: JSON objects from stdout, one per line
//
// Launches three goroutines to handle:
//   - handleStdout: Parses JSON output and broadcasts to SSE clients
//   - handleStderr: Captures diagnostic output
//   - waitForExit: Handles process termination
//
// The session.mu lock must NOT be held when calling this function, as it
// acquires the lock itself and spawns goroutines that also need it.
// startClaudeProcess spawns a new Claude Code process locally using exec.Command
func startClaudeProcess(repoPath string) error {
	session.mu.Lock()
	defer session.mu.Unlock()

	session.State = StateStarting
	session.RepoPath = repoPath
	session.LastActivity = time.Now()

	// Broadcast state change to connected clients
	broadcastState(StateStarting)

	// Build command - use stream-json for bidirectional streaming
	// --print: Show output (don't suppress)
	// --input-format=stream-json: Accept JSON messages on stdin
	// --output-format=stream-json: Emit JSON messages on stdout
	// --verbose: Include session metadata in output
	args := []string{
		"--print",
		"--input-format=stream-json",
		"--output-format=stream-json",
		"--verbose",
	}
	if SkipPermissions {
		args = append(args, "--dangerously-skip-permissions")
	}
	cmd := exec.Command("claude", args...)
	cmd.Dir = repoPath

	// Get pipes for stdin/stdout/stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		session.State = StateNone
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		session.State = StateNone
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		session.State = StateNone
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	session.cmd = cmd
	session.stdin = stdin
	session.stdout = stdout
	session.stderr = stderr

	// Start the process
	if err := cmd.Start(); err != nil {
		session.State = StateNone
		return fmt.Errorf("failed to start claude: %w", err)
	}

	slog.Info("claude process started (local)", "pid", cmd.Process.Pid, "repo_path", repoPath)

	// Start goroutines to handle I/O (these run until process exits)
	go handleStdout()
	go handleStderr()
	go waitForExit()

	// Claude starts in waiting state (ready for first input)
	// StateActive is only when Claude is actively processing
	session.State = StateWaiting
	broadcastState(StateWaiting)

	// Start idle timer immediately - session will stop if no activity
	resetIdleTimer()
	slog.Info("session ready", "state", StateWaiting, "idle_timeout", session.idleTimeout)

	return nil
}

// startClaudeProcessOnSprite spawns a new Claude Code process on a Sprite
// startClaudeProcessWithMessage spawns a new Claude Code process and sends an initial message.
//
// Similar to startClaudeProcess, but transitions directly to StateActive and sends
// the provided message immediately after the process starts. This allows users to
// "wake up" the application by sending a message directly from the welcome screen.
//
// The session.mu lock must NOT be held when calling this function.
func startClaudeProcessWithMessage(repoPath, initialMessage string) error {
	// Start the process first
	if err := startClaudeProcess(repoPath); err != nil {
		return err
	}

	// Now send the initial message
	session.mu.Lock()
	stdin := session.stdin
	session.State = StateActive
	session.mu.Unlock()

	broadcastState(StateActive)

	inputMsg := map[string]interface{}{
		"type": MessageTypeUser,
		"message": map[string]interface{}{
			"role":    MessageTypeUser,
			"content": initialMessage,
		},
	}
	msgBytes, err := json.Marshal(inputMsg)
	if err != nil {
		slog.Error("failed to marshal initial message", "error", err)
		return fmt.Errorf("failed to format initial message: %w", err)
	}

	// Write JSON message to stdin
	_, err = fmt.Fprintf(stdin, "%s\n", msgBytes)
	if err != nil {
		slog.Error("failed to write initial message to stdin", "error", err)
		return fmt.Errorf("failed to send initial message: %w", err)
	}

	slog.Info("initial message sent", "message", initialMessage)

	return nil
}

// resumeClaudeProcess resumes a stopped Claude Code session with --resume.
//
// Similar to startClaudeProcess, but uses the stored session ID to resume
// a previous conversation. The queued message is sent immediately after the
// process starts.
//
// If resume fails (e.g., session ID not found), the process will exit quickly
// and waitForExit will handle the error state.
//
// The session.mu lock must NOT be held when calling this function.
// Resumes a stopped Claude Code session with --resume.
func resumeClaudeProcess(queuedMessage string) error {
	session.mu.Lock()
	defer session.mu.Unlock()

	if session.ClaudeSessionID == "" {
		return fmt.Errorf("no session ID available for resume")
	}

	sessionID := session.ClaudeSessionID
	repoPath := session.RepoPath
	if repoPath == "" {
		// Fallback to current directory if session.RepoPath wasn't set
		var err error
		repoPath, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
	}

	session.State = StateStarting
	session.LastActivity = time.Now()

	broadcastState(StateStarting)

	// Build command with --resume flag
	args := []string{
		"--resume", sessionID,
		"--print",
		"--input-format=stream-json",
		"--output-format=stream-json",
		"--verbose",
	}
	if SkipPermissions {
		args = append(args, "--dangerously-skip-permissions")
	}
	cmd := exec.Command("claude", args...)
	cmd.Dir = repoPath

	// Get pipes for stdin/stdout/stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		session.State = StateStopped
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		session.State = StateStopped
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		session.State = StateStopped
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	session.cmd = cmd
	session.stdin = stdin
	session.stdout = stdout
	session.stderr = stderr

	// Start the process
	if err := cmd.Start(); err != nil {
		session.State = StateStopped
		return fmt.Errorf("failed to resume claude: %w", err)
	}

	slog.Info("claude process resumed", "pid", cmd.Process.Pid, "session_id", sessionID, "repo_path", repoPath)

	// Start goroutines to handle I/O (these run until process exits)
	go handleStdout()
	go handleStderr()
	go waitForExit()

	// Transition to active state (we're about to send a message)
	session.State = StateActive
	broadcastState(StateActive)

	// Send the queued message immediately
	// Claude will buffer it if not quite ready yet
	inputMsg := map[string]interface{}{
		"type": MessageTypeUser,
		"message": map[string]interface{}{
			"role":    MessageTypeUser,
			"content": queuedMessage,
		},
	}
	msgBytes, err := json.Marshal(inputMsg)
	if err != nil {
		slog.Error("failed to marshal queued message", "error", err)
		return fmt.Errorf("failed to format queued message: %w", err)
	}

	// Write to stdin in a goroutine to avoid blocking
	go func() {
		if _, err := fmt.Fprintf(stdin, "%s\n", msgBytes); err != nil {
			slog.Error("failed to write queued message to stdin", "error", err)
		}
		slog.Info("queued message sent to resumed session")
	}()

	return nil
}

// resumeClaudeProcessOnSprite resumes a stopped Claude Code session on a Sprite with --resume.
// ClaudeStreamMessage represents a message from Claude's stream-json output.
//
// Claude emits several message types:
//   - "assistant": Contains Claude's response text in Message.Content
//   - "result": Indicates completion of a response (SessionID captured here)
//   - "error": Contains error information in Result
//   - "system": Internal messages (not shown to user)
//   - "user": Echo of user input (including tool results and replay messages)
//
// The SessionID field is critical for resume functionality - it's captured
// and stored in session.ClaudeSessionID for later use with --resume.
//
// The Message field uses json.RawMessage to handle different structures:
//   - For "assistant" messages: {content: [{type, text}]}
//   - For "user" messages: {role: "user", content: "text"}
type ClaudeStreamMessage struct {
	Type      string          `json:"type"`                 // Message type: "assistant", "result", "error", "system", "user"
	SessionID string          `json:"session_id,omitempty"` // Session ID for --resume (appears in result messages)
	Result    string          `json:"result,omitempty"`     // Final result text or error message
	Message   json.RawMessage `json:"message,omitempty"`    // Raw message data (structure varies by type)
}

// ContentBlock represents a block of content in a message.
//
// Can be either:
//   - Text block: Type="text", Text contains the response
//   - Tool use block: Type="tool_use", ID/Name/Input contain tool call details
type ContentBlock struct {
	Type  string                 `json:"type"`            // "text" or "tool_use"
	Text  string                 `json:"text,omitempty"`  // For text blocks
	ID    string                 `json:"id,omitempty"`    // For tool_use blocks
	Name  string                 `json:"name,omitempty"`  // For tool_use blocks (e.g., "Read", "Bash")
	Input map[string]interface{} `json:"input,omitempty"` // For tool_use blocks (tool parameters)
}

// handleStdout reads and processes Claude's stdout stream.
//
// This goroutine runs for the lifetime of the Claude process. It:
//  1. Reads line-by-line (each line is a JSON message)
//  2. Parses stream-json messages
//  3. Extracts and broadcasts Claude's response text
//  4. Captures the session ID for --resume
//  5. Detects when Claude finishes and transitions to StateWaiting
//  6. Starts the idle timer when waiting for input
//
// Message types handled:
//   - "assistant": Claude's response text (broadcast to clients)
//   - "result": Response complete, transition to StateWaiting and start idle timer
//   - "error": Error messages (broadcast with [Error] prefix)
//   - "system": Internal messages (logged only, not shown to user)
//
// The scanner buffer is increased to handle large JSON messages (up to 1MB).
func handleStdout() {
	scanner := bufio.NewScanner(session.stdout)
	// Increase buffer size for large JSON messages (Claude can send big responses)
	buf := make([]byte, ScannerInitialBuffer)
	scanner.Buffer(buf, ScannerMaxBuffer)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// Update activity timestamps
		session.mu.Lock()
		session.LastOutputAt = time.Now()
		session.LastActivity = time.Now()
		session.mu.Unlock()

		// Try to parse as JSON
		var msg ClaudeStreamMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			// Not JSON, broadcast as raw output (shouldn't happen with stream-json)
			slog.Warn("received non-json stdout", "content", line)
			session.mu.Lock()
			if _, writeErr := session.outputBuffer.Write([]byte(line + "\n")); writeErr != nil {
				slog.Error("failed to write to output buffer", "error", writeErr)
			}
			session.mu.Unlock()
			broadcastOutput(line + "\n")
			continue
		}

		// Handle different message types
		var content string
		switch msg.Type {
		case MessageTypeAssistant:
			// Parse the message content (assistant messages have content blocks)
			var assistantMsg struct {
				Content []ContentBlock `json:"content"`
			}
			if err := json.Unmarshal(msg.Message, &assistantMsg); err != nil {
				slog.Warn("failed to parse assistant message", "error", err)
				continue
			}

			// Extract content from message content blocks
			for _, c := range assistantMsg.Content {
				switch c.Type {
				case ContentTypeText:
					content += c.Text
				case ContentTypeToolUse:
					// Tool usage - broadcast structured data for rich display
					slog.Debug("tool use detected", "tool", c.Name, "input", c.Input)

					// Send structured tool use event
					toolData := map[string]interface{}{
						"tool":  c.Name,
						"input": c.Input,
					}
					toolJSON, err := json.Marshal(toolData)
					if err == nil {
						broadcastEvent(SSEEvent{Type: EventTypeToolUse, Content: string(toolJSON)})
					}

					// Also send formatted message for backward compatibility
					toolMsg := formatToolUse(c.Name, c.Input)
					broadcastEvent(SSEEvent{Type: EventTypeInfo, Content: toolMsg})

					// Track file edits in real-time
					trackFileEdit(c.Name, c.Input)
				}
			}
			// Capture session ID if present (needed for --resume)
			if msg.SessionID != "" {
				session.mu.Lock()
				session.ClaudeSessionID = msg.SessionID
				session.mu.Unlock()
				slog.Info("captured session id", "session_id", msg.SessionID)
			}

		case MessageTypeResult:
			// Result indicates Claude has finished responding
			// Don't output the result text (already shown via assistant messages)
			if msg.SessionID != "" {
				session.mu.Lock()
				session.ClaudeSessionID = msg.SessionID
				session.mu.Unlock()
			}
			// Transition to waiting state and start idle timer
			session.mu.Lock()
			if session.State == StateActive {
				session.State = StateWaiting
				slog.Info("state transition", "from", StateActive, "to", StateWaiting, "reason", "response_complete")
				go broadcastState(StateWaiting)
				go detectAndBroadcastFileChanges() // Check for git changes
				resetIdleTimer()                   // Start countdown to session stop
			}
			session.mu.Unlock()
			continue // Don't output the result text

		case MessageTypeUser:
			// Echo of user input (including tool results) - don't show this to user
			// This is Claude Code echoing back our input, not user-facing content
			continue

		case MessageTypeError:
			content = "[Error] " + msg.Result

		case MessageTypeSystem:
			// System messages are for debugging, not user-facing
			slog.Debug("system message received", "content", line)
			continue

		default:
			// Log unknown types for debugging (helps if Claude adds new message types)
			slog.Warn("unknown message type", "type", msg.Type, "raw", line)
			continue
		}

		// Broadcast non-empty content to all connected SSE clients
		if content != "" {
			session.mu.Lock()
			if _, err := session.outputBuffer.Write([]byte(content)); err != nil {
				slog.Error("failed to write to output buffer", "error", err)
			}
			session.mu.Unlock()
			broadcastOutput(content)
		}
	}

	// Scanner error handling (usually EOF when process exits)
	if err := scanner.Err(); err != nil {
		slog.Error("stdout scanner error", "error", err)
	}
}

// handleStderr reads and processes Claude's stderr stream.
//
// This goroutine runs for the lifetime of the Claude process. Stderr typically
// contains diagnostic output, progress indicators, and error messages that are
// part of the terminal experience. All stderr output is:
//  1. Logged to the server console
//  2. Added to the output buffer
//  3. Broadcast to SSE clients
//
// This ensures users see the same output they would see in a terminal.
func handleStderr() {
	reader := bufio.NewReader(session.stderr)
	buf := make([]byte, StderrReadBufferSize)

	for {
		n, err := reader.Read(buf)
		if err != nil {
			if err != io.EOF {
				slog.Error("stderr read error", "error", err)
			}
			return
		}

		if n > 0 {
			output := string(buf[:n])
			slog.Debug("stderr output", "content", output)

			// Broadcast stderr as output (users expect to see this in the terminal)
			session.mu.Lock()
			if _, err := session.outputBuffer.Write(buf[:n]); err != nil {
				slog.Error("failed to write stderr to output buffer", "error", err)
			}
			session.mu.Unlock()

			broadcastOutput(output)
		}
	}
}

// waitForExit waits for the Claude process to terminate and handles cleanup.
//
// This goroutine blocks on cmd.Wait() until the process exits. It then:
//  1. Logs the exit status
//  2. Transitions state based on why we exited:
//     - StateShuttingDown → StateStopped (expected shutdown)
//     - Any other state → StateNone (unexpected crash)
//  3. Cleans up process handles
//
// If the exit was unexpected, broadcasts an error event to connected clients.
func waitForExit() {
	// Wait for Claude process to exit
	err := session.cmd.Wait()

	session.mu.Lock()
	defer session.mu.Unlock()

	if err != nil {
		slog.Warn("claude process exited with error", "error", err)
	} else {
		slog.Info("claude process exited normally")
	}

	// If we were shutting down, this is expected
	if session.State == StateShuttingDown {
		session.State = StateStopped
		slog.Info("session stopped successfully", "session_id", session.ClaudeSessionID)
		broadcastState(StateStopped)
	} else {
		// Unexpected exit (crash or user killed the process)
		session.State = StateNone
		slog.Error("unexpected process exit", "state", session.State, "session_id", session.ClaudeSessionID)
		broadcastState(StateNone)
		broadcastEvent(SSEEvent{Type: EventTypeError, Content: "Claude process exited unexpectedly"})
	}

	// Clean up process handles
	session.cmd = nil
	session.stdin = nil
	session.stdout = nil
	session.stderr = nil
}

// resetIdleTimer cancels any existing timer and starts a new one.
//
// Called when Claude transitions to StateWaiting after completing a response.
// When the timer fires, stopSession() is called to shut down the idle session.
//
// TODO: Make timeout configurable via config file or environment variable.
func resetIdleTimer() {
	cancelIdleTimer()

	session.idleTimer = time.AfterFunc(session.idleTimeout, func() {
		slog.Info("idle timeout reached", "timeout", session.idleTimeout)
		stopSession()
	})
}

// cancelIdleTimer stops the idle timer if one is running.
//
// Called when:
//   - A new message is sent (user is active again)
//   - Session is shutting down
func cancelIdleTimer() {
	if session.idleTimer != nil {
		session.idleTimer.Stop()
		session.idleTimer = nil
	}
}

// stopSession shuts down an idle Claude session.
//
// Sends SIGTERM to the Claude process for graceful shutdown. If the process
// doesn't exit within the configured timeout, sends SIGKILL to force termination.
//
// Only stops if session is in StateWaiting (idle but ready). If called
// in any other state, logs a warning and returns.
//
// The session can be resumed later by spawning `claude --resume {session-id}`.
// Session state is persisted by Claude in ~/.claude/, and code changes are
// handled by git commits.
func stopSession() {
	session.mu.Lock()
	defer session.mu.Unlock()

	if session.State != StateWaiting {
		slog.Warn("cannot stop session", "state", session.State, "expected", StateWaiting)
		return
	}

	session.State = StateShuttingDown
	broadcastState(StateShuttingDown)

	// Send SIGTERM to Claude process for graceful shutdown
	if session.cmd != nil && session.cmd.Process != nil {
		processToKill := session.cmd.Process
		pid := processToKill.Pid

		slog.Info("sending SIGTERM to claude process", "pid", pid)
		if err := processToKill.Signal(os.Interrupt); err != nil {
			slog.Error("failed to send SIGTERM", "error", err)
			// Try SIGKILL immediately if SIGTERM fails
			if killErr := processToKill.Kill(); killErr != nil {
				slog.Error("failed to kill process immediately", "error", killErr)
			}
			return
		}

		// Give it time to shut down gracefully, then SIGKILL
		go func(proc *os.Process, pid int) {
			time.Sleep(GracefulShutdownTimeout)
			slog.Warn("force killing process after timeout", "timeout", GracefulShutdownTimeout, "pid", pid)
			if err := proc.Kill(); err != nil {
				slog.Debug("failed to force kill process (may have already exited)", "error", err, "pid", pid)
			}
		}(processToKill, pid)
	}

	slog.Info("session stopped")
}

// handleStream establishes a Server-Sent Events (SSE) connection for real-time updates.
//
// GET /stream
//
// This endpoint:
//  1. Sets up SSE headers for streaming
//  2. Registers the client to receive events
//  3. Sends the current session state and recent output (for reconnection)
//  4. Streams events until the client disconnects
//
// Events sent:
//   - "output": Claude's response text (Content field)
//   - "state": Session state changes (State field)
//   - "error": Error messages (Content field)
//
// The connection stays open until the client disconnects or the server shuts down.
func handleStream(w http.ResponseWriter, r *http.Request) {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Create client with unique ID (timestamp-based)
	clientID := fmt.Sprintf("%d", time.Now().UnixNano())
	client := &SSEClient{
		id:     clientID,
		events: make(chan SSEEvent, SSEClientBufferSize),
		done:   make(chan struct{}),
	}

	// Register client
	session.sseMu.Lock()
	session.sseClients[clientID] = client
	session.sseMu.Unlock()

	slog.Info("sse client connected", "client_id", clientID)

	// Send current state and recent output (for reconnection)
	session.mu.RLock()
	currentState := session.State
	recentOutput := session.outputBuffer.String()
	session.mu.RUnlock()

	// Send recent output buffer so reconnecting clients see context
	if recentOutput != "" {
		sendSSE(w, SSEEvent{Type: EventTypeOutput, Content: recentOutput})
	}

	// Send current state
	sendSSE(w, SSEEvent{Type: EventTypeState, State: string(currentState)})

	// Flush to ensure headers and initial events are sent
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	// Stream events until client disconnects
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			// Client disconnected (browser closed, network issue, etc.)
			session.sseMu.Lock()
			delete(session.sseClients, clientID)
			session.sseMu.Unlock()
			close(client.done)
			slog.Info("sse client disconnected", "client_id", clientID)
			return

		case event := <-client.events:
			sendSSE(w, event)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}
}

// sendSSE sends a single SSE event to the HTTP response writer.
//
// Events are JSON-encoded and formatted according to the SSE spec:
//
//	data: {"type":"output","content":"Hello"}\n\n
//
// The double newline signals the end of the event.
func sendSSE(w http.ResponseWriter, event SSEEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		slog.Error("failed to marshal sse event", "error", err)
		return
	}
	// Send SSE event with explicit event type so frontend addEventListener works
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, data); err != nil {
		slog.Error("failed to write sse event", "error", err)
	}
}

// broadcastOutput broadcasts Claude's output text to all connected SSE clients.
func broadcastOutput(content string) {
	event := SSEEvent{Type: EventTypeOutput, Content: content}
	broadcastEvent(event)
}

// broadcastState broadcasts a session state change to all connected SSE clients.
func broadcastState(state SessionState) {
	event := SSEEvent{Type: EventTypeState, State: string(state)}
	broadcastEvent(event)
}

// formatToolUse formats a tool use event for display to the user.
//
// Shows the tool name with an emoji prefix and relevant parameters
// for context (e.g., "🔧 Read main.go", "🔧 Bash: ls -la").
func formatToolUse(name string, input map[string]interface{}) string {
	switch name {
	case "Read":
		if path, ok := input["file_path"].(string); ok {
			return fmt.Sprintf("🔧 Read %s", filepath.Base(path))
		}
	case "Write":
		if path, ok := input["file_path"].(string); ok {
			return fmt.Sprintf("🔧 Write %s", filepath.Base(path))
		}
	case "Edit":
		if path, ok := input["file_path"].(string); ok {
			return fmt.Sprintf("🔧 Edit %s", filepath.Base(path))
		}
	case "Bash":
		if cmd, ok := input["command"].(string); ok {
			// Truncate long commands
			if len(cmd) > 60 {
				cmd = cmd[:60] + "..."
			}
			return fmt.Sprintf("🔧 Bash: %s", cmd)
		}
	case "Glob":
		if pattern, ok := input["pattern"].(string); ok {
			return fmt.Sprintf("🔧 Glob %s", pattern)
		}
	case "Grep":
		if pattern, ok := input["pattern"].(string); ok {
			return fmt.Sprintf("🔧 Grep: %s", pattern)
		}
	}
	// Fallback: just show tool name
	return fmt.Sprintf("🔧 %s", name)
}

// broadcastEvent sends an event to all connected SSE clients.
//
// Uses a non-blocking send (select with default) to prevent slow clients from
// blocking broadcasts. If a client's event buffer is full, the event is dropped
// for that client only.
func broadcastEvent(event SSEEvent) {
	session.sseMu.RLock()
	defer session.sseMu.RUnlock()

	for _, client := range session.sseClients {
		select {
		case client.events <- event:
			// Event queued successfully
		default:
			// Client buffer full (client is slow or disconnected), skip this event
			slog.Warn("client buffer full, skipping event", "client_id", client.id)
		}
	}
}

// FileChange represents a single file change detected by git.
type FileChange struct {
	Path   string `json:"path"`   // File path relative to repo root
	Status string `json:"status"` // Git status: M (modified), A (added), D (deleted), R (renamed), etc.
	Diff   string `json:"diff"`   // Unified diff output for the file
}

// FileEditEvent represents a real-time file edit operation from Claude's tool calls.
type FileEditEvent struct {
	Tool      string `json:"tool"`       // Tool name: "Edit", "Write", or "NotebookEdit"
	FilePath  string `json:"file_path"`  // Absolute path to the file being edited
	Operation string `json:"operation"`  // Type of operation: "edit", "write", "create"
	Timestamp string `json:"timestamp"`  // ISO 8601 timestamp
}

// trackFileEdit tracks file edit operations from Claude's tool calls in real-time.
//
// This function is called immediately when Claude makes an Edit, Write, or NotebookEdit
// tool call. It broadcasts a file_edit event to SSE clients so they can see which files
// are being modified as Claude works, before the final git diff is available.
//
// Supported tools:
//   - Edit: Modifying existing file content
//   - Write: Creating new files or overwriting existing ones
//   - NotebookEdit: Editing Jupyter notebook cells
func trackFileEdit(toolName string, input map[string]interface{}) {
	// Only track file editing tools
	if toolName != "Edit" && toolName != "Write" && toolName != "NotebookEdit" {
		return
	}

	var filePath string
	var operation string

	// Extract file path based on tool type
	switch toolName {
	case "Edit", "Write":
		if path, ok := input["file_path"].(string); ok {
			filePath = path
		}
	case "NotebookEdit":
		if path, ok := input["notebook_path"].(string); ok {
			filePath = path
		}
	}

	// If no file path found, skip tracking
	if filePath == "" {
		return
	}

	// Determine operation type
	switch toolName {
	case "Edit":
		operation = "edit"
	case "Write":
		operation = "write"
	case "NotebookEdit":
		operation = "notebook_edit"
	}

	// Create the event
	event := FileEditEvent{
		Tool:      toolName,
		FilePath:  filePath,
		Operation: operation,
		Timestamp: time.Now().Format(time.RFC3339),
	}

	// Marshal to JSON
	eventJSON, err := json.Marshal(event)
	if err != nil {
		slog.Error("failed to marshal file edit event", "error", err)
		return
	}

	// Broadcast to SSE clients
	slog.Info("file edit tracked", "tool", toolName, "path", filePath, "operation", operation)
	broadcastEvent(SSEEvent{
		Type:    EventTypeFileChanges,
		Content: string(eventJSON),
	})
}

// detectAndBroadcastFileChanges detects file changes using git and broadcasts them to SSE clients.
//
// This function is called after Claude finishes responding. It:
//  1. Runs `git status --short` to detect changed files
//  2. For each changed file, runs `git diff` to get the actual diff
//  3. Broadcasts the changes as a file_changes event
//
// This allows the frontend to show users what files Claude modified.
func detectAndBroadcastFileChanges() {
	session.mu.RLock()
	repoPath := session.RepoPath
	session.mu.RUnlock()

	// Determine repo path
	if repoPath == "" {
		if envPath := os.Getenv("REPO_PATH"); envPath != "" {
			repoPath = envPath
		} else {
			var err error
			repoPath, err = os.Getwd()
			if err != nil {
				slog.Debug("failed to get current directory for git diff", "error", err)
				return
			}
		}
	}

	// Run git status locally
	cmd := exec.Command("git", "status", "--short")
	cmd.Dir = repoPath
	output, err := cmd.Output()

	if err != nil {
		slog.Debug("failed to run git status (not a git repo or no changes)", "error", err)
		return
	}

	if len(output) == 0 {
		// No changes detected
		return
	}

	// Parse git status output
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var changes []FileChange

	for _, line := range lines {
		if len(line) < 4 {
			continue
		}

		// Git status format: "XY filename" where X is staged, Y is unstaged
		statusCode := strings.TrimSpace(line[0:2])
		filePath := strings.TrimSpace(line[3:])

		// Determine the primary status
		status := "M" // default to modified
		if strings.Contains(statusCode, "A") {
			status = "A" // added
		} else if strings.Contains(statusCode, "D") {
			status = "D" // deleted
		} else if strings.Contains(statusCode, "R") {
			status = "R" // renamed
		} else if strings.Contains(statusCode, "?") {
			status = "U" // untracked
		}

		// Get the diff for this file (skip untracked files)
		var diff string
		if status != "U" && status != "D" {
			diffCmd := exec.Command("git", "diff", "HEAD", "--", filePath)
			diffCmd.Dir = repoPath
			diffOutput, diffErr := diffCmd.Output()

			if diffErr == nil {
				diff = string(diffOutput)
			}
		}

		changes = append(changes, FileChange{
			Path:   filePath,
			Status: status,
			Diff:   diff,
		})
	}

	if len(changes) > 0 {
		// Marshal changes to JSON
		changesJSON, err := json.Marshal(changes)
		if err != nil {
			slog.Error("failed to marshal file changes", "error", err)
			return
		}

		slog.Info("detected file changes", "count", len(changes))
		broadcastEvent(SSEEvent{
			Type:    EventTypeFileChanges,
			Content: string(changesJSON),
		})
	}
}

// handleMessage sends a user message to Claude.
//
// POST /message
// Request body:
//
//	{
//	  "content": "Your message to Claude"
//	}
//
// Response on success:
//
//	{
//	  "success": true,
//	  "queued": true,
//	  "state": "active"
//	}
//
// Response on error:
//
//	{
//	  "error": "session is starting, please wait"
//	}
//
// State handling:
//   - StateNone: Starts a new session and sends the message immediately
//   - StateStopped: Triggers resume flow - spawns `claude --resume {session-id}`
//   - StateStarting: Returns error (wait for session to be ready)
//   - StateActive/StateWaiting: Sends message to Claude via stdin
//
// When sending a message to a StateWaiting session:
//   - Cancels the idle timer (user is active again)
//   - Transitions to StateActive
//   - Updates LastActivity timestamp
func handleMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Content == "" {
		http.Error(w, "Content is required", http.StatusBadRequest)
		return
	}

	session.mu.Lock()
	state := session.State
	stdin := session.stdin
	session.mu.Unlock()

	// Handle based on current state
	switch state {
	case StateNone:
		// No session exists - start a new session with this message
		slog.Info("starting new session from message", "message", req.Content)

		// Get repo path from environment or use current directory
		repoPath := os.Getenv("REPO_PATH")
		if repoPath == "" {
			var err error
			repoPath, err = os.Getwd()
			if err != nil {
				respondError(w, http.StatusInternalServerError, fmt.Sprintf("failed to get working directory: %v", err))
				return
			}
		}

		// Start the session with the queued message
		if err := startClaudeProcessWithMessage(repoPath, req.Content); err != nil {
			slog.Error("failed to start session with message", "error", err)
			respondError(w, http.StatusInternalServerError, fmt.Sprintf("failed to start session: %v", err))
			return
		}

		respondJSON(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"queued":  true,
			"started": true,
			"state":   StateActive,
		})
		return

	case StateStopped:
		// Session was stopped due to idle timeout - trigger resume
		slog.Info("resuming stopped session", "session_id", session.ClaudeSessionID, "message", req.Content)

		// Resume the session with the queued message
		if err := resumeClaudeProcess(req.Content); err != nil {
			slog.Error("failed to resume session", "error", err)
			respondError(w, http.StatusInternalServerError, fmt.Sprintf("failed to resume: %v", err))
			return
		}

		respondJSON(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"queued":  true,
			"resumed": true,
			"state":   StateActive,
		})
		return

	case StateStarting:
		// Session is still starting up - user should wait and retry
		respondJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
			"error":  "session is starting, please wait",
			"queued": false,
		})
		return

	case StateActive, StateWaiting:
		// Session is ready - send message to Claude
		if stdin == nil {
			slog.Error("stdin not available", "state", state)
			respondError(w, http.StatusInternalServerError, "stdin not available")
			return
		}

		// Handle /clear command - clear ring buffer
		if strings.TrimSpace(req.Content) == "/clear" {
			session.mu.Lock()
			session.outputBuffer = NewRingBuffer(RingBufferSize)
			session.mu.Unlock()
			slog.Info("cleared ring buffer due to /clear command")
		}

		// Format message as JSON for stream-json input
		// Claude expects: {"type":"user","message":{"role":"user","content":"..."}}
		inputMsg := map[string]interface{}{
			"type": MessageTypeUser,
			"message": map[string]interface{}{
				"role":    MessageTypeUser,
				"content": req.Content,
			},
		}
		msgBytes, err := json.Marshal(inputMsg)
		if err != nil {
			slog.Error("failed to marshal input message", "error", err)
			respondError(w, http.StatusInternalServerError, "failed to format message")
			return
		}

		// Write JSON message to stdin (newline-delimited)
		_, err = fmt.Fprintf(stdin, "%s\n", msgBytes)
		if err != nil {
			slog.Error("failed to write to stdin", "error", err)
			respondError(w, http.StatusInternalServerError, "failed to send message to Claude")
			return
		}

		// Update state if we were waiting (cancel idle timer, mark active)
		session.mu.Lock()
		session.LastActivity = time.Now()
		if session.State == StateWaiting {
			session.State = StateActive
			cancelIdleTimer() // User is active again, don't stop session
			go broadcastState(StateActive)
		}
		session.mu.Unlock()

		respondJSON(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"queued":  true,
			"state":   session.State,
		})

	default:
		// Unexpected state (should never happen)
		slog.Error("unexpected state", "state", state)
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("unexpected state: %s", state))
	}
}

// handleDiff returns the git diff for a specific file.
//
// GET /diff?file=path/to/file
//
// Query parameters:
//   - file: Path to the file (required)
//
// Response on success:
//
//	{
//	  "file": "path/to/file",
//	  "diff": "... git diff output ..."
//	}
//
// Response on error:
//
//	{
//	  "error": "file parameter required"
//	}
func handleDiff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	filePath := r.URL.Query().Get("file")
	if filePath == "" {
		respondError(w, http.StatusBadRequest, "file parameter required")
		return
	}

	session.mu.RLock()
	repoPath := session.RepoPath
	session.mu.RUnlock()

	if repoPath == "" {
		respondError(w, http.StatusBadRequest, "no active session")
		return
	}

	// Run git diff for the specific file
	cmd := exec.Command("git", "diff", "HEAD", "--", filePath)
	cmd.Dir = repoPath

	output, err := cmd.CombinedOutput()
	if err != nil {
		// If file is untracked, try showing it as a new file
		if strings.Contains(string(output), "no such path") || strings.Contains(err.Error(), "exit status") {
			// Try git diff for staged files
			cmd = exec.Command("git", "diff", "--cached", "--", filePath)
			cmd.Dir = repoPath
			output, err = cmd.CombinedOutput()

			if err != nil {
				// If still fails, try to show the file as completely new
				cmd = exec.Command("git", "diff", "/dev/null", filePath)
				cmd.Dir = repoPath
				output, _ = cmd.CombinedOutput()
			}
		}
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"file": filePath,
		"diff": string(output),
	})
}

// handleIndex serves the web UI.
//
// GET /
//
// Serves static files from the Vite build directory (dist/). The path can be
// configured via the WEB_PATH environment variable, defaulting to "../web/dist".
// Falls back to index.html for client-side routing.
func handleIndex(w http.ResponseWriter, r *http.Request) {
	// Get web directory path (Vite dist output)
	webPath := os.Getenv("WEB_PATH")
	if webPath == "" {
		webPath = DefaultWebPath
	}

	// Construct file path
	path := filepath.Join(webPath, r.URL.Path)

	// Check if file exists
	info, err := os.Stat(path)
	if err == nil && !info.IsDir() {
		// File exists, serve it
		http.ServeFile(w, r, path)
		return
	}

	// File doesn't exist or is directory, serve index.html for SPA routing
	indexPath := filepath.Join(webPath, "index.html")
	http.ServeFile(w, r, indexPath)
}
