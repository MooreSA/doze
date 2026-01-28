# Claude Code Remote - MVP Design

## Core Value Proposition

**Chat with Claude Code from your phone without burning money when idle.**

That's it. Everything else is future work.

---

## What's In vs Out

### ✅ MVP (Week 1-2)

- Single repo (hardcoded in config)
- Text-only chat from mobile browser
- Live terminal streaming
- Auto-hibernate after 3 minutes idle
- Resume when you send next message
- Reconnection without losing your place

### ❌ Not MVP (Future)

- Voice input
- Push notifications
- Multiple repos / repo picker
- GitHub integration
- QR code pairing
- Session history UI
- Any native mobile app work
- Branch protection enforcement
- Fancy UI/UX polish

**Rationale:** Prove that hibernate/resume works reliably before building anything else. If `--resume` is broken, we need to know NOW, not after building the full product.

---

## Architecture (Simplified)

```
┌─────────────────────────────┐
│  Phone Browser              │
│  - Single HTML page         │
│  - EventSource (SSE)        │
│  - Fetch API for messages   │
└─────────────┬───────────────┘
              │ HTTPS
              ▼
┌─────────────────────────────┐
│  API Server (Go)            │
│  - /stream (SSE)            │
│  - /message (POST)          │
│  - /status (GET)            │
│  - Session state machine    │
└─────────────┬───────────────┘
              │ Sprites SDK
              ▼
┌─────────────────────────────┐
│  Single Sprite              │
│  - Claude Code authed       │
│  - Your repo cloned         │
│  - Hibernates when idle     │
└─────────────────────────────┘
```

**No database.** Session state lives in memory. Server restart = session lost. That's acceptable for MVP.

**No GitHub API.** Repo is cloned once during Sprite setup and hardcoded in config.

**No auth (testing).** Start with no auth for local testing. Add simple bearer token before deploying publicly. Save Tailscale for V2.

---

## Critical Path: Hibernate/Resume Flow

This is the **highest risk** piece. Build and test this first.

### Session States

```
STARTING → ACTIVE → WAITING_FOR_INPUT → HIBERNATING → HIBERNATED
                                                     → WAKING → ACTIVE

WAITING_FOR_INPUT → (user sends message) → ACTIVE
WAITING_FOR_INPUT → (idle timeout) → HIBERNATING → HIBERNATED
```

### State Detection

**How do we know Claude is waiting for input?**

Parse stdout for Claude Code's prompt patterns:
- The cursor appears (readline prompt)
- Last line contains `>`
- No output for 2 seconds (heuristic)

**Idle timeout:** 3 minutes of `WAITING_FOR_INPUT` state → trigger hibernate.

### Hibernate Process

```go
1. Send SIGTERM to Claude Code process
2. Wait up to 10s for graceful shutdown
3. Checkpoint Sprite via Sprites SDK
4. Update state to HIBERNATED
5. Sprite automatically stops (no cost)
```

### Resume Process

```go
1. Restore Sprite from checkpoint (~1 second)
2. Run: claude --resume <session-id>
3. Wait for Claude Code to be ready (detect prompt)
4. Inject queued user message via stdin
5. Update state to ACTIVE
```

### Resume Fallback (if --resume fails)

Given the known bugs (corrupted sessions, index issues), you **MUST** have a fallback strategy.

**Strategy 1: Graceful degradation (RECOMMENDED FOR MVP)**

Don't checkpoint during tool execution. Only hibernate when Claude is idle and waiting for input.

```go
func canSafelyHibernate() bool {
    // Only hibernate if:
    // 1. No active tool execution
    // 2. Claude is showing prompt (waiting for input)
    // 3. No partial output in last 5 seconds
    return state == WAITING_FOR_INPUT &&
           !hasActiveToolExecution() &&
           time.Since(lastOutputAt) > 5*time.Second
}
```

This minimizes risk of incomplete `.jsonl` files.

**Strategy 2: Always use direct session ID**

Don't rely on Claude Code's session list. Track the session ID yourself when starting:

```go
// When starting Claude
cmd := exec.Command("claude", "--cwd", repoPath)
// Parse stdout for: "Session ID: abc123-def456"
sessionID := extractSessionID(stdout)
session.ClaudeSessionID = sessionID

// When resuming
cmd := exec.Command("claude", "--resume", sessionID)
```

