package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SessionState represents the current state of a Claude Code session
type SessionState string

const (
	StateNone       SessionState = "none"       // No session started
	StateStarting   SessionState = "starting"   // Sprite/process starting up
	StateActive     SessionState = "active"     // Claude is working (generating output)
	StateWaiting    SessionState = "waiting"    // Claude is waiting for input
	StateHibernating SessionState = "hibernating" // Preparing to hibernate
	StateHibernated SessionState = "hibernated"  // Session hibernated (checkpoint saved)
)

// SSEClient represents a connected SSE client
type SSEClient struct {
	id     string
	events chan SSEEvent
	done   chan struct{}
}

// SSEEvent is an event to send to SSE clients
type SSEEvent struct {
	Type    string `json:"type"`              // "output", "state", "error", "info"
	Content string `json:"content,omitempty"` // For output events
	State   string `json:"state,omitempty"`   // For state events
}

// RingBuffer is a simple ring buffer for keeping recent output
type RingBuffer struct {
	data     []byte
	size     int
	writePos int
	full     bool
	mu       sync.Mutex
}

func NewRingBuffer(size int) *RingBuffer {
	return &RingBuffer{
		data: make([]byte, size),
		size: size,
	}
}

func (rb *RingBuffer) Write(p []byte) (n int, err error) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	for _, b := range p {
		rb.data[rb.writePos] = b
		rb.writePos = (rb.writePos + 1) % rb.size
		if rb.writePos == 0 {
			rb.full = true
		}
	}
	return len(p), nil
}

func (rb *RingBuffer) String() string {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if !rb.full {
		return string(rb.data[:rb.writePos])
	}

	// Buffer is full, need to read from writePos to end, then start to writePos
	result := make([]byte, rb.size)
	copy(result, rb.data[rb.writePos:])
	copy(result[rb.size-rb.writePos:], rb.data[:rb.writePos])
	return string(result)
}

// Session holds all state for a Claude Code session
type Session struct {
	mu sync.RWMutex

	State           SessionState
	ClaudeSessionID string
	RepoPath        string
	LastActivity    time.Time
	LastOutputAt    time.Time

	// Process management
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	// Output handling
	outputBuffer *RingBuffer
	sseClients   map[string]*SSEClient
	sseMu        sync.RWMutex

	// Idle detection
	idleTimer    *time.Timer
	idleTimeout  time.Duration
}

