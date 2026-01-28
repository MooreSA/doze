# Claude Code Mobile Interface

## Overview

A self-hosted mobile interface for Claude Code that enables conversational coding sessions in sandboxed cloud environments. Built for personal use across multiple repositories with a focus on security, simplicity, and avoiding third-party code access.

**This is not fire-and-forget task execution.** It's a conversational interface where you can chat back and forth with Claude Code, ask follow-up questions, provide clarification, and guide the work - all from your phone.

**Why build this instead of using existing solutions:**
- Anthropic's Claude Code iOS is buggy (tasks get stuck)
- Third-party tools (Moltbot, Happy Coder, etc.) route code through their infrastructure
- Full control over security, secrets, and workflow

**Key technical decisions:**
- Uses Claude Code CLI (not Agent SDK) with Max subscription
- CLI has better session persistence and is officially supported with Max
- Sessions hibernate when idle to avoid burning money while waiting for input

---

## Architecture

```
┌─────────────────────────────────────────────────────┐
│  Phone                                              │
│  PWA → Capacitor (future native wrapper)            │
│  - Chat UI with voice input                         │
│  - Live terminal stream                             │
│  - Task list                                        │
└─────────────────┬───────────────────────────────────┘
                  │ HTTPS (Tailscale or token auth)
                  ▼
┌─────────────────────────────────────────────────────┐
│  API Server (Go on Fly.io)                          │
│  - Task management                                  │
│  - GitHub API integration                           │
│  - Sprites orchestration                            │
│  - SSE streaming                                    │
│  - Ntfy push notifications                          │
└─────────────────┬───────────────────────────────────┘
                  │ Sprites SDK
                  ▼
┌─────────────────────────────────────────────────────┐
│  Sprites (Fly.io)                                   │
│  - Base sprite (checkpointed dev environment)       │
│  - Per-repo sprites (cloned from base)              │
│  - Claude Code CLI running autonomously             │
│  - Network egress locked to allowlist               │
└─────────────────────────────────────────────────────┘
```

---

## Components

### 1. PWA Frontend

**Purpose:** Mobile-friendly chat interface for conversing with Claude Code.

**Core Features:**
- Session list (active, hibernated, recent)
- Repo selector when starting new session (dynamic list from GitHub)
- Create new repo option
- Chat interface with live terminal output stream (SSE)
- Voice input (tap to talk)
- Visual indicator: Claude working vs. waiting for you
- Reconnection: if you leave and come back, catch up on what you missed
- Hibernate/resume controls
- **QR code pairing:** Scan code from terminal to authenticate (no typing tokens)
- **Offline message queue:** Messages queued locally if connection drops
- **Chat commands:** `/status`, `/new`, `/compact` processed in-band

**Voice (V1):**
- Web Speech API for input (on-device, free)
- Browser speechSynthesis for TTS output

**Voice (V2):**
- Whisper API for transcription
- OpenAI TTS for output
- **Voice agent intermediary:** Sonnet helps refine vague ideas before sending to Claude Code

**Future:** Wrap in Capacitor for native iOS push, better voice APIs, haptics.

---

### 2. API Server (Go)

**Purpose:** Orchestrates everything. The brain.

**Endpoints:**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/repos` | List repos from GitHub |
| POST | `/repos` | Create new repo |
| GET | `/sessions` | List sessions (active, hibernated, recent) |
| POST | `/sessions` | Start new session (select repo) |
| GET | `/sessions/{id}` | Get session details |
| GET | `/sessions/{id}/stream` | SSE stream of terminal output |
| POST | `/sessions/{id}/message` | Send message to Claude |
| POST | `/sessions/{id}/hibernate` | Force hibernate |
| POST | `/sessions/{id}/resume` | Wake and resume |
| POST | `/sessions/{id}/end` | End session |

**Responsibilities:**
- Authenticate requests (Tailscale identity or token)
- Manage Sprite lifecycle (create, wake, checkpoint, hibernate)
- Inject secrets into Sprite environment
- Buffer terminal output for reconnection
- Detect idle state, trigger hibernate after timeout
- Send push notifications via Ntfy
- Store session metadata
- Handle resume logic (including fallback if `--resume` fails)

**Tech:**
- Go (good Sprites SDK, handles streaming well)
- SQLite or Turso for session metadata
- Fly.io hosting (auto-sleep, cheap)

---

### 3. Sprites (Sandboxed Compute)

**Purpose:** Isolated, persistent Linux VMs where Claude Code actually runs.

**Sprite Strategy:**

```
base-sprite (checkpointed after setup + Claude Code auth)
├── Dev tools (node, go, git, etc.)
├── Shell config
├── Global packages
├── Claude Code CLI (authenticated with Max subscription)
└── ~/.claude/ credentials persisted

     ↓ clone + restore

