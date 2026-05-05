FROM golang:1.26-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o zerosync ./cmd/server

FROM alpine:3.23

# OCI image labels — link the GHCR package to the public Apache 2.0 source
# repository (instead of the private dev workspace it was previously inheriting
# from). Also set license + description so registry UIs render correctly.
LABEL org.opencontainers.image.source="https://github.com/tovsa7/zerosync-self-hosted" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.title="zerosync-server" \
      org.opencontainers.image.description="ZeroSync signaling server — end-to-end encrypted real-time collaboration over WebRTC + Yjs." \
      org.opencontainers.image.url="https://github.com/tovsa7/zerosync-self-hosted" \
      org.opencontainers.image.documentation="https://github.com/tovsa7/zerosync-self-hosted#readme"

# ca-certificates: required for TLS outbound connections.
# wget: used by docker-compose healthcheck (GET /health).
RUN apk add --no-cache ca-certificates wget
COPY --from=builder /build/zerosync /usr/local/bin/zerosync

EXPOSE 8080
ENTRYPOINT ["zerosync"]
CMD ["-addr", ":8080"]
