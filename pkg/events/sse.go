package events

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// SSEHandler serves Server-Sent Events for a single workspace stream.
// Auth: if tokens is non-empty, accepts either ?token=... (preferred for EventSource)
// or Authorization: Bearer ... (fallback for non-browser clients).
// Query:
//
//	workspaceId: required
//	since: optional last seen event id (also respects Last-Event-ID header)
//
// Behavior:
//   - Replays buffered events with id > since (ring buffer) then streams live
//   - Sends heartbeat comments every 25s
func SSEHandler(hub *Hub, tokens []string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hub == nil {
			http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
			return
		}

		// Auth (query token or Bearer header)
		if len(tokens) > 0 {
			if !isAuthorized(r, tokens) {
				w.Header().Set("WWW-Authenticate", `Bearer realm="events", error="invalid_token"`)
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
				return
			}
		}

		wsID := r.URL.Query().Get("workspaceId")
		if strings.TrimSpace(wsID) == "" {
			http.Error(w, "workspaceId is required", http.StatusBadRequest)
			return
		}

		// Determine since id from query or Last-Event-ID
		var since int64
		if s := r.URL.Query().Get("since"); s != "" {
			if v, err := strconv.ParseInt(s, 10, 64); err == nil {
				since = v
			}
		}
		if s := r.Header.Get("Last-Event-ID"); s != "" {
			if v, err := strconv.ParseInt(s, 10, 64); err == nil && v > since {
				since = v
			}
		}

		// Prepare streaming response
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		// Disable proxies buffering (nginx)
		w.Header().Set("X-Accel-Buffering", "no")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		// Subscribe (includes replay)
		eventsCh, unsubscribe := hub.Subscribe(wsID, since, 128)
		defer unsubscribe()

		// Heartbeats
		heartbeat := time.NewTicker(25 * time.Second)
		defer heartbeat.Stop()

		notify := r.Context().Done()

		// Initial flush to start stream
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		for {
			select {
			case evt, ok := <-eventsCh:
				if !ok {
					return
				}
				// Serialize and emit SSE frame
				data, err := json.Marshal(evt)
				if err != nil {
					slog.Warn("failed to marshal event", "error", err)
					continue
				}
				// id + named event for filtering on client
				if _, err = w.Write([]byte("id: " + strconv.FormatInt(evt.ID, 10) + "\n")); err != nil {
					return
				}
				if _, err = w.Write([]byte("event: workspace.event\n")); err != nil {
					return
				}
				if _, err = w.Write([]byte("data: ")); err != nil {
					return
				}
				if _, err = w.Write(data); err != nil {
					return
				}
				if _, err = w.Write([]byte("\n\n")); err != nil {
					return
				}
				flusher.Flush()

			case <-heartbeat.C:
				// Comment line as heartbeat
				if _, err := w.Write([]byte(": ping\n\n")); err != nil {
					return
				}
				flusher.Flush()

			case <-notify:
				return
			}
		}
	})
}

func isAuthorized(r *http.Request, tokens []string) bool {
	// Prefer query token for EventSource
	q := strings.TrimSpace(r.URL.Query().Get("token"))
	if q != "" && tokenAllowed(q, tokens) {
		return true
	}
	// Fallback: Bearer header
	authz := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(authz), "bearer ") {
		bearer := strings.TrimSpace(authz[len("Bearer "):])
		return tokenAllowed(bearer, tokens)
	}
	parts := strings.SplitN(authz, " ", 2)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return tokenAllowed(strings.TrimSpace(parts[1]), tokens)
	}
	return false
}

func tokenAllowed(got string, tokens []string) bool {
	if got == "" {
		return false
	}
	gb := []byte(got)
	for _, t := range tokens {
		tb := []byte(strings.TrimSpace(t))
		if len(tb) == 0 {
			continue
		}
		if subtle.ConstantTimeCompare(gb, tb) == 1 {
			return true
		}
	}
	return false
}
