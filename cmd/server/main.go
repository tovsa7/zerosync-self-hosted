package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tovsa7/zerosync-self-hosted/auth"
	"github.com/tovsa7/zerosync-self-hosted/room"
	"github.com/tovsa7/zerosync-self-hosted/signaling"
)

const version = "0.1.0"

func main() {
	addr          := flag.String("addr", ":8080", "listen address")
	maxConnsPerIP := flag.Int("max-conns-per-ip", 10, "max concurrent WebSocket connections per IP (0 = unlimited)")
	flag.Parse()

	rooms     := room.NewRegistry()
	nonces    := signaling.NewNonceStore()
	validator := auth.NoopValidator{}
	handler   := signaling.NewHandler(rooms, nonces, validator)
	limiter := signaling.NewConnLimiter(*maxConnsPerIP)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", cors(handleHealth(rooms)))
	mux.HandleFunc("GET /ws", handleWS(handler, limiter))

	srv := &http.Server{
		Addr:         *addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("zerosync server starting", "addr", *addr, "version", version)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	<-quit
	slog.Info("shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("shutdown error", "err", err)
	}

	rooms.Stop()
	nonces.Stop()
}

func handleWS(h *signaling.Handler, limiter *signaling.ConnLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := signaling.RemoteIP(r)
		if !limiter.Acquire(ip) {
			http.Error(w, "too many connections from your IP", http.StatusTooManyRequests)
			slog.Warn("connection limit reached", "ip", ip)
			return
		}
		conn, err := signaling.Upgrade(w, r)
		if err != nil {
			limiter.Release(ip)
			slog.Warn("ws upgrade failed", "err", err)
			return
		}
		go func() {
			defer limiter.Release(ip)
			h.Serve(conn)
		}()
	}
}

func handleHealth(rooms *room.Registry) http.HandlerFunc {
	start := time.Now()
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status":  "ok",
			"version": version,
			"rooms":   rooms.Len(),
			"uptime":  fmt.Sprintf("%s", time.Since(start).Round(time.Second)),
		})
	}
}

func cors(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h(w, r)
	}
}

