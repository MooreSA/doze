# Claude Code Remote - TODO

## Week 1: MVP Build

### Day 1: Sprite Setup ✓
- [x] Create base Sprite with Claude Code
- [x] Authenticate via browser OAuth (Max subscription)
- [x] Test resume thoroughly
- [x] Checkpoint as "base-authed"
- [ ] Clone your main repo into Sprite
- [ ] Install dependencies (npm install, go mod download, etc.)
- [ ] Checkpoint as "repo-ready"
- [ ] Document checkpoint names and sprite ID in config.yml

**Deliverable:** Checkpointed Sprite you can restore that's ready to work

---

### Day 2: API Server Core
- [ ] Go project setup (`go mod init`)
- [ ] Add Sprites SDK dependency
- [ ] Implement session state struct (ClaudeCmd, Stdin, State, LastActivity, IdleTimer)
- [ ] POST /start endpoint - restore checkpoint, spawn `claude --cwd /workspace/repo`
- [ ] GET /stream endpoint - SSE streaming of stdout
- [ ] POST /message endpoint - write to stdin, reset idle timer
- [ ] Capture session ID from Claude Code stdout/config
- [ ] Deploy to Fly.io (or run locally for testing)

**Test:**
```bash
curl -X POST http://localhost:8080/start
curl -N http://localhost:8080/stream
curl -X POST http://localhost:8080/message -d '{"content":"list files"}'
```

**Deliverable:** Can have a conversation with Claude Code via curl

---

### Day 3: Idle Detection & Hibernate
- [ ] Parse stdout for Claude prompt patterns (readline cursor, `>`, no output for 2s)
- [ ] Implement state machine: ACTIVE → WAITING → HIBERNATING → HIBERNATED
- [ ] Start 3-minute idle timer when state becomes WAITING
- [ ] On timeout: send SIGTERM to Claude Code process
- [ ] Wait for graceful shutdown (up to 10s)
- [ ] Call Sprites SDK checkpoint
- [ ] Update state to HIBERNATED
- [ ] Add logging for all state transitions

**Test:**
- Send message, wait for response
- Verify state transitions to WAITING
- Wait 3+ minutes (or set timer to 30s for testing)
- Verify SIGTERM sent and checkpoint created
- Check Sprite is stopped (no cost)

**Deliverable:** Sessions automatically hibernate when idle

---

### Day 4: Resume Flow
- [ ] POST /message when hibernated → trigger resume
- [ ] Restore Sprite from checkpoint via SDK
- [ ] Run `claude --resume {session-id}` (pass ID directly, don't rely on list)
- [ ] Wait for Claude to be ready (detect prompt)
- [ ] Inject queued user message to stdin
- [ ] Update state to ACTIVE
- [ ] Resume streaming to SSE clients
- [ ] Add fallback: detect resume failure ("No conversations found"), log error, offer fresh start

**Test:**
- Start session, send message: "List files"
- Wait for response
- Wait for hibernate (3+ min or short timeout)
- Send new message: "Tell me about main.go"
- Verify session resumes
- Verify Claude has context from previous messages
- Check logs for session ID usage

**Deliverable:** Sessions resume seamlessly after hibernation

---

### Day 5: HTML UI
- [ ] Create single HTML page (no framework, vanilla JS)
- [ ] Terminal output div (monospace, dark theme, auto-scroll)
- [ ] Input field (mobile-friendly, font-size: 16px to prevent iOS zoom)
- [ ] Status indicator showing current state (active/waiting/hibernated)
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
- How many times did it hibernate?
- How many times did resume work vs fail?
- What was the total active time vs hibernated time?
- Estimated cost for the week
- What felt annoying? (typing? seeing output? something else?)
- What feature did you miss most?

**Deliverable:** Data to inform next steps

---

## Week 2: Decide & Iterate

### Review Session
- [ ] Review logs from Week 1 usage
- [ ] Calculate actual costs (Sprite active hours × rate)
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
- [ ] Or: add manual hibernate/resume controls

**Option C: Add most-missed feature**
- [ ] Voice input (Web Speech API)
- [ ] Notifications (Ntfy)
- [ ] Output buffer (reconnect sees recent history)
- [ ] Multi-repo support

---

## Future Features (Not MVP)

### High Priority (Week 3-4)
- [ ] Bearer token auth for public deploy
- [ ] Ntfy push notifications (Claude waiting, session hibernated)
- [ ] Output buffer / replay (10KB ring buffer for reconnection)
- [ ] Better error handling (Sprite fails to start, Claude crashes)
- [ ] Session persistence (SQLite) so server restart doesn't lose session

### Medium Priority (Week 5-6)
- [ ] Voice input (Web Speech API, tap to talk)
- [ ] GitHub repo selector (list repos via API)
- [ ] Multiple concurrent sessions
- [ ] Manual hibernate/resume controls in UI

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
- Hibernated % of time: ___%

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

sprites:
  base_checkpoint: "base-authed"
  repo_checkpoint: "repo-ready"
  org_slug: "personal"

timeouts:
  idle_seconds: 180        # 3 minutes (or 30 for testing)
  hibernate_grace: 10
  resume_timeout: 30

server:
  port: 8080
```

### .env (for local testing)
```bash
FLY_API_TOKEN=xxx
SPRITES_ORG_SLUG=personal
```