repo-sprite-cornerstone (checkpointed after setup)
├── Cloned repo
├── Dependencies installed
└── Ready to work

repo-sprite-side-project (checkpointed after setup)
├── ...
```

**Initial Auth Setup (one time):**
1. Create base Sprite
2. Install Claude Code and dev tools
3. Run `claude` interactively, complete browser OAuth with Max account
4. Credentials saved to `~/.claude/`
5. Checkpoint → "base-authed"
6. All future Sprites restore from this checkpoint (already authenticated)

Note: OAuth tokens appear to be long-lived. If they expire, re-auth and re-checkpoint.

**Lifecycle:**
1. First time using a repo → restore from base-authed, clone repo, install deps, checkpoint
2. Subsequent sessions → restore from repo checkpoint, already ready
3. Sprites hibernate after 30s idle (no cost)
4. Wake in ~1 second when needed

**Network Policy (egress allowlist):**
- `api.anthropic.com` (Claude API)
- `github.com` / `api.github.com`
- `registry.npmjs.org`
- `proxy.golang.org`
- Other package registries as needed

**Security:**
- No inbound connections
- Can't reach arbitrary internet
- Secrets injected at runtime, not persisted (except Claude auth)

---

### 4. GitHub Integration

**Authentication:**
- Fine-grained Personal Access Token
- Scoped to specific repos only
- Stored in Fly Secrets, injected at runtime

**Capabilities:**
- List user's repos
- Create new repos
- Clone/pull/push to allowed repos
- Open pull requests
- Read issues/PRs (for context)

**Branch Protection (configured on GitHub):**
- `main` branch protected
- Require PR to merge
- Require 1 approval (you)
- Claude cannot push directly to main or merge

**Workflow Enforcement:**
Claude Code is instructed to:
- Create branch: `claude/task-{id}-{short-slug}`
- Commit messages include: `[task:{id}]`
- PR body references task ID

---

### 5. Session Management

**This is conversational, not task-based.** You chat with Claude Code, it responds, you follow up. Sessions can last minutes or days.

**Session Lifecycle:**

```
created → active (Claude working or waiting) → hibernated → resumed
                                            → ended
