# ZeroSync — Self-Hosted

Deploy ZeroSync on your own infrastructure in minutes using pre-built Docker images.

No source code required. The signaling server and relay node run as containers pulled from GHCR.

## What's included

| Service | Image | Description |
|---------|-------|-------------|
| `zerosync` | `ghcr.io/tovsa7/zerosync-server` | Go signaling server |
| `relay` | `ghcr.io/tovsa7/zerosync-relay` | Relay node — encrypted data stays in your network |
| `caddy` | `caddy:2-alpine` | Reverse proxy with automatic TLS (Let's Encrypt) |

## Prerequisites

- Docker >= 24 and Docker Compose >= 2.20
- A domain with a DNS A record pointing to this server
- Ports 80 and 443 open
- A license key (free tier available)

## Quick start

```bash
# 1. Clone this repo
git clone https://github.com/tovsa7/zerosync-self-hosted.git
cd zerosync-self-hosted

# 2. Configure
cp .env.example .env
# Edit .env — set ZEROSYNC_DOMAIN and optionally ZEROSYNC_LICENSE_KEY

# 3. Start
docker compose up -d

# 4. Verify
curl https://your.domain.com/health
```

## Environment variables

| Variable | Required | Description |
|----------|----------|-------------|
| `ZEROSYNC_DOMAIN` | Yes | Your domain (e.g. `sync.example.com`) |
| `ZEROSYNC_LICENSE_KEY` | No | JWT license key — leave empty for Free tier |
| `ZEROSYNC_LICENSE_SECRET` | If key set | HMAC-SHA256 signing secret from your license |
| `RELAY_REGION` | No | Region tag shown to clients (default: `us-east`) |
| `RELAY_ROOM_ID` | No | Room the relay joins on startup (default: `default`) |
| `GOMAXPROCS` | No | OS threads for signaling server (default: `2`) |

## License tiers

| Tier | Rooms | Peers/room |
|------|-------|------------|
| Community | 5 (dev/test) | 10 |
| Starter | 50 | 10 |
| Business | Unlimited | Unlimited |
| Enterprise | Unlimited | Unlimited |

For more information visit [tovsa7.github.io/ZeroSync](https://tovsa7.github.io/ZeroSync/).

## Architecture

```
Client SDK (browser)
  │  WebSocket (WSS)
  ▼
Caddy (TLS termination)
  │
  ├── /ws       → zerosync (signaling server)
  ├── /health   → zerosync
  └── /relay/health → relay
```

All user data is end-to-end encrypted with AES-256-GCM in the browser. The signaling server sees only hashed room/peer IDs and ICE candidates — it cannot read your data.

The relay node forwards encrypted blobs between peers when a direct P2P connection is not possible. It does not hold the room key and cannot decrypt the data.

## Client SDK

The client SDK is MIT-licensed and available on npm:

```bash
npm install @tovsa7/zerosync-client
```

Source: [github.com/tovsa7/ZeroSync](https://github.com/tovsa7/ZeroSync)
