# Doze Architecture

## Overview

Doze is a remote Claude Code interface designed for coding from mobile devices with cost-optimized infrastructure.

## MVP Architecture (Single-Session)

```
Phone Browser
    ↓ HTTPS
Fly.io (Doze Go + React UI) ← Auto-scales to 0 when idle
    ↓ Sprites.dev API
Single Sprite (ephemeral) ← Created on demand, destroyed on idle
```

## Cost Model

**When coding:**
- Fly.io: ~$0.01/hour
- Sprite: ~$0.10/hour
- **Total: ~$0.11/hour**

**When idle:**
- Everything scales to 0
- **Total: $0/hour**

**Expected monthly cost:** $5-10 (based on 2 hours/day average usage)

## User Flow

1. **Open phone** → `https://doze.fly.dev`
2. **Fly.io wakes** (2 seconds if asleep)
3. **Enter auth token** (stored in localStorage)
4. **Click "Wake Up"** → Doze API calls Sprites.dev:
   - Creates Sprite from checkpoint (with Claude Code pre-logged-in)
   - Clones doze repo in Sprite
   - Starts Claude Code process
   - Returns connection details
5. **SSE connection** established: Phone → Fly.io → Sprite
6. **Code from phone** → Messages proxied through Fly.io to Claude in Sprite
7. **Close phone / 30s idle**:
   - Doze detects idle timeout
   - Destroys Sprite via API
   - Closes SSE connection
   - Fly.io scales to 0

## Components

### 1. Fly.io Container (Doze API)

**Responsibilities:**
- Serve React UI
- Authenticate requests (bearer token)
- Manage Sprite lifecycle (create/destroy)
- Proxy commands between phone and Sprite
- Stream SSE from Sprite to phone
- Auto-scale to 0 when no requests

**Stack:**
- Go HTTP server
- React + TypeScript UI (built to static files)
- Sprites.dev API client

**Endpoints:**
- `GET /health` - Health check for monitoring
- `GET /status` - Current session status
- `POST /start` - Create Sprite and start session
- `GET /stream` - SSE connection to Sprite output
- `POST /message` - Send message to Claude (proxied to Sprite)
- `GET /` - Serve React UI

### 2. Sprite Instance (Ephemeral Workspace)

**Responsibilities:**
- Run Claude Code process
- Host git repository
- Execute tool calls (Read, Write, Edit, Bash, etc.)
- Stream output to Doze API

**Lifecycle:**
- Created from checkpoint (pre-configured with logged-in Claude)
- Lives only while actively coding
- Destroyed after 30 seconds of inactivity
- Fully ephemeral (no persistent state)

**Pre-configured (via checkpoint):**
- Claude Code installed
- Logged in with Claude Max subscription
- Ready to clone repos and start coding

### 3. Phone Browser

**Responsibilities:**
- Render UI
- Store auth token (localStorage)
- Maintain SSE connection
- Display messages and tool usage
- Handle reconnection

**Features:**
- Responsive design (mobile-first)
- Touch-optimized input
- Real-time message streaming
- Tool usage visualization
- File change tracking
- Slash command autocomplete (`/clear`, `/compact`, `/help`)

## Authentication

**Bearer Token (MVP):**
- Simple token stored in Fly.io secrets
- Frontend stores in localStorage
- Added to all HTTP requests
- No user management (single user for MVP)

**Environment variables:**
```bash
DOZE_AUTH_TOKEN=your-secret-token
SPRITES_API_KEY=your-sprites-api-key
```

## Sprites Integration

### Checkpoint Setup (One-time)

```bash
# 1. Create Sprite
sprites create doze-setup

# 2. SSH in
sprites ssh doze-setup

# 3. Install Claude Code
curl -fsSL https://claude.ai/install.sh | sh

# 4. Login with Max subscription
claude login  # Opens browser for OAuth

# 5. Verify
claude "test message"

# 6. Create checkpoint
exit
sprites checkpoint create doze-setup --name "claude-logged-in"
```

### Session Lifecycle

**Start:**
```go
spriteID := sprites.Create("claude-logged-in")
sprites.SSH(spriteID, "git clone https://github.com/user/doze /workspace/doze")
sprites.SSH(spriteID, "cd /workspace/doze && claude --print --input-format=stream-json --output-format=stream-json")
```

**Stop:**
```go
sprites.Destroy(spriteID)
```

## State Machine

