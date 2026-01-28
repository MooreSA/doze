# Doze API Server

Go server that orchestrates Claude Code sessions in Fly.io Sprites.

## Responsibilities

- Manage Sprite lifecycle (create, wake, checkpoint, hibernate)
- Stream Claude Code stdout to clients via SSE
- Inject user messages to Claude stdin
- Detect idle state, trigger auto-hibernate
- Resume sessions seamlessly

## API Endpoints

### POST /start
Start a new Claude Code session.

**Response:**
```json
{
  "session_id": "abc123",
  "state": "starting"
}
```

### GET /stream
Server-Sent Events stream of Claude Code output.

**Events:**
```
data: {"type": "output", "content": "Running tests...\n"}
data: {"type": "state", "state": "active"}
```

### POST /message
Send a message to Claude.

**Request:**
```json
{
  "content": "Fix the bug in auth.go"
}
```

**Response:**
```json
{
  "queued": true,
  "state": "active"
}
```

### GET /status
Get current session state.

**Response:**
```json
{
  "state": "active",
  "claude_session_id": "abc123",
  "idle_seconds": 45,
  "last_output": "Ready for your next message..."
}
```

## State Machine

```
STARTING → ACTIVE → WAITING → HIBERNATING → HIBERNATED
                       ↓                        ↓
                     ACTIVE  ←─────────────  WAKING
```

## Development

```bash
cd api
go mod init github.com/yourusername/doze/api
go mod tidy
go run main.go
```

## Environment Variables

```bash
FLY_API_TOKEN=xxx           # Fly.io API token
SPRITES_ORG_SLUG=personal   # Your Sprites org
REPO_PATH=/workspace/repo   # Path in Sprite
BASE_CHECKPOINT=base-authed # Sprite checkpoint name
```

## Build

```bash
go build -o doze
```

## Deploy

```bash
fly deploy
```
