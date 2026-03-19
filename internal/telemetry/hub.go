// Package telemetry manages gRPC server-streaming subscriptions for telemetry events.
// The Hub fans out worker.Telemetry events to all active StreamTelemetry callers.
package telemetry

import (
	"sync"

	"liquidity-guard-bot/internal/worker"
)

// Subscriber is a channel that receives Telemetry events for one gRPC stream.
type Subscriber chan worker.Telemetry

// Hub is a thread-safe fan-out broker.
// Workers push to the hub's inbound channel; gRPC handlers subscribe and receive.
type Hub struct {
	mu          sync.RWMutex
	subscribers map[uint64]subscription
	nextID      uint64
	inbound     chan worker.Telemetry
}

type subscription struct {
	ch     Subscriber
	filter map[string]struct{} // empty = all bots; non-empty = only listed botIDs
}

// NewHub creates a Hub and starts its dispatch goroutine.
// The returned channel is the inbound feed — workers write here.
func NewHub() (*Hub, chan<- worker.Telemetry) {
	ch := make(chan worker.Telemetry, 256)
	h := &Hub{
		subscribers: make(map[uint64]subscription),
		inbound:     ch,
	}
	go h.run()
	return h, ch
}

// Subscribe registers a new subscriber. botIDs may be empty to receive all events.
// Returns a channel that will receive matching events and an unsubscribe function.
func (h *Hub) Subscribe(botIDs []string) (Subscriber, func()) {
	filter := make(map[string]struct{}, len(botIDs))
	for _, id := range botIDs {
		filter[id] = struct{}{}
	}

	ch := make(Subscriber, 64)

	h.mu.Lock()
	id := h.nextID
	h.nextID++
	h.subscribers[id] = subscription{ch: ch, filter: filter}
	h.mu.Unlock()

	unsubscribe := func() {
		h.mu.Lock()
		delete(h.subscribers, id)
		h.mu.Unlock()
		// Drain the channel so the producer goroutine never blocks on a dead subscriber.
		for len(ch) > 0 {
			<-ch
		}
		close(ch)
	}
	return ch, unsubscribe
}

// run is the single dispatch goroutine; it never holds the lock while sending.
func (h *Hub) run() {
	for event := range h.inbound {
		h.mu.RLock()
		subs := make([]subscription, 0, len(h.subscribers))
		for _, s := range h.subscribers {
			subs = append(subs, s)
		}
		h.mu.RUnlock()

		for _, s := range subs {
			if len(s.filter) > 0 {
				if _, ok := s.filter[event.BotID]; !ok {
					continue
				}
			}
			// Non-blocking send — slow consumers miss events rather than stall the hub.
			select {
			case s.ch <- event:
			default:
			}
		}
	}
}
