package logevent

import (
	"sync"
	"time"
)

const subscriberBufferSize = 16

// Event is the small, step-level progress message pushed to WebSocket clients.
//
// Do not add request bodies, response bodies, raw headers, or similarly large
// or sensitive fields here. Detailed request/response data belongs in result
// storage, not in the live log event stream.
type Event struct {
	TaskID    string    `json:"task_id"`
	AccountID string    `json:"account_id"`
	SeqNo     int       `json:"seq_no"`
	StepName  string    `json:"step_name"`
	Success   bool      `json:"success"`
	ErrMsg    string    `json:"err_msg,omitempty"`
	CostMs    int64     `json:"cost_ms"`
	Timestamp time.Time `json:"timestamp"`
}

// Hub broadcasts task log events to subscribers.
type Hub struct {
	mu          sync.RWMutex
	subscribers map[string]map[*subscriber]struct{}
}

type subscriber struct {
	ch   chan Event
	once sync.Once
}

// NewHub creates an empty task log event hub.
func NewHub() *Hub {
	return &Hub{
		subscribers: make(map[string]map[*subscriber]struct{}),
	}
}

// Subscribe subscribes to events for a TaskID.
//
// Multiple subscribers may watch the same TaskID. The returned unsubscribe
// function is safe to call repeatedly and concurrently with Subscribe, Publish,
// and other unsubscribe calls.
func (h *Hub) Subscribe(taskID string) (<-chan Event, func()) {
	sub := &subscriber{ch: make(chan Event, subscriberBufferSize)}

	h.mu.Lock()
	if h.subscribers[taskID] == nil {
		h.subscribers[taskID] = make(map[*subscriber]struct{})
	}
	h.subscribers[taskID][sub] = struct{}{}
	h.mu.Unlock()

	unsubscribe := func() {
		sub.once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()

			if subscribers := h.subscribers[taskID]; subscribers != nil {
				delete(subscribers, sub)
				if len(subscribers) == 0 {
					delete(h.subscribers, taskID)
				}
			}
			close(sub.ch)
		})
	}

	return sub.ch, unsubscribe
}

// Publish broadcasts event to all subscribers for event.TaskID.
//
// Publish never blocks on slow subscribers. If a subscriber's channel is full,
// that subscriber simply drops this event while other subscribers still receive
// it when they have capacity.
func (h *Hub) Publish(event Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for sub := range h.subscribers[event.TaskID] {
		select {
		case sub.ch <- event:
		default:
		}
	}
}

// SubscriberCount returns the current number of subscribers for a TaskID.
func (h *Hub) SubscriberCount(taskID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return len(h.subscribers[taskID])
}
