# Sprites Integration Testing Guide

## Server Status

The server should already be running. If not, start it with:

```bash
cd /Users/seamus/work/doze
./run-with-sprites.sh
```

You should see:
```
time=... level=INFO msg="sprites client initialized"
time=... level=INFO msg="server listening" port=2020
```

## Test 1: Health Check

```bash
curl http://localhost:2020/health | jq .
```

**Expected:**
```json
{
  "status": "ok"
}
```

## Test 2: Check Initial Status

```bash
curl http://localhost:2020/status | jq .
```

**Expected:**
```json
{
  "state": "none",
  "claude_session_id": "",
  "repo_path": "",
  "last_activity": "0001-01-01T00:00:00Z",
  "idle_seconds": 0,
  "recent_output": ""
}
```

## Test 3: Start a Session (Creates Sprite!)

This will:
1. Create a new sprite
2. Restore from checkpoint "v1" 
3. Clone your repo
4. Start Claude Code

```bash
curl -X POST http://localhost:2020/start \
  -H "Content-Type: application/json" \
  -d '{}' | jq .
```

**Expected:**
```json
{
  "success": true,
  "state": "waiting",
  "repo_path": "/workspace/doze"
}
```

**Watch the server logs** - you should see:
```
level=INFO msg="creating sprite" name=doze-xxxxxxxx
level=INFO msg="sprite created" name=doze-xxxxxxxx
level=INFO msg="restoring sprite from checkpoint" checkpoint=v1
level=INFO msg="sprite restored from checkpoint" name=doze-xxxxxxxx checkpoint=v1
level=INFO msg="claude process started on sprite" sprite=doze-xxxxxxxx
```

## Test 4: Send a Message to Claude

```bash
curl -X POST http://localhost:2020/message \
  -H "Content-Type: application/json" \
  -d '{"content":"Say hello!"}' | jq .
```

**Expected:**
```json
{
  "success": true,
  "queued": true,
  "state": "active"
}
```

## Test 5: Stream Output (SSE)

Open a new terminal and run:

```bash
curl -N http://localhost:2020/stream
```

You should see real-time events:
```
event: output
data: {"type":"output","content":"Hello! ..."}

event: state
data: {"type":"state","state":"waiting"}
```

Press Ctrl+C to stop streaming.

## Test 6: Check Status After Activity

```bash
curl http://localhost:2020/status | jq .
```

**Expected:**
```json
{
  "state": "waiting",
  "claude_session_id": "claude_session_...",
  "repo_path": "/workspace/doze",
  "last_activity": "2026-01-29T...",
  "idle_seconds": 5,
  "recent_output": "..."
}
```

## Test 7: Wait for Idle Timeout (30 seconds)

Wait 30 seconds without sending any messages. The server should:
1. Detect idle session
2. Shut down Claude
3. Destroy the sprite

**Watch the logs:**
```
level=INFO msg="idle timeout reached" timeout=30s
level=INFO msg="stopping sprite-based session"
level=INFO msg="destroying sprite" name=doze-xxxxxxxx
level=INFO msg="sprite destroyed" name=doze-xxxxxxxx
```

## Test 8: Check Status After Timeout

```bash
curl http://localhost:2020/status | jq .
```

**Expected:**
```json
{
  "state": "stopped",
  "claude_session_id": "claude_session_...",
  ...
}
```

## Test 9: Resume Session

Send a new message - this should create a NEW sprite and resume the session:

```bash
curl -X POST http://localhost:2020/message \
  -H "Content-Type: application/json" \
  -d '{"content":"What did we talk about?"}' | jq .
```

**Expected:**
```json
{
  "success": true,
  "queued": true,
  "resumed": true,
  "state": "active"
}
```

**Watch the logs** - you should see a new sprite created and Claude resumed with `--resume` flag.

## Cleanup

To stop the server:

```bash
# Find the process
ps aux | grep doze-api

# Kill it
pkill doze-api
```

Or just press Ctrl+C in the terminal where it's running.

## Troubleshooting

### Error: "failed to create sprite"

Check:
- Is your SPRITES_API_KEY valid?
- Do you have sprites access enabled?
- Check server logs for detailed error

### Error: "failed to restore from checkpoint"

Check:
- Does checkpoint "v1" exist?
- List checkpoints: `sprites checkpoints list`

### Error: "failed to clone repo"

Check:
- Is REPO_URL correct in .env?
- Is the repo public or accessible?

## Next Steps

If all tests pass:
1. ✅ Sprites integration is working!
2. ✅ Checkpoint restore is working!
3. ✅ Resume flow is working!
4. 🚀 Ready to deploy to Fly.io

## Monitoring Costs

Check your sprites usage:
```bash
sprites list
```

Each test creates/destroys sprites. With 30s idle timeout, sprites should scale to $0 quickly.
