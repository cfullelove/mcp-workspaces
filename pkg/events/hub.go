package events

import (
	"sync"
	"time"
)

// Actor identifies the source of an event (API, MCP agent, filesystem watcher, user, etc).
type Actor struct {
	Kind    string  `json:"kind"`              // "api" | "mcp" | "fswatch" | "user"
	ID      *string `json:"id,omitempty"`      // stable id if available
	Display *string `json:"display,omitempty"` // human-friendly name
}

// WorkspaceEvent is the normalized change notification delivered to frontends.
type WorkspaceEvent struct {
	ID            int64   `json:"id"`                      // monotonically increasing per workspace
	TS            string  `json:"ts"`                      // RFC3339 timestamp
	WorkspaceID   string  `json:"workspaceId"`             // workspace scope
	Type          string  `json:"type"`                    // "file.created" | "file.updated" | "file.deleted" | "file.moved" | "dir.created" | "dir.deleted" | "presence.join" | "presence.leave" | "lock.acquired" | "lock.released"
	Path          string  `json:"path"`                    // canonical path (workspace-relative)
	PrevPath      *string `json:"prevPath,omitempty"`      // for moves/renames
	IsDir         bool    `json:"isDir"`                   // whether Path is a directory
	Size          *int64  `json:"size,omitempty"`          // optional size in bytes
	MTime         *string `json:"mtime,omitempty"`         // RFC3339 mtime, if known
	Actor         *Actor  `json:"actor,omitempty"`         // event initiator
	Commit        *string `json:"commit,omitempty"`        // workspace HEAD after mutation
	CorrelationID *string `json:"correlationId,omitempty"` // request correlation ID if provided
}

type subscriber struct {
	id int
	ch chan WorkspaceEvent
}

type workspaceState struct {
	seq       int64
	ring      []WorkspaceEvent // circular buffer
	ringCap   int
	ringStart int // index of oldest
	subs      map[int]subscriber
	nextSubID int
}

type Hub struct {
	mu     sync.RWMutex
	ws     map[string]*workspaceState
	cap    int
	closed bool
}

// NewHub creates an in-memory event hub with a per-workspace ring buffer capacity.
func NewHub(ringCapacity int) *Hub {
	if ringCapacity <= 0 {
		ringCapacity = 200
	}
	return &Hub{
		ws:  make(map[string]*workspaceState),
		cap: ringCapacity,
	}
}

func (h *Hub) getOrCreateWS(id string) *workspaceState {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil
	}
	st, ok := h.ws[id]
	if !ok {
		st = &workspaceState{
			seq:       0,
			ring:      make([]WorkspaceEvent, 0, h.cap),
			ringCap:   h.cap,
			ringStart: 0,
			subs:      make(map[int]subscriber),
			nextSubID: 1,
		}
		h.ws[id] = st
	}
	return st
}

// Publish appends an event to the workspace ring and fanouts to subscribers.
// It sets the event ID and timestamp if not set.
func (h *Hub) Publish(workspaceID string, evt WorkspaceEvent) {
	ws := h.getOrCreateWS(workspaceID)
	if ws == nil {
		return
	}

	// Fill defaults
	if evt.TS == "" {
		evt.TS = time.Now().UTC().Format(time.RFC3339)
	}
	evt.WorkspaceID = workspaceID

	// Mutate state under lock, then fanout
	h.mu.Lock()
	ws.seq++
	evt.ID = ws.seq

	// Append to ring buffer (circular)
	if len(ws.ring) < ws.ringCap {
		ws.ring = append(ws.ring, evt)
	} else {
		// overwrite oldest
		ws.ring[ws.ringStart] = evt
		ws.ringStart = (ws.ringStart + 1) % ws.ringCap
	}

	// Snapshot subscribers to avoid holding lock during sends
	subs := make([]subscriber, 0, len(ws.subs))
	for _, s := range ws.subs {
		subs = append(subs, s)
	}
	h.mu.Unlock()

	// Non-blocking fanout; drop if buffer is full to avoid backpressure issues
	for _, s := range subs {
		select {
		case s.ch <- evt:
		default:
			// Drop oldest by draining one, then try again once
			select {
			case <-s.ch:
			default:
			}
			select {
			case s.ch <- evt:
			default:
				// Still blocked; skip
			}
		}
	}
}

// Subscribe registers a new subscriber for a workspace. If sinceID > 0,
// the hub will replay buffered events with ID > sinceID before delivering live events.
// Returns a receive-only channel and an unsubscribe function.
func (h *Hub) Subscribe(workspaceID string, sinceID int64, buffer int) (<-chan WorkspaceEvent, func()) {
	if buffer <= 0 {
		buffer = 64
	}
	ws := h.getOrCreateWS(workspaceID)
	ch := make(chan WorkspaceEvent, buffer)

	h.mu.Lock()
	if h.closed {
		close(ch)
		h.mu.Unlock()
		return ch, func() {}
	}
	id := ws.nextSubID
	ws.nextSubID++
	ws.subs[id] = subscriber{id: id, ch: ch}

	// Collect replay slice
	replay := h.collectSinceLocked(ws, sinceID)
	h.mu.Unlock()

	// Deliver replay asynchronously
	go func() {
		for _, e := range replay {
			select {
			case ch <- e:
			default:
				// Drop if subscriber is too slow during replay
			}
		}
	}()

	unsub := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if st, ok := h.ws[workspaceID]; ok {
			if s, exists := st.subs[id]; exists {
				delete(st.subs, id)
				close(s.ch)
			}
		}
	}
	return ch, unsub
}

func (h *Hub) collectSinceLocked(ws *workspaceState, sinceID int64) []WorkspaceEvent {
	if len(ws.ring) == 0 {
		return nil
	}
	out := make([]WorkspaceEvent, 0, len(ws.ring))
	// Iterate logical order from oldest to newest
	for i := 0; i < len(ws.ring); i++ {
		idx := (ws.ringStart + i) % ws.ringCap
		e := ws.ring[idx]
		// Uninitialized slots when ring not yet filled
		if e.WorkspaceID == "" && e.TS == "" && e.Type == "" {
			continue
		}
		if e.ID > sinceID {
			out = append(out, e)
		}
	}
	return out
}

// Close shuts down the hub and all subscriptions.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.closed = true
	for _, ws := range h.ws {
		for _, s := range ws.subs {
			close(s.ch)
		}
		ws.subs = map[int]subscriber{}
	}
}
