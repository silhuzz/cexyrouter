package ws

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"

	"github.com/silhuzz/cexyrouter/internal/api"
)

func Mount(r chi.Router, deps api.Deps) {
	if deps.DB == nil {
		h := handler{allowedOrigins: deps.Config.CORSAllowedOrigins}
		r.Get("/v1/ws", h.serve)
		return
	}
	store := NewSQLStore(deps.DB)
	dispatcher := NewDispatcher(store)
	ctx := deps.Context
	if ctx == nil {
		ctx = context.Background()
	}
	startListener(ctx, NewPGListener(deps.DB), dispatcher)

	h := handler{
		allowedOrigins: deps.Config.CORSAllowedOrigins,
		store:          store,
		dispatcher:     dispatcher,
	}
	r.Get("/v1/ws", h.serve)
}

type handler struct {
	allowedOrigins []string
	store          BackfillStore
	dispatcher     *Dispatcher
}

func (h handler) serve(w http.ResponseWriter, r *http.Request) {
	if !h.originAllowed(r) {
		http.Error(w, "websocket origin is not allowed", http.StatusForbidden)
		return
	}
	if !isWebSocketUpgrade(r) {
		http.Error(w, "websocket upgrade required", http.StatusUpgradeRequired)
		return
	}
	if h.store == nil || h.dispatcher == nil {
		http.Error(w, "websocket runtime is not configured", http.StatusServiceUnavailable)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	readCtx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	_, payload, err := conn.Read(readCtx)
	if err != nil {
		cancel()
		_ = conn.Close(websocket.StatusPolicyViolation, "subscription required")
		return
	}
	cancel()

	sub, err := ParseSubscription(payload)
	if err != nil {
		_ = conn.Close(websocket.StatusPolicyViolation, err.Error())
		return
	}

	filters, err := NormalizeFilters(sub.Filters)
	if err != nil {
		_ = conn.Close(websocket.StatusPolicyViolation, err.Error())
		return
	}

	write := func(payload any) error {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		return wsjson.Write(ctx, conn, payload)
	}

	if sub.Since != nil {
		if time.Since(sub.Since.OccurredAt) > maxBackfillWindow {
			_ = conn.Close(websocket.StatusPolicyViolation, "backfill window too old")
			return
		}
		_, err := BackfillSince(r.Context(), h.store, *sub.Since, filters, func(event Event) error {
			return write(envelope{Type: "event", Event: event})
		})
		if err != nil {
			_ = conn.Close(websocket.StatusInternalError, "backfill failed")
			return
		}
	}

	out, unsubscribe, err := h.dispatcher.Subscribe(filters)
	if err != nil {
		_ = conn.Close(websocket.StatusPolicyViolation, err.Error())
		return
	}
	defer unsubscribe()

	closed := make(chan struct{})
	go func() {
		defer close(closed)
		for {
			if _, _, err := conn.Read(r.Context()); err != nil {
				return
			}
		}
	}()

	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-closed:
			return
		case msg, ok := <-out:
			if !ok {
				return
			}
			if err := write(msg); err != nil {
				return
			}
		case <-heartbeat.C:
			if err := write(map[string]string{"type": "heartbeat"}); err != nil {
				return
			}
		}
	}
}

func startListener(ctx context.Context, listener Listener, dispatcher *Dispatcher) {
	if listener == nil || dispatcher == nil {
		return
	}
	notifications := make(chan Notification, 256)
	go func() {
		for notification := range notifications {
			if err := dispatcher.DispatchNotification(ctx, notification); err != nil && ctx.Err() == nil {
				slog.Warn("dispatch websocket notification", "error", err)
			}
		}
	}()
	go func() {
		for {
			if err := listener.Listen(ctx, notifications); err != nil && ctx.Err() == nil {
				slog.Warn("websocket listener stopped", "error", err)
				select {
				case <-ctx.Done():
					close(notifications)
					return
				case <-time.After(2 * time.Second):
				}
				continue
			}
			close(notifications)
			return
		}
	}()
}

func (h handler) originAllowed(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	allowed := h.allowedOrigins
	if len(allowed) == 0 {
		allowed = []string{"*"}
	}
	for _, candidate := range allowed {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || strings.EqualFold(candidate, origin) {
			return true
		}
	}
	return false
}

func isWebSocketUpgrade(r *http.Request) bool {
	return headerContains(r.Header, "Connection", "upgrade") &&
		strings.EqualFold(strings.TrimSpace(r.Header.Get("Upgrade")), "websocket")
}

func headerContains(header http.Header, key string, expected string) bool {
	expected = strings.ToLower(expected)
	for _, value := range header.Values(key) {
		for _, part := range strings.Split(value, ",") {
			if strings.ToLower(strings.TrimSpace(part)) == expected {
				return true
			}
		}
	}
	return false
}
