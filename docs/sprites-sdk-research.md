# Sprites.dev Go SDK Research

**Last Updated:** 2026-01-29
**Purpose:** Documentation of sprites-go library capabilities for Doze integration

## Overview

The official Go SDK for Sprites.dev is **github.com/superfly/sprites-go**. It provides an idiomatic Go API that mirrors the standard library's `exec.Cmd` interface, making it a drop-in replacement for local process execution but running commands on remote Sprites instead.

**Key Insight:** The SDK design means our existing process management code can be adapted with minimal changes—the I/O patterns, pipe handling, and goroutine structure remain the same.

## Installation

```bash
go get github.com/superfly/sprites-go
```

**Import:**
```go
import "github.com/superfly/sprites-go"
// Package name is "sprites"
```

## Authentication

Requires a Sprites API token (from Fly.io):

```bash
export SPRITES_API_KEY="your-token-here"
```

Or create a token programmatically:
```go
token, err := sprites.CreateToken(ctx, flyMacaroon, orgSlug, inviteCode)
```

## Core API

### Client Initialization

```go
client := sprites.New(os.Getenv("SPRITES_API_KEY"))

// With custom options
client := sprites.New(token,
    sprites.WithBaseURL("https://api.sprites.dev"),
    sprites.WithHTTPClient(customHTTPClient),
)
```

### Sprite Lifecycle Management

```go
// Create a new sprite
sprite, err := client.Create("my-sprite-name")
if err != nil {
    log.Fatal(err)
}

// Get handle to existing sprite (doesn't create on server)
sprite := client.Sprite("my-sprite-name")

// List all sprites
sprites, err := client.List()

// Destroy sprite
err := sprite.Destroy()
```

### Command Execution (The Important Part!)

**Basic Commands:**
```go
sprite := client.Sprite("my-sprite")

// Run command and wait
cmd := sprite.Command("echo", "hello", "world")
err := cmd.Run()

// Get output
output, err := cmd.Output()

// Get combined stdout/stderr
combined, err := cmd.CombinedOutput()
```

**Streaming I/O (Perfect for Claude Code):**
```go
cmd := sprite.Command("claude",
    "--print",
    "--input-format=stream-json",
    "--output-format=stream-json",
)

// Get pipes (exactly like exec.Cmd!)
stdin, err := cmd.StdinPipe()
stdout, err := cmd.StdoutPipe()
stderr, err := cmd.StderrPipe()

// Start the command
if err := cmd.Start(); err != nil {
    log.Fatal(err)
}

// Use goroutines to handle I/O
go func() {
    scanner := bufio.NewScanner(stdout)
    for scanner.Scan() {
        handleOutput(scanner.Text())
    }
}()

go func() {
    io.Copy(os.Stderr, stderr)
}()

// Write to stdin
fmt.Fprintln(stdin, `{"type":"user","message":"hello"}`)

// Wait for completion
err = cmd.Wait()
```

**Context Support:**
```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

cmd := sprite.CommandContext(ctx, "long-running-command")
err := cmd.Run()
// Command terminates if context times out
```

**Environment Variables & Working Directory:**
```go
cmd := sprite.Command("env")
cmd.Env = []string{"FOO=bar", "BAZ=qux"}
cmd.Dir = "/workspace/doze"
output, err := cmd.Output()
```

### TTY Support (Interactive Shells)

```go
cmd := sprite.Command("bash")
cmd.SetTTY(true)
cmd.SetTTYSize(24, 80)  // rows, cols

err := cmd.Start()

// Resize during execution
cmd.Resize(30, 100)

cmd.Wait()
```

### Port Forwarding

**Single Port:**
```go
session, err := sprite.ProxyPort(ctx, 3000, 3000)
defer session.Close()
// Now local :3000 forwards to sprite :3000
```

**Multiple Ports:**
```go
sessions, err := sprite.ProxyPorts(ctx, []sprites.PortMapping{
    {LocalPort: 3000, RemotePort: 3000},
    {LocalPort: 8080, RemotePort: 80},
})
defer func() {
    for _, s := range sessions {
        s.Close()
    }
}()
```

