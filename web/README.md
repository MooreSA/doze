# Doze Web UI

Minimal mobile-friendly chat interface for Doze.

## Design Goals

- Works on any mobile browser (iOS Safari, Android Chrome)
- No build step (vanilla HTML/JS)
- Real-time streaming via Server-Sent Events
- Auto-reconnect on disconnect
- Visual state indicators (active/waiting/hibernated)

## Files

- `index.html` - Single-page app (coming soon)
- `styles.css` - Dark theme, mobile-first (optional, may inline)
- `app.js` - SSE client, message sending (optional, may inline)

## Development

No build step needed. Just open in browser or serve via API server.

## Future

- Convert to PWA (manifest.json, service worker)
- Offline message queue
- Voice input (Web Speech API)
- Native wrapper (Capacitor)
