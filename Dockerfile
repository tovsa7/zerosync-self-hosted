FROM golang:1.26-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o zerosync ./cmd/server

FROM alpine:3.20

# ca-certificates: required for TLS outbound connections.
# wget: used by docker-compose healthcheck (GET /health).
RUN apk add --no-cache ca-certificates wget
COPY --from=builder /build/zerosync /usr/local/bin/zerosync

EXPOSE 8080
ENTRYPOINT ["zerosync"]
CMD ["-addr", ":8080"]