```
StateNone (no session)
    ↓ /start
StateStarting (creating Sprite, spawning Claude)
    ↓
StateWaiting (idle, ready for input)
    ↓ user sends message
StateActive (Claude processing)
    ↓ response complete
StateWaiting (ready for next input)
    ↓ 30s idle timeout
StateShuttingDown (destroying Sprite)
    ↓
StateStopped (everything off)
```

## Security Considerations

### Current Approach (MVP)
- Single bearer token for authentication
- Claude runs with `--dangerously-skip-permissions` (required for remote)
- Sprite has access to env vars (but minimal secrets)
- Container isolation via Fly.io/Sprites
- Ephemeral - destroyed on every session end

### Risks Accepted (MVP)
- Claude could read environment variables
- Claude could exfiltrate code (but repo is open source)
- No rate limiting
- No input validation (beyond basic checks)

### Mitigations
- Minimal secrets in Sprite
- Separate API keys for Doze (not production keys)
- Bearer token rotation capability
- Ephemeral containers (no persistent attack surface)
- Single user (dogfooding phase)

### Production Hardening (Future)
- User authentication (OAuth, JWT)
- Rate limiting per user
- Input validation and sanitization
- Audit logging
- Read-only git tokens
- Multi-tenant isolation

## Technology Stack

### Backend
- **Language:** Go 1.21+
- **Dependencies:** None (stdlib only)
- **Architecture:** Single-file server (1600+ lines)
- **Concurrency:** Goroutines for I/O (stdout, stderr, wait)
- **Thread safety:** RWMutex for session state

### Frontend
- **Framework:** React 19 + TypeScript
- **Build tool:** Vite 7
- **Styling:** Tailwind CSS
- **State management:** Context API (SessionContext)
- **Real-time:** SSE (Server-Sent Events)

### Infrastructure
- **Hosting:** Fly.io (Go app + static UI)
- **Compute:** Sprites.dev (ephemeral workspaces)
- **DNS:** Fly.io default or custom domain
- **SSL:** Automatic via Fly.io

## Configuration

### Fly.io (fly.toml)
```toml
app = "doze"
primary_region = "sjc"

[http_service]
  internal_port = 2020
  force_https = true
  auto_stop_machines = true
  auto_start_machines = true
  min_machines_running = 0

[env]
  REPO_PATH = "/app"
```

### Environment Variables
- `PORT` - HTTP port (default: 2020)
- `DOZE_AUTH_TOKEN` - Bearer token for auth
- `SPRITES_API_KEY` - Sprites.dev API key
- `REPO_PATH` - Working directory (default: current dir)
- `IDLE_TIMEOUT` - Session timeout (default: 30s)

## Deployment

### Prerequisites
1. Fly.io account + CLI installed
2. Sprites.dev account + API key
3. Claude Max subscription (for checkpoint)
4. Go 1.21+ and Node.js (for building)

### Steps

```bash
# 1. Build web UI
cd web
npm install
npm run build

# 2. Deploy to Fly.io
cd ..
fly deploy

# 3. Set secrets
fly secrets set DOZE_AUTH_TOKEN="your-secret-token"
fly secrets set SPRITES_API_KEY="your-sprites-key"

# 4. Get URL
fly status  # https://doze.fly.dev
```

## Future Enhancements (V2+)

### Multi-Tenant Support
- User accounts and authentication
- Multiple Sprites per user
- Session management UI
- Usage tracking and billing

### GitHub Integration
- OAuth login
- Repository picker UI
- Automatic cloning
- Private repo support with PATs
- Commit/push capabilities

### Advanced Features
- Multiple concurrent sessions
- Session persistence across restarts
- Chat history storage (SQLite)
- Cost tracking per user
- Session limits (max 5 per user)
- Collaborative sessions
- Mobile PWA with offline support

### Performance Optimizations
- Session caching
- Faster Sprite warmup
- SSE compression
- Message batching

## Monitoring

### Health Checks
- `/health` endpoint returns 200 OK
- Fly.io auto-restarts on failure

### Metrics (Future)
- Session duration
- Message count
- Sprite creation/destruction frequency
- Error rates
- Cost per session

## Known Limitations (MVP)

1. **Single session only** - One user, one Sprite at a time
2. **No session persistence** - Server restart loses state
3. **No retry logic** - Failed messages need manual resend
4. **No rate limiting** - Open to spam (mitigated by auth)
5. **Manual Sprite checkpoint** - Requires initial setup
6. **No multi-repo support** - Hardcoded to doze repo
7. **No offline mode** - Requires internet connection

## Support

- **Issues:** https://github.com/yourusername/doze/issues
- **Documentation:** This file + code comments
- **API Docs:** See main.go godoc comments