**Port Notifications (Auto-Discovery):**
```go
cmd.TextMessageHandler = func(data []byte) {
    var notification sprites.PortNotificationMessage
    if err := json.Unmarshal(data, &notification); err != nil {
        return
    }

    if notification.Type == "port_opened" {
        log.Printf("Port %d opened on %s", notification.Port, notification.Address)

        // Automatically set up forwarding
        session, _ := sprite.ProxyPorts(ctx, []sprites.PortMapping{
            {
                LocalPort: notification.Port,
                RemotePort: notification.Port,
                RemoteHost: notification.Address,
            },
        })
    }
}
```

## Checkpoint Operations

Checkpoints allow you to save and restore the entire state of a Sprite (filesystem + memory).

**List Checkpoints:**
```go
checkpoints, err := sprite.ListCheckpoints(ctx, "")
if err != nil {
    log.Fatal(err)
}

for _, cp := range checkpoints {
    fmt.Printf("Checkpoint: %s (created: %s)\n", cp.ID, cp.CreatedAt)
}
```

**Create Checkpoint:**
```go
// Create checkpoint with specific ID/name
checkpoint, err := sprite.CreateCheckpoint(ctx, "claude-logged-in")
if err != nil {
    log.Fatal(err)
}
// Takes ~300ms, captures entire disk state
```

**Restore from Checkpoint:**
```go
// Restore to previous state
err := sprite.RestoreCheckpoint(ctx, "claude-logged-in")
if err != nil {
    log.Fatal(err)
}
// Returns immediately, restore happens async and restarts environment
```

**Access Checkpoint Files:**
The last 5 checkpoints are mounted at `/.sprite/checkpoints` inside the Sprite for direct file access.

## Error Handling

**Exit Codes:**
```go
cmd := sprite.Command("false")
err := cmd.Run()

if exitErr, ok := err.(*sprites.ExitError); ok {
    fmt.Printf("Exit code: %d\n", exitErr.ExitCode())
    fmt.Printf("Stderr: %s\n", exitErr.Stderr)
}
```

**Connection Errors:**
Handle network issues, authentication failures, and sprite not found errors as appropriate.

## How This Applies to Doze

### Current Architecture (Local Process)

```
api/main.go:
  - exec.Command("claude", args...)
  - StdinPipe, StdoutPipe, StderrPipe
  - Goroutines for I/O
  - SSE broadcast to clients
```

### Target Architecture (Sprites)

```
api/main.go:
  - client.Sprite(name).Command("claude", args...)
  - StdinPipe, StdoutPipe, StderrPipe  ← Same!
  - Goroutines for I/O                  ← Same!
  - SSE broadcast to clients            ← Same!
```

### Migration Strategy

The beauty of sprites-go is that it implements the same interfaces as `exec.Cmd`, so:

1. **Replace process creation:**
   ```go
   // OLD:
   cmd := exec.Command("claude", args...)

   // NEW:
   sprite := s.spriteClient.Sprite(s.spriteName)
   cmd := sprite.Command("claude", args...)
   ```

2. **Keep all I/O handling code:** The pipe interfaces, scanners, goroutines, and SSE broadcasting can remain unchanged.

3. **Add sprite lifecycle management:**
   - Create sprite on `/start`
   - Destroy sprite on idle timeout
   - Optionally checkpoint before destroy for faster resume

4. **Update session struct:**
   ```go
   type Session struct {
       spriteClient *sprites.Client  // NEW
       spriteName   string           // NEW
       cmd          *sprites.Cmd     // Changed from *exec.Cmd
       stdin        io.WriteCloser
       // ... rest stays same
   }
   ```

### Code Sections to Update

**In `api/main.go`:**

1. **Lines ~100-150:** Add sprites client initialization
   ```go
   spriteClient := sprites.New(os.Getenv("SPRITES_API_KEY"))
   ```