**Strategy 3: Detect resume failure, start fresh**

```go
func tryResume(sessionID string, userMessage string) error {
    cmd := exec.Command("claude", "--resume", sessionID)
    stderr := captureStderr(cmd)

    if strings.Contains(stderr, "No conversations found") ||
       strings.Contains(stderr, "No messages returned") {

        // Resume failed - start fresh with context
        return startFreshWithContext(sessionID, userMessage)
    }

    return nil
}

func startFreshWithContext(oldSessionID string, userMessage string) error {
    // Read output buffer (last 10KB we have in memory)
    recentContext := session.OutputBuffer.String()

    contextPrompt := fmt.Sprintf(`Previous session ended unexpectedly.

Recent context from that session:
%s

User's new request: %s`, recentContext, userMessage)

    // Start new session with context
    cmd := exec.Command("claude",
        "--append-system-prompt", contextPrompt,
        "--cwd", repoPath)

    return cmd.Run()
}
```

**Strategy 4: Increase idle timeout to reduce resume frequency**

Instead of 3 minutes, use 15-30 minutes. Trade cost for reliability.

**MVP approach:**
1. Use Strategy 1 (only hibernate when safe) + Strategy 2 (direct session ID)
2. Implement Strategy 3 (graceful fallback) on Day 6
3. Consider Strategy 4 if resume is still flaky

---

## API Design (Minimal)

### GET /status
Returns current session state.

```json
{
  "state": "active" | "waiting" | "hibernated",
  "repo": "my-repo",
  "claude_session_id": "abc123",
  "last_output": "last few lines...",
  "idle_seconds": 45
}
```

### GET /stream (Server-Sent Events)
Streams terminal output in real-time.

```
data: {"type": "output", "content": "Running tests...\n"}
data: {"type": "state", "state": "active"}
data: {"type": "output", "content": "✓ All tests passed\n"}
data: {"type": "state", "state": "waiting"}
```

Client reconnects automatically on disconnect.

### POST /message
Send message to Claude.

```json
{
  "content": "Fix the bug in auth.go"
}
```

Response:
```json
{
  "queued": true,
  "state": "active"
}
```

If session is hibernated, this triggers resume + message injection.

---

## Authentication (Progressive)

### Local Testing (Day 1-4)
No auth. API runs on localhost or local network.

```bash
# Run API server
./api-server

# Phone on same WiFi
http://192.168.1.100:8080
```

### Public Deploy (Day 5+)
Simple bearer token.

**Server:**
```go
func authMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")

        if token != os.Getenv("AUTH_TOKEN") {
            http.Error(w, "Unauthorized", 401)
            return
        }

        next.ServeHTTP(w, r)
    })
}
```

**Client (HTML):**
```javascript
// Load token from URL on first visit
const urlParams = new URLSearchParams(window.location.search);
const token = urlParams.get('token') || localStorage.getItem('token');

if (token) {
    localStorage.setItem('token', token);
    // Remove from URL
    window.history.replaceState({}, '', '/');
}

// Use in all requests
fetch('/message', {
    headers: { 'Authorization': `Bearer ${token}` }
});
```

**Setup:**
```bash
# Generate token
TOKEN=$(openssl rand -hex 32)

# Save to Fly secrets
fly secrets set AUTH_TOKEN=$TOKEN