// Global session (single session for MVP)
var session *Session
var sessionMu sync.Mutex

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.SetFlags(log.Ltime | log.Lshortfile)
	log.Println("Doze API Server starting...")

	// Initialize global session
	session = &Session{
		State:        StateNone,
		outputBuffer: NewRingBuffer(10 * 1024), // 10KB ring buffer
		sseClients:   make(map[string]*SSEClient),
		idleTimeout:  3 * time.Minute,
	}

	// API endpoints
	http.HandleFunc("/status", handleStatus)
	http.HandleFunc("/start", handleStart)
	http.HandleFunc("/stream", handleStream)
	http.HandleFunc("/message", handleMessage)

	// Serve web UI
	http.HandleFunc("/", handleIndex)

	log.Printf("Listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	session.mu.RLock()
	defer session.mu.RUnlock()

	status := map[string]interface{}{
		"state":            session.State,
		"claude_session_id": session.ClaudeSessionID,
		"repo_path":        session.RepoPath,
		"last_activity":    session.LastActivity,
		"idle_seconds":     0,
	}

	if !session.LastActivity.IsZero() {
		status["idle_seconds"] = int(time.Since(session.LastActivity).Seconds())
	}

	// Include recent output (last 500 chars for status)
	recentOutput := session.outputBuffer.String()
	if len(recentOutput) > 500 {
		recentOutput = recentOutput[len(recentOutput)-500:]
	}
	status["recent_output"] = recentOutput

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessionMu.Lock()
	defer sessionMu.Unlock()

	// Check if session already running
	if session.State != StateNone && session.State != StateHibernated {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "session already active",
			"state": session.State,
		})
		return
	}

	// Parse request for optional repo path
	var req struct {
		RepoPath string `json:"repo_path"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	repoPath := req.RepoPath
	if repoPath == "" {
		repoPath = os.Getenv("REPO_PATH")
	}
	if repoPath == "" {
		repoPath = "/home/sprite/doze" // Default for testing
	}

	// Start Claude Code process
	if err := startClaudeProcess(repoPath); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"state":     session.State,
		"repo_path": session.RepoPath,
	})
}

func startClaudeProcess(repoPath string) error {
	session.mu.Lock()
	defer session.mu.Unlock()

	session.State = StateStarting
	session.RepoPath = repoPath
	session.LastActivity = time.Now()

	// Broadcast state change
	broadcastState(StateStarting)

	// Build command - use stream-json for bidirectional streaming
	cmd := exec.Command("claude",
		"--print",
		"--input-format=stream-json",
		"--output-format=stream-json",
		"--verbose",
	)
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

	log.Printf("Claude process started with PID %d", cmd.Process.Pid)

	// Start goroutines to handle output
	go handleStdout()
	go handleStderr()
	go waitForExit()

	session.State = StateActive
	broadcastState(StateActive)

	return nil
}

// ClaudeStreamMessage represents a message from Claude's stream-json output
type ClaudeStreamMessage struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id,omitempty"`
	Result    string `json:"result,omitempty"` // Final result text
	Message   struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"message,omitempty"`
}

func handleStdout() {
	scanner := bufio.NewScanner(session.stdout)
	// Increase buffer size for large JSON messages
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		session.mu.Lock()
		session.LastOutputAt = time.Now()
		session.LastActivity = time.Now()
		session.mu.Unlock()

		// Try to parse as JSON
		var msg ClaudeStreamMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			// Not JSON, broadcast as raw output
			log.Printf("Non-JSON stdout: %s", line)
			session.mu.Lock()
			session.outputBuffer.Write([]byte(line + "\n"))
			session.mu.Unlock()
			broadcastOutput(line + "\n")
			continue
		}

		// Handle different message types
		var content string
		switch msg.Type {
		case "assistant":
			// Extract text from message content array
			for _, c := range msg.Message.Content {
				if c.Type == "text" {
					content += c.Text
				}
			}
			// Capture session ID
			if msg.SessionID != "" {
				session.mu.Lock()
				session.ClaudeSessionID = msg.SessionID
				session.mu.Unlock()
				log.Printf("Captured session ID: %s", msg.SessionID)
			}
		case "result":
			// Result indicates completion - don't output content (already shown via assistant message)
			if msg.SessionID != "" {
				session.mu.Lock()
				session.ClaudeSessionID = msg.SessionID
				session.mu.Unlock()
			}
			// Mark as waiting for input
			session.mu.Lock()
			if session.State == StateActive {
				session.State = StateWaiting
				log.Println("Claude completed response, waiting for input")
				go broadcastState(StateWaiting)
				resetIdleTimer()
			}
			session.mu.Unlock()
			continue // Don't output the result text (already shown)
		case "error":
			content = "[Error] " + msg.Result
		case "system":
			// System messages can be logged but not shown to user
			log.Printf("System message: %s", line)
			continue
		default:
			// Log unknown types for debugging
			log.Printf("Unknown message type: %s, raw: %s", msg.Type, line)
			continue
		}

		if content != "" {
			session.mu.Lock()
			session.outputBuffer.Write([]byte(content))
			session.mu.Unlock()
			broadcastOutput(content)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("stdout scanner error: %v", err)
	}
}

func handleStderr() {
	reader := bufio.NewReader(session.stderr)
	buf := make([]byte, 1024)

	for {
		n, err := reader.Read(buf)
		if err != nil {
			if err != io.EOF {
				log.Printf("stderr read error: %v", err)
			}
			return
		}

		if n > 0 {
			output := string(buf[:n])
			log.Printf("stderr: %s", output)

			// Also broadcast stderr as output (it's part of the terminal experience)
			session.mu.Lock()
			session.outputBuffer.Write(buf[:n])
			session.mu.Unlock()

			broadcastOutput(output)
		}
	}
}

func waitForExit() {
	err := session.cmd.Wait()

	session.mu.Lock()
	defer session.mu.Unlock()

	if err != nil {
		log.Printf("Claude process exited with error: %v", err)
	} else {
		log.Println("Claude process exited normally")
	}

	// If we were hibernating, this is expected
	if session.State == StateHibernating {
		session.State = StateHibernated
		broadcastState(StateHibernated)
	} else {
		// Unexpected exit
		session.State = StateNone
		broadcastState(StateNone)
		broadcastEvent(SSEEvent{Type: "error", Content: "Claude process exited unexpectedly"})
	}

	// Clean up
	session.cmd = nil
	session.stdin = nil
	session.stdout = nil
	session.stderr = nil
}

// detectWaitingForInput checks if output indicates Claude is waiting for user input
func detectWaitingForInput(output string) bool {
	// Look for common prompt patterns
	// Claude Code typically shows a prompt like "> " or has specific ANSI sequences
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Check for prompt indicators
		if strings.HasSuffix(trimmed, ">") ||
			strings.HasSuffix(trimmed, "❯") ||
			strings.Contains(line, "Waiting for input") ||
			strings.Contains(line, "What would you like") {
			return true
		}
	}
	return false
}

func resetIdleTimer() {
	cancelIdleTimer()

	session.idleTimer = time.AfterFunc(session.idleTimeout, func() {
		log.Println("Idle timeout reached, hibernating...")
		hibernate()
	})
}

func cancelIdleTimer() {
	if session.idleTimer != nil {
		session.idleTimer.Stop()
		session.idleTimer = nil
	}
}

func hibernate() {
	session.mu.Lock()
	defer session.mu.Unlock()

	if session.State != StateWaiting {
		log.Println("Cannot hibernate: not in waiting state")
		return
	}

	session.State = StateHibernating
	broadcastState(StateHibernating)

	// Send SIGTERM to Claude process for graceful shutdown
	if session.cmd != nil && session.cmd.Process != nil {
		log.Println("Sending SIGTERM to Claude process")
		session.cmd.Process.Signal(os.Interrupt)

		// Give it 10 seconds to shut down gracefully
		go func() {
			time.Sleep(10 * time.Second)
			session.mu.Lock()
			defer session.mu.Unlock()
			if session.cmd != nil && session.cmd.Process != nil {
				log.Println("Force killing Claude process")
				session.cmd.Process.Kill()
			}
		}()
	}

	// TODO: Create Sprite checkpoint here via Sprites SDK
	// For now, we just mark as hibernated when process exits
}

func handleStream(w http.ResponseWriter, r *http.Request) {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Create client
	clientID := fmt.Sprintf("%d", time.Now().UnixNano())
	client := &SSEClient{
		id:     clientID,
		events: make(chan SSEEvent, 100),
		done:   make(chan struct{}),
	}

	// Register client
	session.sseMu.Lock()
	session.sseClients[clientID] = client
	session.sseMu.Unlock()

	log.Printf("SSE client connected: %s", clientID)

	// Send current state
	session.mu.RLock()
	currentState := session.State
	recentOutput := session.outputBuffer.String()
	session.mu.RUnlock()

	// Send recent output buffer for reconnection
	if recentOutput != "" {
		sendSSE(w, SSEEvent{Type: "output", Content: recentOutput})
	}

	// Send current state
	sendSSE(w, SSEEvent{Type: "state", State: string(currentState)})

	// Flush to ensure headers are sent
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	// Stream events
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			// Client disconnected
			session.sseMu.Lock()
			delete(session.sseClients, clientID)
			session.sseMu.Unlock()
			close(client.done)
			log.Printf("SSE client disconnected: %s", clientID)
			return

		case event := <-client.events:
			sendSSE(w, event)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}
}

func sendSSE(w http.ResponseWriter, event SSEEvent) {
	data, _ := json.Marshal(event)
	fmt.Fprintf(w, "data: %s\n\n", data)
}

func broadcastOutput(content string) {
	event := SSEEvent{Type: "output", Content: content}
	broadcastEvent(event)
}

func broadcastState(state SessionState) {
	event := SSEEvent{Type: "state", State: string(state)}
	broadcastEvent(event)
}

func broadcastEvent(event SSEEvent) {
	session.sseMu.RLock()
	defer session.sseMu.RUnlock()

	for _, client := range session.sseClients {
		select {
		case client.events <- event:
		default:
			// Client buffer full, skip
			log.Printf("Client %s buffer full, skipping event", client.id)
		}
	}
}

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
		// No session, start one and queue the message
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "no active session, call /start first",
		})
		return

	case StateHibernated:
		// TODO: Resume from hibernation
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "session is hibernated, resume not yet implemented",
			"queued":  false,
		})
		return

	case StateStarting:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":  "session is starting, please wait",
			"queued": false,
		})
		return

	case StateActive, StateWaiting:
		// Send message to Claude
		if stdin == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": "stdin not available",
			})
			return
		}

		// Format message as JSON for stream-json input
		// The format requires a nested message object with role
		inputMsg := map[string]interface{}{
			"type": "user",
			"message": map[string]interface{}{
				"role":    "user",
				"content": req.Content,
			},
		}
		msgBytes, _ := json.Marshal(inputMsg)

		// Write JSON message to stdin
		_, err := fmt.Fprintf(stdin, "%s\n", msgBytes)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": err.Error(),
			})
			return
		}

		// Update state
		session.mu.Lock()
		session.LastActivity = time.Now()
		if session.State == StateWaiting {
			session.State = StateActive
			cancelIdleTimer()
			go broadcastState(StateActive)
		}
		session.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"queued":  true,
			"state":   session.State,
		})

	default:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": fmt.Sprintf("unexpected state: %s", state),
		})
	}
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	// Only serve index for root path
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// Serve web UI from /web directory
	webPath := os.Getenv("WEB_PATH")
	if webPath == "" {
		webPath = "../web"
	}

	indexPath := filepath.Join(webPath, "index.html")
	http.ServeFile(w, r, indexPath)
}