2. **Lines ~400-450:** Replace `exec.Command` with `sprite.Command`
   ```go
   sprite := session.spriteClient.Sprite(session.spriteName)
   cmd := sprite.Command("claude", args...)
   ```

3. **Lines ~200-250:** Add sprite creation to `handleStart()`
   ```go
   spriteName := fmt.Sprintf("doze-%s", randomID())
   sprite, err := spriteClient.Create(spriteName)
   if err != nil {
       return err
   }

   // Clone repo
   cloneCmd := sprite.Command("git", "clone", repoURL, "/workspace/doze")
   if err := cloneCmd.Run(); err != nil {
       sprite.Destroy()
       return err
   }
   ```

4. **Lines ~600-650:** Add sprite destruction to shutdown logic
   ```go
   func (s *Session) shutdown(ctx context.Context) error {
       // ... existing SIGTERM logic ...

       // Destroy sprite
       sprite := s.spriteClient.Sprite(s.spriteName)
       if err := sprite.Destroy(); err != nil {
           log.Printf("Failed to destroy sprite: %v", err)
       }

       return nil
   }
   ```

5. **Resume logic:** Optionally add checkpoint support
   ```go
   // Before destroy
   checkpointID := fmt.Sprintf("session-%s", sessionID)
   sprite.CreateCheckpoint(ctx, checkpointID)
   sprite.Destroy()

   // On resume
   newSprite, _ := client.Create(newSpriteName)
   newSprite.RestoreCheckpoint(ctx, checkpointID)
   ```

### What Stays the Same

- SSE streaming logic
- Message parsing (stream-json)
- Ring buffer
- State machine
- Frontend (no changes!)
- All I/O goroutines
- Error handling patterns

### Environment Variables to Add

```bash
SPRITES_API_KEY=your-sprites-token
SPRITES_ORG_SLUG=your-org  # Optional
```

## Checkpoint Strategy for Doze

From ARCHITECTURE.md considerations:

**Problem:** Claude's `--resume` can fail if checkpointed mid-tool-execution.

**Solution:** Only checkpoint when Claude is idle (StateWaiting).

```go
func (s *Session) checkpointIfSafe() error {
    s.mu.RLock()
    state := s.state
    s.mu.RUnlock()

    if state != StateWaiting {
        return errors.New("not safe to checkpoint during tool execution")
    }

    sprite := s.spriteClient.Sprite(s.spriteName)
    checkpointID := fmt.Sprintf("session-%s", s.sessionID)
    _, err := sprite.CreateCheckpoint(context.Background(), checkpointID)
    return err
}
```

**Checkpoint Timing:**
- After Claude responds and returns to waiting state
- Before destroying on idle timeout
- Only if idle for >30s (safe to assume no active tools)

## Performance Notes

- **Sprite creation:** ~2-5 seconds cold start
- **Checkpoint creation:** ~300ms
- **Checkpoint restore:** ~2-3 seconds
- **Command execution latency:** Similar to SSH (~50-100ms)

## Resources

- **GitHub:** https://github.com/superfly/sprites-go
- **API Docs:** https://sprites.dev/api
- **Checkpoints API:** https://sprites.dev/api/sprites/checkpoints
- **Working with Sprites:** https://docs.sprites.dev/working-with-sprites/
- **Blog Post:** https://fly.io/blog/code-and-let-live/
- **Simon Willison Analysis:** https://simonwillison.net/2026/Jan/9/sprites-dev/

## Next Steps for Integration

1. Add `github.com/superfly/sprites-go` to go.mod
2. Update Session struct to include sprite client
3. Replace exec.Command with sprite.Command
4. Add sprite creation to /start endpoint
5. Add sprite destruction to shutdown logic
6. Test with local Claude first, then deploy
7. Optionally add checkpoint support for faster resume

## Notes & Caveats

- SDK is very new (published Jan 27, 2026)
- Some methods marked as deprecated (Create, List on Client)
- Prefer using `client.Sprite(name)` handle pattern
- No license detected on pkg.go.dev (as of Jan 2026)
- Documentation best found in GitHub README and source code