# Visit on phone
https://your-app.fly.dev/?token=abc123def456...
```

Token is saved in localStorage, only needs to be entered once.

### V2: Tailscale
Proper zero-trust networking. Requires Tailscale app on phone and integration with Fly.io. Save for later.

---

## Data Model (In-Memory)

```go
type Session struct {
    ID              string
    Repo            string
    SpriteName      string
    ClaudeSessionID string

    State           SessionState // STARTING, ACTIVE, WAITING, HIBERNATING, HIBERNATED
    LastActivity    time.Time
    IdleTimer       *time.Timer

    // Streaming
    OutputBuffer    *RingBuffer    // Last 10KB of output for reconnection
    SSEClients      []*SSEClient   // Active EventSource connections

    // Process management
    ClaudeCmd       *exec.Cmd
    ClaudeStdin     io.WriteCloser
    ClaudeStdout    io.Reader

    // Sprite management
    SpriteClient    *sprites.Client
}
```

**Ring buffer:** Keep last 10KB of output. When client reconnects, replay buffer then continue streaming.

**No persistence:** Server restart = session lost. Acceptable for MVP. Sprite checkpoint survives, so you can manually resume from terminal if needed.

---

## Web UI (Brutally Simple)

Single HTML file served by API server. No build step, no framework.

```html
<!DOCTYPE html>
<html>
<head>
    <title>Claude Code</title>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <style>
        /* Mobile-first, dark theme, monospace */
        body {
            background: #1e1e1e;
            color: #d4d4d4;
            font-family: 'Courier New', monospace;
            margin: 0;
            padding: 0;
        }
        #output {
            padding: 1rem;
            height: calc(100vh - 120px);
            overflow-y: auto;
            white-space: pre-wrap;
            word-wrap: break-word;
        }
        #input-area {
            position: fixed;
            bottom: 0;
            width: 100%;
            background: #252526;
            padding: 1rem;
            box-sizing: border-box;
        }
        #input {
            width: 100%;
            padding: 0.75rem;
            background: #3c3c3c;
            border: 1px solid #555;
            color: #d4d4d4;
            font-family: inherit;
            font-size: 16px; /* Prevents iOS zoom on focus */
        }
        .status {
            padding: 0.5rem 1rem;
            background: #2d2d30;
            border-bottom: 1px solid #555;
            font-size: 0.85rem;
        }
        .waiting { color: #4ec9b0; }
        .active { color: #dcdcaa; }
        .hibernated { color: #ce9178; }
    </style>
</head>
<body>
    <div class="status" id="status">Connecting...</div>
    <div id="output"></div>
    <div id="input-area">
        <input type="text" id="input" placeholder="Send a message..." />
    </div>

    <script>
        const output = document.getElementById('output');
        const input = document.getElementById('input');
        const status = document.getElementById('status');

        let eventSource;

        function connect() {
            eventSource = new EventSource('/stream');

            eventSource.onmessage = (e) => {
                const data = JSON.parse(e.data);

                if (data.type === 'output') {
                    output.textContent += data.content;
                    output.scrollTop = output.scrollHeight;
                }

                if (data.type === 'state') {
                    status.textContent = `State: ${data.state}`;
                    status.className = `status ${data.state}`;
                }
            };

            eventSource.onerror = () => {
                status.textContent = 'Disconnected - reconnecting...';
                eventSource.close();
                setTimeout(connect, 2000);
            };
        }

        input.addEventListener('keypress', async (e) => {
            if (e.key === 'Enter' && input.value.trim()) {
                const message = input.value;
                input.value = '';

                await fetch('/message', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ content: message })
                });
            }
        });

        connect();
    </script>
</body>
</html>
```

That's the entire UI. 70 lines. No dependencies. Works on any mobile browser.

---

## Build Plan (1-2 Weeks)

### Day 1-2: Sprite Setup & Validation

**Goal:** Prove Claude Code works in a Sprite and --resume is reliable.

- [ ] Create base Sprite (manual via CLI)
- [ ] Install dev tools (node, go, git, etc.)
- [ ] Install Claude Code CLI
- [ ] Authenticate with Max subscription (browser OAuth)
- [ ] Checkpoint as `base-authed`
- [ ] Clone your repo, install deps
- [ ] Checkpoint as `repo-ready`
- [ ] Test: run Claude Code, checkpoint, restore, --resume
- [ ] **Critical:** Test --resume multiple times. Does context survive? Does state survive?
- [ ] Document any --resume bugs you hit

**Deliverable:** Checkpointed Sprite that can reliably run and resume Claude Code sessions.

**Stop gate:** If --resume is fundamentally broken, pivot to "keep sessions alive longer" strategy or accept fresh context on resume.

**Known issues (January 2026):**
- Killing Claude during tool execution corrupts session files ([#18880](https://github.com/anthropics/claude-code/issues/18880))
- Session index can become out of sync ([#18311](https://github.com/anthropics/claude-code/issues/18311))
- Must use direct session ID, not rely on resume list

### Day 3-4: API Server Core

**Goal:** Go server that can stream Claude Code output.

- [ ] Go project setup
- [ ] Integrate Sprites SDK
- [ ] Session state machine (in-memory)
- [ ] Spawn Claude Code process in Sprite
- [ ] Stream stdout to SSE clients
- [ ] POST /message endpoint (writes to stdin)
- [ ] GET /status endpoint
- [ ] Ring buffer for output history
- [ ] Deploy to Fly.io (or run locally on Tailscale)

**Deliverable:** Can curl the API to chat with Claude Code.

**Test:**
```bash
# Terminal 1: Stream output
curl -N http://localhost:8080/stream

