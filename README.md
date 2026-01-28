# Doze

**Chat with Claude Code from your phone without burning money when idle.**

Doze is a self-hosted mobile interface for Claude Code that runs in sandboxed Fly.io Sprites. Sessions automatically hibernate after a few minutes of inactivity and resume seamlessly when you send your next message.

## Why?

- Anthropic's Claude Code iOS app is buggy (tasks get stuck)
- Third-party tools route your code through their infrastructure
- You want full control over security, secrets, and workflow
- You're already paying for Claude Max, why pay more for idle compute?

## How It Works

```
┌─────────────────┐
│  Your Phone     │  Chat from anywhere
│  (Browser)      │
└────────┬────────┘
         │ HTTPS
         ▼
┌─────────────────┐
│  API Server     │  Orchestrates everything
│  (Fly.io)       │
└────────┬────────┘
         │ Sprites SDK
         ▼
┌─────────────────┐
│  Sprite         │  Claude Code runs here
│  (Fly.io)       │  - Hibernates when idle ($0)
│                 │  - Resumes in ~1 second
└─────────────────┘
```

## Status

**MVP in progress.** See [TODO.md](TODO.md) for current build status.

## Docs

- [TODO.md](TODO.md) - Current build plan and progress
- [docs/mvp-design.md](docs/mvp-design.md) - Detailed MVP design (focus on hibernate/resume)
- [docs/design.md](docs/design.md) - Full vision (V2+ features)

## Tech Stack

- **API:** Go + Fly.io Sprites SDK
- **Frontend:** Vanilla HTML/JS (PWA later)
- **Compute:** Fly.io Sprites (sandboxed Linux VMs)
- **Claude:** Claude Code CLI with Max subscription

## Cost Estimate

- Sprite active: ~$0.12/hour
- Sprite hibernated: $0.00/hour
- With 3-min idle timeout: ~$0.06/hour average
- API server (Fly.io): ~$2/month
- **Realistic monthly cost:** $5-10 for light usage

*Not included: Claude Max subscription ($100-200/month) - you already pay this*

## Quick Start

**Prerequisites:**
- Docker & Docker Compose
- Fly.io account with Sprites access
- Claude Max subscription

**Local Development:**

```bash
# Clone the repo
git clone https://github.com/yourusername/doze.git
cd doze

# Copy env template and fill in your values
cp .env.example .env
# Edit .env with your FLY_API_TOKEN

# Build and start services
make build
make up

# View logs
make logs

# Open browser to http://localhost:3030
```

**Available Commands:**
```bash
make help          # Show all commands
make build         # Build Docker images
make up            # Start services
make down          # Stop services
make logs          # View logs
make restart       # Restart services
make clean         # Clean up everything
```

## Development

See [TODO.md](TODO.md) for current build status.

### Structure

```
doze/
├── api/           # Go server + Sprites SDK
├── web/           # HTML/JS frontend
├── docs/          # Design documents
└── TODO.md        # Build plan and progress
```

## License

MIT (probably - decide later)

## Name

"Doze" captures the magic: your Claude sessions sleep when idle, wake instantly when needed.
