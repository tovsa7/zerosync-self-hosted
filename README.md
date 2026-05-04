# ZeroSync — Self-Hosted

[![CI/CD](https://github.com/tovsa7/zerosync-self-hosted/actions/workflows/ci.yml/badge.svg)](https://github.com/tovsa7/zerosync-self-hosted/actions/workflows/ci.yml)

End-to-end encrypted real-time collaboration server. Apache 2.0 licensed.

ZeroSync clients negotiate WebRTC connections through this signaling server.
When direct P2P is not possible, the server also broadcasts encrypted blobs
between connected peers in the same room. The server never sees the room
key — payloads are encrypted in the browser with AES-256-GCM, and the server
only sees hashed peer/room IDs and ciphertext.

## What's included

| Service | Image | Description |
|---------|-------|-------------|
| `zerosync` | `ghcr.io/tovsa7/zerosync-server` | Go signaling server (also relays encrypted blobs in-memory) |
| `caddy` | `caddy:2-alpine` | Reverse proxy with automatic TLS (Let's Encrypt) |

## Prerequisites

- Docker >= 24 and Docker Compose >= 2.20
- A domain with a DNS A record pointing to this server
- Ports 80 and 443 open

## Quick start (pre-built images)

```bash
git clone https://github.com/tovsa7/zerosync-self-hosted.git
cd zerosync-self-hosted

cp .env.example .env
# Edit .env — set ZEROSYNC_DOMAIN

docker compose up -d
curl https://your.domain.com/health
```

## Build from source

```bash
git clone https://github.com/tovsa7/zerosync-self-hosted.git
cd zerosync-self-hosted

go build -o zerosync ./cmd/server
./zerosync -addr :8080
```

Or via Docker:

```bash
docker build -t zerosync-server:local .
docker run --rm -p 8080:8080 zerosync-server:local
```

## Environment variables

| Variable | Required | Description |
|----------|----------|-------------|
| `ZEROSYNC_DOMAIN` | Yes | Your domain (e.g. `sync.example.com`) |
| `GOMAXPROCS` | No | OS threads for signaling server (default: `2`) |

## Architecture

```
Client SDK (browser)
  │  WebSocket (WSS)
  ▼
Caddy (TLS termination)
  │
  ├── /ws     → zerosync (signaling server)
  └── /health → zerosync
```

All user data is end-to-end encrypted with AES-256-GCM in the browser. The
signaling server sees only hashed room/peer IDs and ciphertext — it cannot
read your data. When two peers cannot establish a direct WebRTC connection,
the server forwards their encrypted blobs in-memory between currently
connected peers in the same room. It does not hold the room key and cannot
decrypt the data.

## Repository layout

```
auth/              Validator interface and NoopValidator default
signaling/         WebSocket handler, peer/room signaling protocol
room/              Room registry and per-room peer state
relay/             Relay blob store
cmd/server/        Server entry point
Caddyfile          Reverse proxy config
docker-compose.yml Compose stack (pre-built images)
Dockerfile         Build server from source
```

## Client SDK

The MIT-licensed client SDK is on npm:

```bash
npm install @tovsa7/zerosync-client
```

Source: [github.com/tovsa7/ZeroSync](https://github.com/tovsa7/ZeroSync)

## License

Apache License 2.0 — see [LICENSE](LICENSE).

Copyright 2024-2026 tovsa7.