# Terminal 2: Send messages
curl -X POST http://localhost:8080/message \
  -H "Content-Type: application/json" \
  -d '{"content": "What files are in this repo?"}'
```

### Day 5-6: Idle Detection & Hibernate

**Goal:** Sessions hibernate automatically when idle.

- [ ] Detect "waiting for input" state (parse stdout)
- [ ] Start 3-minute idle timer on WAITING state
- [ ] On timeout: SIGTERM → checkpoint → HIBERNATED
- [ ] Resume flow: restore checkpoint → --resume → inject message
- [ ] Test: start session, walk away, come back, send message
- [ ] **Critical:** Does resume work after checkpoint? Full context? Tool state?

**Deliverable:** Sessions hibernate and resume without manual intervention.

**Test:**
1. Send message: "List files"
2. Wait for response
3. Wait 3+ minutes (or set timeout to 10s for testing)
4. Verify session hibernates
5. Send message: "Tell me about auth.go"
6. Verify session resumes and Claude has context

### Day 7: Web UI

**Goal:** Chat from phone browser.

- [ ] Serve static HTML from API server
- [ ] EventSource connection to /stream
- [ ] Input field sends POST /message
- [ ] Auto-reconnect on disconnect
- [ ] Status indicator (active/waiting/hibernated)
- [ ] Test on phone browser (iPhone Safari, Android Chrome)

**Deliverable:** Can chat with Claude Code from phone.

**Test:**
1. Open browser on phone
2. Navigate to API server URL
3. Send message
4. See live streaming response
5. Close browser
6. Reopen → should reconnect and show recent output

### Day 8: Hardening

**Goal:** Handle edge cases without crashing.

- [ ] Sprite fails to start → surface error to user
- [ ] Claude Code crashes → detect and restart
- [ ] Resume fails → show error, offer fresh start
- [ ] Connection drops during message → retry
- [ ] Multiple clients connected → all see output
- [ ] Sprite takes too long to wake → timeout and error
- [ ] Ring buffer full → evict oldest, keep most recent

**Deliverable:** MVP is stable enough to use daily.

---

## Testing the Risky Stuff

### Hibernate/Resume Test Suite

Run these tests before declaring MVP done:

1. **Basic resume:** Start session, checkpoint, restore, --resume → context intact?
2. **Multi-turn resume:** 10 back-and-forth messages, checkpoint, restore, --resume → full history?
3. **Tool state resume:** Claude uses a tool (e.g., edits file), checkpoint, restore → tool state preserved?
4. **Long idle resume:** Hibernate for 1 hour, resume → still works?
5. **Failed resume recovery:** Force --resume to fail → fallback works?

### Reconnection Test Suite

1. **Output buffering:** Start streaming, disconnect client, reconnect → replay last 10KB?
2. **Multiple clients:** Two browsers open → both see output?
3. **Disconnect during message send:** Send message, kill connection mid-request → message still queued?

---

## Config (Simple YAML)

```yaml
# config.yml
repo:
  name: "my-repo"
  url: "git@github.com:yourusername/my-repo.git"

sprites:
  base_checkpoint: "base-authed"
  repo_checkpoint: "repo-ready"
  org_slug: "personal"

timeouts:
  idle_seconds: 180        # 3 minutes
  hibernate_grace_seconds: 10
  resume_timeout_seconds: 30

server:
  port: 8080
  buffer_size_kb: 10
