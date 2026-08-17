package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"helmdex/internal/helmutil"
)

// event is a server-sent progress event (applies, catalog syncs, bundled
// helm downloads). The web UI listens on /api/events.
type event struct {
	Type     string `json:"type"`
	Instance string `json:"instance,omitempty"`
	Message  string `json:"message,omitempty"`
}

type eventBroker struct {
	mu   sync.Mutex
	subs map[chan event]struct{}
}

func newEventBroker() *eventBroker {
	return &eventBroker{subs: map[chan event]struct{}{}}
}

func (b *eventBroker) publish(ev event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- ev:
		default:
			// Slow subscriber: drop rather than block mutations.
		}
	}
}

func (b *eventBroker) subscribe() chan event {
	ch := make(chan event, 16)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *eventBroker) unsubscribe(ch chan event) {
	b.mu.Lock()
	delete(b.subs, ch)
	b.mu.Unlock()
}

// StartHelmEventForwarding wires bundled-helm download events into the
// broker. Call once per process (the sink is global); returns a stop func.
func (s *Server) StartHelmEventForwarding() func() {
	events := make(chan helmutil.BundledHelmEvent, 8)
	helmutil.SetBundledHelmEventSink(events)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			case ev := <-events:
				msg := ev.Version
				if ev.Err != "" {
					msg = ev.Err
				}
				s.events.publish(event{Type: "helm." + string(ev.Kind), Message: msg})
			}
		}
	}()
	return func() {
		helmutil.SetBundledHelmEventSink(nil)
		close(done)
	}
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpError(w, http.StatusInternalServerError, fmt.Errorf("streaming unsupported by connection"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := s.events.subscribe()
	defer s.events.unsubscribe(ch)

	// Initial comment so clients know the stream is live.
	_, _ = fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-ch:
			b, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			_, _ = fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()
		}
	}
}