```

**The Idle Problem:**

Claude Code blocks on stdin waiting for your response. If you walk away, the Sprite stays awake burning money. Solution:

**Idle Timeout Flow:**
1. Claude responds or asks a question
2. Start idle timer (configurable, default 5 minutes)
3. If you send a message, reset timer, continue conversation
4. If timer expires:
   - Send SIGTERM to Claude Code (graceful shutdown)
   - Claude saves session state to `~/.claude/projects/.../session.jsonl`
   - Sprite hibernates automatically (free)
   - Push notification: "Session paused - waiting for you"
5. When you come back and send a message:
   - Sprite wakes (~1 second)
   - Run `claude --resume <session-id>` 
   - Inject your new message
   - Conversation continues

**Resume Fallback:**

There's a known bug where `--resume` sometimes doesn't restore full context. If this happens:
- Session history exists in `.jsonl` files on disk
- Fallback: parse the `.jsonl`, inject conversation via `--append-system-prompt`
- Or: summarize previous context and start fresh

**Stored per session:**

```
Session {
  id
  repo
  sprite_name
  claude_session_id (from ~/.claude/)
  branch (if created)
  pr_url (if opened)
  status: active | hibernated | ended
  last_activity_at
  started_at
  ended_at
  
  // Origin metadata (stolen from Moltbot)
  origin {
    repo_name
    repo_url
    branch_started_from
    initial_prompt (truncated)
  }
}
```

**What lives where:**
- Conversation history: `~/.claude/` in the Sprite (survives hibernate via checkpoint)
- Session metadata: SQLite in API server
- Terminal output buffer: Memory in API server (for reconnection during active session)
- Code changes: Git (the source of truth)

---

### 6. Notifications (Ntfy)

**Purpose:** Push notifications to phone without App Store complexity.

**Triggers:**
- Claude responded (waiting for your input)
- Session hibernated (idle timeout)
- Session errored
- PR opened
- Long-running operation complete (tests passed, build done)
- **Tests passed/failed** (parse stdout for test results)
- **Lint errors found** (parse stdout)
- **Permission needed** (Claude wants to do something risky)

**Implementation:**
- Subscribe to a private topic: `ntfy.sh/seamus-claude-{random}`
- API server POSTs to topic on state changes
- Parse Claude's stdout for known patterns (test results, errors, etc.)
- Phone receives push via Ntfy app

**Message format:**
```
Title: Tests Failed ❌
Body: cornerstone-api: 3 tests failed in auth_test.go
Click: deep link to session in PWA
```

```
Title: PR Ready for Review
Body: cornerstone-api: #42 "Add retry logic to webhook handler"
Click: deep link to GitHub PR
```

---

### 7. Secrets Management

**Flow:**

```
Fly Secrets (encrypted at rest)
    ↓ runtime injection
API Server environment
    ↓ passed per-command