```

**No secrets in config.** Secrets come from environment variables:
- `ANTHROPIC_API_KEY` - already in Sprite via Claude Code auth
- `FLY_API_TOKEN` - for Sprites SDK

---

## What This MVP Proves

1. **Claude Code works in sandboxed Sprites** ✅
2. **Hibernate/resume is reliable (or we learn it's not)** ✅
3. **Mobile chat experience is usable** ✅
4. **Cost is acceptable** (3 min idle timeout should keep costs low) ✅

## What It Doesn't Prove

- Multiple repos (but architecture supports it - just add config)
- Voice input (but can add later - it's just another input method)
- Push notifications (but can add later - just POST to Ntfy on state change)
- Auth (but Tailscale is good enough for personal use)
- Native app (but PWA is testbed for that)

---

## Future Architecture Decisions Made Now

Even though MVP is minimal, design it so these future features don't require rewrites:

### ✅ Multi-repo support
- Session struct already has `Repo` field
- Just add repo selector UI and config array

### ✅ Push notifications
- State machine emits events
- Add notification handler that POSTs to Ntfy

### ✅ Voice input
- POST /message accepts text
- Voice is just transcription + POST

### ✅ Session persistence
- Session struct is serializable
- Add SQLite later without changing API

### ✅ Multiple concurrent sessions
- Session management is already per-ID
- UI needs tabs/list view

### ✅ Auth
- Add middleware that checks token or Tailscale identity
- No API changes needed

---

## Cost Estimate (MVP)

**Sprites:**
- Active session: ~$0.12/hour
- Hibernated: $0.00/hour
- With 3-min idle timeout: ~$0.06/hour average (50% active, 50% hibernated)

**Fly.io API server:**
- Shared CPU, 256MB RAM: ~$2/month
- Auto-sleeps when not in use

**Realistic monthly cost:**
- 1 hour/day coding: ~$1.80 in Sprites + $2 server = **~$4/month**
- 3 hours/day coding: ~$5.40 in Sprites + $2 server = **~$8/month**

**Not included:**
- Claude Max subscription ($100-200/month) - unavoidable cost, already paying it

---

## Success Metrics (MVP)

After 2 weeks, you should be able to:

- [ ] Start a conversation from your phone
- [ ] See live terminal output as Claude works
- [ ] Walk away for 10 minutes, come back, continue conversation
- [ ] Not lose your place if you close the browser
- [ ] Use it for a real task (fix a bug, add a feature)
- [ ] Not pay more than $10 in compute for the 2-week period

**If all these work: the core value prop is proven. Build V2.**

**If --resume is broken: pivot to longer idle timeouts or summarized context fallback.**

---

## What to Build After MVP

### Phase 2: Multi-Session (Week 3)
- Session list UI
- Multiple concurrent sessions
- Session history (add SQLite)
- Session naming

### Phase 3: Polish (Week 4)
- Voice input (Web Speech API)
- Push notifications (Ntfy)
- Repo picker (GitHub API integration)
- QR code pairing

### Phase 4: Advanced (Week 5+)
- Better resume fallback (context summarization)
- Permission prompts (parse dangerous commands)
- Chat commands (`/status`, `/hibernate`)
- Native app (Capacitor wrapper)

---

## Open Questions to Answer During MVP

1. **How reliable is --resume?** (Day 1-2)
2. **How do we detect "Claude is waiting"?** (Day 5-6)
3. **Does the Sprite checkpoint preserve everything?** (Day 5-6)
4. **How long does a Sprite take to wake?** (Day 5-6)
5. **Is 3 minutes the right idle timeout?** (Week 1-2 of usage)
6. **Is 10KB ring buffer enough for reconnection?** (Week 1-2 of usage)

---

## Risk Mitigation

| Risk | Mitigation |
|------|-----------|
| `--resume` corrupts on SIGTERM during tool execution | Only hibernate when Claude is idle (not during tool execution). Detect tool state before checkpointing. |
| `--resume` reports "No conversations found" | Store session ID yourself, pass directly to `--resume <id>` (bypass index). |
| Resume fails entirely | Detect failure, start fresh session with recent context from output buffer injected via `--append-system-prompt`. |
| Sprite wakes too slowly | Profile wake time. If >3 seconds, warn user before hibernate. Consider keeping alive longer. |
| Sprite crashes | Detect crash, restart automatically, notify user of interruption. |
| Output buffer overflows | Ring buffer with eviction. Accept that very old output is lost on reconnect. |
| Claude gets confused after resume | Add manual "start fresh" button that clears context. |
| Costs spiral | Fly.io spending alerts. Monitor Sprite usage daily during MVP. Consider 15-min timeout instead of 3-min. |

---

## Done Definition

MVP is done when:

1. You've used it for 3 real coding sessions from your phone
2. Hibernate/resume has worked correctly each time
3. You've walked away and come back without losing context
4. You haven't hit any crashes or data loss
5. Total cost for testing period is under $10

**At that point: write up what you learned and decide if V2 is worth building.**