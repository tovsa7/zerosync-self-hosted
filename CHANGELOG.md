# Changelog

All notable changes to the `zerosync-self-hosted` signaling server are
documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [0.2.0] — 2026-05-05

### Changed
- **`GET /health` is now cap-aware.** When the calling IP is at the per-IP
  connection cap, the endpoint returns HTTP **429 Too Many Requests** with
  `{"status":"at-capacity", ...}` instead of always returning 200.

### Added
- `signaling.ConnLimiter.Available(ip)` — peeks per-IP cap status without
  consuming a slot. Used by `/health` so the response reports prospective
  WebSocket admissibility without changing it.

### Why
Browser WebSocket close events drop HTTP status, so a 429 capacity rejection
on `/ws` upgrade is indistinguishable from a network failure on the client.
Clients running `@tovsa7/zerosync-client ≥ 0.3.0` now probe `/health` after a
failed handshake to learn the cause and surface a precise UX message
(`"Server at capacity"` vs `"Server unavailable"`). This change closes the
information gap on the server side. Older clients are unaffected (they don't
probe `/health`).

### Internal / tests
- 5 new tests for `Available()` on `ConnLimiter`.

---

## [0.1.0] — 2026-04-30

### Added
- Initial Apache 2.0 split: signaling server extracted from the (now-private)
  enterprise repo.
- WebSocket signaling protocol with HELLO / PEER_LIST / PEER_JOINED /
  PEER_LEFT / RELAY / ICE_OFFER / ICE_ANSWER / ICE_CANDIDATE / PING / PONG.
- Pluggable `auth.Validator` interface (NoopValidator default; license
  validator lives in the private `zerosync-enterprise` repo).
- Per-IP and per-room connection caps.
- Nonce store for replay protection.
- `GET /health` for liveness probes.

[0.2.0]: https://github.com/tovsa7/zerosync-self-hosted/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/tovsa7/zerosync-self-hosted/releases/tag/v0.1.0