Sprite (env vars, ephemeral)
```

**Secrets needed:**
- `GITHUB_TOKEN` - fine-grained PAT
- `ANTHROPIC_API_KEY` - for Claude Code
- `NTFY_TOPIC` - push notification topic
- `AUTH_TOKEN` - for API authentication (if not using Tailscale)

**Principles:**
- Secrets never written to Sprite filesystem
- Exist only for lifetime of Claude Code process
- Rotatable without redeploying

---

## Security Model

**Threat:** Prompt injection via malicious code in repo, issue, or PR comment.

**Mitigations:**

| Layer | Protection |
|-------|------------|
| GitHub token | Fine-grained, scoped to specific repos only |
| Branch protection | Claude can't push to main, only open PRs |
| Network egress | Sprite can only reach allowlisted domains |
| Secrets | Not persisted, injected at runtime only |
| Blast radius | Worst case: bad PR opened, you reject it |

**Accepted risks:**
- Claude could mess up branches on allowed repos
- Claude could open spam PRs
- Claude could read all code in allowed repos

**Not possible:**
- Access repos outside the allowlist
- Push to protected branches
- Exfiltrate data to arbitrary servers
- Access production credentials (they're never provided)

---

## Cost Estimate

**Sprites:**
- ~$0.50 per 4-hour intensive session
- Hibernated sprites are free
- Storage: ~$0.00068/GB-hour (negligible)

**API Server (Fly.io):**
- Small shared machine: ~$2-3/month
- SQLite volume: ~$0.15-0.45/month

**Monthly scenarios:**

| Usage | Sprites | Fly | Total |
|-------|---------|-----|-------|
| Light (2-3 sessions/week) | ~$2 | ~$3 | ~$5/month |
| Moderate (daily 1-2hr) | ~$15 | ~$3 | ~$18/month |
| Heavy (4+ hrs/day) | ~$50 | ~$3 | ~$55/month |

**Not included:**
- Claude Max subscription ($100-200/month) - the real cost
- Ntfy - free
- GitHub - free for private repos

---

## Version Roadmap

### V1 - PWA

- [ ] Go API server on Fly.io
- [ ] Sprites integration (base-authed + per-repo)
- [ ] Claude Code CLI with Max subscription auth
- [ ] Session management with idle timeout + hibernate
- [ ] Resume sessions with `--resume` (with fallback)
- [ ] PWA with chat UI
- [ ] Live terminal streaming (SSE)
- [ ] Output buffer for reconnection
- [ ] GitHub repo list + selection
- [ ] Voice input (Web Speech API)
- [ ] Ntfy push notifications
- [ ] Branch naming convention enforced
- [ ] **QR code pairing** (scan to authenticate mobile)
- [ ] **Chat commands:** `/status`, `/new`, `/compact`
- [ ] **Session origin metadata** (repo, branch, start time)
- [ ] **Offline message queue** (service worker)
- [ ] **Rich notifications** (parse stdout for test results, PR opened)

### V2 - Polish

- [ ] Better voice (Whisper + OpenAI TTS)
- [ ] Create new repos from UI
- [ ] Session search/history
- [ ] Multiple concurrent sessions
- [ ] Hybrid view (summary + expandable terminal)
- [ ] **Permission prompts** (approve risky operations from mobile)
- [ ] **Voice agent intermediary** (Sonnet refines vague ideas)
- [ ] **Skills system** (select at session start)
- [ ] **Device handoff** (press key on desktop to transfer control)
- [ ] **Daily reset option** (fresh context at 4am)

### V3 - Native

- [ ] Capacitor wrapper for iOS
- [ ] Native push notifications
- [ ] Native voice input
- [ ] Haptics on events
- [ ] TestFlight distribution
- [ ] **Cron/heartbeat tasks** (scheduled recurring jobs)
- [ ] **`doctor` health check** (diagnose configuration issues)
- [ ] **Context visibility** (`/context list` command)

---

## Build Chunks

Sequential pieces to build, each delivering working functionality.

### Chunk 1: Sprite + Claude Code (1-2 days)

**Goal:** Prove you can run Claude Code in a Sprite and interact with it.

- [ ] Create Fly.io account, get Sprites access
- [ ] Create base Sprite manually via CLI
- [ ] Install Claude Code in Sprite
- [ ] Authenticate Claude Code (browser OAuth with Max)
- [ ] Checkpoint as "base-authed"
- [ ] Test: restore from checkpoint, run `claude --version`, confirm still authed
- [ ] Test: run a simple Claude Code command, see output

**Deliverable:** A checkpointed Sprite you can restore that has Claude Code ready to go.

### Chunk 2: API Server Skeleton (1-2 days)

**Goal:** Go server that can talk to Sprites.

- [ ] Go project setup
- [ ] Sprites SDK integration
- [ ] Single endpoint: `POST /sessions` - creates a session, starts Claude Code in Sprite
- [ ] Single endpoint: `GET /sessions/{id}/stream` - SSE stream of stdout
- [ ] Single endpoint: `POST /sessions/{id}/message` - sends stdin to Claude
- [ ] Deploy to Fly.io
- [ ] Test with curl

**Deliverable:** API you can hit from terminal to have a conversation with Claude Code.

### Chunk 3: Minimal Web UI (1-2 days)

**Goal:** Chat interface that works on your phone.

- [ ] Simple HTML/JS page (no framework needed for V1)
- [ ] Connect to SSE stream, display output
- [ ] Text input, send messages via POST
- [ ] Deploy as static site (Fly.io or wherever)
- [ ] Test on phone browser

**Deliverable:** You can chat with Claude Code from your phone.

### Chunk 4: Session Persistence + Reconnection (1-2 days)

**Goal:** Don't lose context when you switch apps.

- [ ] SQLite for session metadata
- [ ] Output buffer in memory
- [ ] On reconnect, replay buffer then continue stream
- [ ] Session list endpoint
- [ ] UI: show active sessions, tap to reconnect

**Deliverable:** You can leave the app, come back, and continue where you left off.

### Chunk 5: Idle Detection + Hibernate (1-2 days)

**Goal:** Stop burning money when idle.

- [ ] Detect "Claude is waiting for input" state (parse stdout)
- [ ] Start idle timer on that state
- [ ] On timeout: SIGTERM Claude, checkpoint Sprite, mark session hibernated
- [ ] Resume endpoint: restore checkpoint, `claude --resume`
- [ ] Test the resume bug - implement fallback if needed
- [ ] UI: show hibernated state, resume button

**Deliverable:** Sessions auto-hibernate and can be resumed.

### Chunk 6: GitHub Integration (1 day)

**Goal:** Pick repos dynamically.

- [ ] GitHub PAT stored in Fly secrets
- [ ] `GET /repos` - list user's repos via GitHub API
- [ ] Inject GitHub token into Sprite for clone/push
- [ ] UI: repo picker when starting session
- [ ] Clone repo into Sprite on session start

**Deliverable:** Start a session on any of your repos.

### Chunk 7: Push Notifications (half day)

**Goal:** Know when Claude needs you.

- [ ] Ntfy integration in API server
- [ ] Send notification on: Claude waiting, session hibernated, error
- [ ] Install Ntfy app on phone, subscribe to topic
- [ ] Test end-to-end

**Deliverable:** Phone buzzes when Claude needs attention.

### Chunk 8: Voice Input (1 day)

**Goal:** Talk instead of type.

- [ ] Web Speech API integration in PWA
- [ ] Mic button, tap to record
- [ ] Transcribe, send as message
- [ ] Handle mobile browser quirks

**Deliverable:** You can speak your prompts.

### Chunk 9: Per-Repo Sprites (1 day)

**Goal:** Fast startup for repos you use often.

- [ ] After first session on a repo, checkpoint as "repo-{name}"
- [ ] On subsequent sessions, restore from repo checkpoint (deps already installed)
- [ ] Manage checkpoint lifecycle (don't accumulate forever)

**Deliverable:** Second session on a repo starts fast.

### Chunk 10: Hardening (ongoing)

- [ ] Auth (Tailscale or token)
- [ ] Rate limiting
- [ ] Fly spending alerts
- [ ] Error handling
- [ ] Logging
- [ ] Branch protection reminders in UI

---

## Ideas Stolen from Moltbot and Happy Coder

After researching these tools, here are features worth stealing:

### From Happy Coder (Mobile Claude Code Client)

**1. QR Code Pairing**
Instead of typing tokens on your phone, scan a QR code displayed in terminal. The QR encodes a shared secret for end-to-end encryption. Zero configuration on mobile.

**2. Device Handoff with Single Keypress**
When you sit down at your laptop, press any key to "take control" back from mobile. The phone shows "session transferred to desktop." When you walk away, phone automatically reconnects. No explicit resume needed.

**3. Offline Message Queue**
If you type a message while in a tunnel or on flaky connection, it queues locally and delivers when connection returns. Your input never gets lost.

**4. Permission Prompts for Sensitive Operations**
When Claude Code wants to do something risky (delete files, run destructive commands, call external APIs via MCP), the mobile app shows an Allow/Deny prompt before it executes. Gives you a chance to review even when away from terminal.

**5. Voice Agent as Intermediary**
Rather than just transcribing voice to text, use a lightweight LLM (Sonnet) as an intermediary that:
- Helps you think through vague ideas conversationally
- Converts rambling stream-of-consciousness into concrete Claude Code prompts
- Only sends to Claude Code once you confirm the refined request

**6. Real-Time Sync (No "Primary" Device)**
Both desktop and mobile can initiate, send, and receive. There's no concept of a "main" device - they're both equal participants in the same session.

### From Moltbot (Personal AI Gateway)

**1. Skills System**
Modular skills in `SKILL.md` format that inject context and capabilities. Example: a "refactoring" skill, a "testing" skill, a "documentation" skill. Select at session start or switch mid-conversation.

**2. Daily Session Reset**
Sessions auto-expire at a configurable time (default 4am). Fresh context each day. Prevents sessions from accumulating stale context forever.

**3. Idle vs Daily Reset (Whichever First)**
Combine idle timeout (your 5 min) with daily reset. Session ends on whichever comes first. Keeps things fresh.

**4. Chat Commands**
In-band commands without leaving the chat:
- `/status` - show session state, token usage, model
- `/new` or `/reset` - fresh session
- `/compact` - summarize older context to free up window
- `/think high` - toggle thinking mode (for Claude Code, this maps to --think flag)

**5. Context Visibility**
`/context list` shows what's in the system prompt and biggest context contributors. Helps debug why Claude is confused.

**6. Session Origin Metadata**
Track where each session came from - which repo, when started, what branch. Useful for reviewing history.

**7. Proactive Notifications**
Not just "Claude waiting" but:
- Tests passed/failed
- Build completed
- PR opened
- Error in logs
Parse Claude's output for these events and notify.

**8. Heartbeat/Cron Integration**
Schedule recurring tasks: "Every morning at 9am, pull latest main and run tests." Agent wakes, does the job, notifies you, hibernates.

**9. `moltbot doctor` Health Checks**
CLI command that checks configuration, permissions, API connectivity, disk space. Surfaces problems before you hit them.

### Feature Priority for This Project

**Add to V1 (simple, high value):**
- [ ] QR code pairing for mobile auth
- [ ] Chat commands: `/status`, `/new`, `/compact`
- [ ] Session origin metadata (repo, branch, start time)
- [ ] Offline message queue (PWA with service worker)
- [ ] Richer notifications (tests passed, PR opened - parse stdout)

**Add to V2 (more complex):**
- [ ] Permission prompts for risky operations
- [ ] Voice agent intermediary (Sonnet for refinement)
- [ ] Skills system (select at session start)
- [ ] Device handoff (press key to transfer)
- [ ] Daily reset option

**Add to V3 (advanced):**
- [ ] Cron/heartbeat scheduled tasks
- [ ] `doctor` health check command
- [ ] Context visibility (`/context list`)

---

## Ideas Stolen From Moltbot & Happy Coder

These tools solve similar problems. Here's what to borrow:

### From Happy Coder

**1. Instant Device Switching**
Happy lets you start on desktop, pick up on phone, switch back with one keypress. The "primary" device concept is fluid.

**Steal:** When you're at your computer and want to take over from the phone session, just start typing - the phone becomes passive viewer. When you walk away, phone takes over automatically.

**Implementation:** Track "active input device" - whichever device sent the last message is the controller. Other devices become read-only viewers of the stream. No explicit handoff needed.

**2. Voice Agent as Intermediary**
Happy has a "rubber duck" voice agent (Claude Sonnet) that you can brainstorm with BEFORE committing to a Claude Code execution. It helps refine your prompt.

**Steal:** Add a "planning mode" voice conversation that helps you think through what you want before wasting tokens on a full Claude Code execution.

**Implementation:** Lightweight Claude Haiku call to refine the prompt. "I want to add caching to the API" → agent asks clarifying questions → generates a crisp prompt for Claude Code.

**3. Permission Prompts for Sensitive Operations**
Happy intercepts MCP tool calls and file edits, shows mobile user Allow/Deny before proceeding.

**Steal:** For dangerous operations (deleting files, running migrations, pushing to remote), show a confirmation prompt on phone before proceeding.

**Implementation:** Claude Code's `--allowlist` and `--denylist` flags. Parse stdout for tool calls matching a "needs approval" list. Pause execution, push notification, wait for approval via API.

**4. Offline-First with Encrypted Pub/Sub**
Happy queues commands even if connection drops. Messages are reliably delivered.

**Steal:** Messages should be durable. If you type on the train and lose signal, the message waits and sends when reconnected.

**Implementation:** Client-side queue with retry. Server acknowledges receipt. Idempotency keys prevent duplicate sends.

### From Moltbot

**1. Multi-Channel Delivery (Not Just Push)**
Moltbot can send to WhatsApp, Telegram, Slack, Discord, iMessage, etc. Notifications arrive where you already are.

**Steal:** Let users pick their notification channel. Some people live in Slack, others in iMessage.

**Implementation (V2+):** Abstract notification delivery. Start with Ntfy, add Telegram bot as second channel. User configures preferred channel in settings.

**2. WebSocket Control Plane**
Moltbot uses WebSocket for real-time bidirectional control between Gateway and all clients. Typed protocol with JSON Schema validation.

**Steal:** SSE is fine for server→client streaming, but WebSocket would enable richer real-time features (typing indicators, presence, instant command acknowledgment).

**Implementation (V2+):** Upgrade from SSE to WebSocket. Protocol includes: `connect`, `message`, `status`, `typing`, `error`. JSON frames with type field.

**3. Session Compaction / Summary**
Moltbot has `/compact` command that summarizes session context to reduce token usage.

**Steal:** When resuming a hibernated session, optionally summarize the previous context instead of replaying full history.

**Implementation:** Before resume, offer "Full context" vs "Summary" option. Summary uses Claude to compress prior conversation into a few paragraphs injected via `--append-system-prompt`.

**4. Skills / Agent Templates**
Moltbot has a skills system - reusable prompt templates for common tasks like "review PR", "write tests", "refactor".

**Steal:** Pre-built session templates for common workflows.

**Implementation:** Templates stored as markdown files. "New Session" UI shows template picker. Template becomes initial system prompt context.

Example templates:
- **Code Review:** "Review the changes in this PR. Focus on security, performance, and maintainability."
- **Test Writer:** "Write comprehensive tests for the module I specify. Include edge cases."
- **Refactor:** "Refactor for readability and maintainability. Explain each change."
- **Bug Hunt:** "Help me debug this issue. Ask clarifying questions before making changes."

**5. Chat Commands**
Moltbot has `/status`, `/new`, `/reset`, `/think`, `/verbose` commands inline in chat.

**Steal:** Slash commands that control session behavior without leaving the chat interface.

**Implementation:** Parse messages starting with `/`. Commands:
- `/status` - show current session info, token usage
- `/reset` - clear context, start fresh (within same repo)
- `/hibernate` - manually trigger hibernate
- `/branch <name>` - switch to different branch
- `/model <name>` - change model (if supported)

**6. Presence / Typing Indicators**
Moltbot shows when the agent is "typing" or processing.

**Steal:** Visual feedback for "Claude is thinking" vs "Claude is writing" vs "Claude is waiting".

**Implementation:** Parse Claude Code stdout for progress indicators. Map to UI states:
- Spinner: Claude is thinking (no output yet)
- Animated dots: Claude is streaming response
- Waiting icon: Claude asked a question, awaiting your input
- Checkmark: Claude completed the task

**7. Health Checks / Doctor Command**
Moltbot has `moltbot doctor` that diagnoses issues.

**Steal:** Diagnostic endpoint that checks if everything is healthy.

**Implementation:** `GET /health` returns:
- API server status
- Sprite availability
- GitHub token validity
- Claude Code auth status
- Last successful session
- Any errors in last hour

---

## Updated Roadmap with Stolen Ideas

### V1 - Core (unchanged)
All existing V1 items...

### V1.5 - Quick Wins from Stolen Ideas
- [ ] Slash commands: `/status`, `/reset`, `/hibernate`
- [ ] Visual presence states (thinking/writing/waiting)
- [ ] Durable message queue (offline-tolerant)
- [ ] Health check endpoint

### V2 - Polish + Stolen Features
- [ ] Voice planning mode (brainstorm before execute)
- [ ] Session templates / skills
- [ ] Permission prompts for dangerous operations
- [ ] Session compaction option on resume
- [ ] Alternative notification channels (Telegram)
- [ ] Device switching (phone/desktop fluid handoff)

### V3 - Native + Advanced
- [ ] Capacitor iOS wrapper
- [ ] WebSocket upgrade (from SSE)
- [ ] Multi-agent routing (multiple Claude Code instances)
- [ ] Full skill/template marketplace

---

## Open Items / Future Considerations

- **Resume bug:** Claude Code `--resume` has a known issue. Monitor for fix, have fallback ready.
- **OAuth token expiry:** Test how long Claude Code auth lasts. Re-auth and re-checkpoint if needed.
- **Multiple concurrent sessions:** V1 supports it at Sprite level, UI needs design work.
- **Session templates:** Common starting prompts saved for reuse.
- **MCP integration:** Claude Code supports MCP servers, could add custom tools.
- **Cost monitoring:** Alert if Sprite usage exceeds threshold.
- **Audit logging:** Full record of what Claude did (beyond git history).
- **Shared Sprites:** Multiple repos in one Sprite to save checkpoint overhead (tradeoff: less isolation).
- **TTS output:** Dropped from V1 for simplicity, add in V2 if voice input feels incomplete without it.
- **End-to-end encryption:** Happy uses E2E encryption so even their relay can't read your code. Consider for self-hosted version if sharing with others.
- **Hemingway Technique:** Happy documents a workflow where you dictate big-picture ideas on the go, then refine at desktop. Good use case to optimize for.