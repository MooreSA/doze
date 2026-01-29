# Claude Code Remote - TODO

## Week 1: MVP Build

### Day 1: Environment Setup ✓
- [x] Go project setup (`go mod init`)
- [x] Test Claude Code session resume locally
- [x] Verify session persistence in ~/.claude/
- [x] Clone/access your main repo for testing

**Deliverable:** Local environment ready for development ✓

---

### Day 2: API Server Core ✓
- [x] Implement session state struct (ClaudeCmd, Stdin, State, LastActivity, IdleTimer)
- [x] POST /start endpoint - spawn claude with stream-json mode
- [x] GET /stream endpoint - SSE streaming of stdout
- [x] POST /message endpoint - write to stdin, reset idle timer
- [x] Capture session ID from Claude Code stdout/config
- [ ] Deploy to Fly.io (or run locally for testing)

**Test:**
```bash
curl -X POST http://localhost:8080/start
curl -N http://localhost:8080/stream
curl -X POST http://localhost:8080/message -d '{"content":"list files"}'
```

**Deliverable:** Can have a conversation with Claude Code via curl

---

### Day 3: Idle Detection & Shutdown
- [ ] Parse stdout for Claude prompt patterns (readline cursor, `>`, no output for 2s)
- [ ] Implement state machine: ACTIVE → WAITING → SHUTTING_DOWN → STOPPED
- [ ] Start 3-minute idle timer when state becomes WAITING
- [ ] On timeout: send SIGTERM to Claude Code process
- [ ] Wait for graceful shutdown (up to 10s), then SIGKILL if needed
- [ ] Update state to STOPPED
- [ ] Add logging for all state transitions

**Test:**
- Send message, wait for response
- Verify state transitions to WAITING
- Wait 3+ minutes (or set timer to 30s for testing)
- Verify SIGTERM sent and process stops
- Check Claude Code process is no longer running

**Deliverable:** Sessions automatically stop when idle (no background cost)

---

### Day 4: Resume Flow
- [ ] POST /message when STOPPED → trigger resume
- [ ] Spawn new Claude process: `claude --resume {session-id}`
- [ ] Wait for Claude to be ready (detect prompt)
- [ ] Inject queued user message to stdin
- [ ] Update state to ACTIVE
- [ ] Resume streaming to SSE clients
- [ ] Add fallback: detect resume failure ("No conversations found"), log error, offer fresh start

**Test:**
- Start session, send message: "List files"
- Wait for response
- Wait for idle timeout (3+ min or 30s for testing)
- Verify process stops
- Send new message: "Tell me about main.go"
- Verify new Claude process starts with --resume
- Verify Claude has context from previous messages
- Check logs for session ID usage

**Deliverable:** Sessions resume seamlessly after idle shutdown
**Note:** Session persistence relies on Claude's ~/.claude/ storage, code changes use git

---

### Day 5: HTML UI
- [ ] Create single HTML page (no framework, vanilla JS)
- [ ] Terminal output div (monospace, dark theme, auto-scroll)
- [ ] Input field (mobile-friendly, font-size: 16px to prevent iOS zoom)
- [ ] Status indicator showing current state (active/waiting/stopped)
- [ ] EventSource connection to GET /stream
- [ ] POST to /message on Enter key
- [ ] Auto-reconnect on disconnect
- [ ] Serve static HTML from Go server

**Test on actual phone:**
- [ ] Open in Safari (iOS)
- [ ] Open in Chrome (Android)
- [ ] Send message, see streaming response
- [ ] Close browser, wait 3+ min
- [ ] Reopen, send message, verify resume works
- [ ] Check if keyboard behavior is acceptable

**Deliverable:** Can chat with Claude Code from your phone

---

### Day 6-7: Real Usage & Measurement
- [ ] Use for actual coding task #1
- [ ] Use for actual coding task #2
- [ ] Use for actual coding task #3

**Track in a notes file:**
- How many times did it stop due to idle?
- How many times did resume work vs fail?
- What was the total active time vs stopped time?
- Estimated cost for the week (API usage only)
- What felt annoying? (typing? seeing output? something else?)
- What feature did you miss most?

**Deliverable:** Data to inform next steps

---

## Week 2: Decide & Iterate

### Review Session
- [ ] Review logs from Week 1 usage
- [ ] Calculate actual costs (Claude API usage only)
- [ ] Check resume reliability (success rate)
- [ ] List pain points by severity

### Decision Point
Based on Week 1 data, choose next priority:

**Option A: MVP is good enough, deploy properly**
- [ ] Add bearer token auth
- [ ] Deploy to public Fly.io URL
- [ ] Use for another week

**Option B: Resume needs work**
- [ ] Implement graceful fallback (fresh start with context)
- [ ] Or: increase idle timeout to 10-15 min
- [ ] Or: add manual stop/resume controls

**Option C: Add most-missed feature**
- [ ] Voice input (Web Speech API)
- [ ] Notifications (Ntfy)
- [ ] Output buffer (reconnect sees recent history)
- [ ] Multi-repo support

---

## Architecture Notes

**Simplified Model:**
- Claude Code runs as local process (no Sprites/VMs needed)
- Session state: stored by Claude in ~/.claude/
- Code changes: persisted via git commits
- On idle: process stops (no cost)
- On resume: spawn fresh Claude process with `--resume {session-id}`

**Cost Model:**
- Only pay for active API usage (no idle VM costs)
- Host server can be cheap (just routing I/O)

---

## Future Features (Not MVP)

### High Priority (Week 3-4)
- [ ] Bearer token auth for public deploy
- [ ] Ntfy push notifications (Claude waiting, session stopped)
- [ ] Output buffer / replay (10KB ring buffer for reconnection)
- [ ] Better error handling (Claude crashes, resume fails)
- [ ] Session metadata persistence (SQLite) so server restart knows last session ID

### Medium Priority (Week 5-6)
- [ ] Voice input (Web Speech API, tap to talk)
- [ ] GitHub repo selector (list repos via API)
- [ ] Multiple concurrent sessions
- [ ] Manual stop/resume controls in UI

### Low Priority (Later)
- [ ] QR code pairing
- [ ] Session history/search
- [ ] Chat commands (/status, /reset)
- [ ] Better voice (Whisper + OpenAI TTS)
- [ ] Native app wrapper (Capacitor)

---

## Notes & Learnings

### Resume Test Results
- Tested scenarios: [fill in after Day 1]
- Edge cases found: [fill in]
- Reliability: [fill in]

### Cost Tracking
- Week 1 total: $___
- Average per session: $___
- Stopped % of time: ___%
- Note: Cost = Claude API usage only (no VM/Sprite costs)

### Pain Points
1. [fill in after real usage]
2.
3.

### Ideas for V2
- [add as you discover them]

---

## Config Files Needed

### config.yml
```yaml
repo:
  name: "my-repo"
  path: "/workspace/my-repo"

claude:
  binary: "claude"        # or full path to claude binary
  working_dir: "/workspace/my-repo"

timeouts:
  idle_seconds: 180        # 3 minutes (or 30 for testing)
  shutdown_grace: 10       # wait for SIGTERM before SIGKILL
  resume_timeout: 30

server:
  port: 8080
  bearer_token: ""         # leave empty for no auth (dev only)
```

### .env (for local testing)
```bash
# No external dependencies needed for MVP
# Git credentials should already be configured on the host
```
