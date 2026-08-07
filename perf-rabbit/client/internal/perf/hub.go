package perf

import "sync"

// Hub 是任务级别的"有新数据"唤醒信号广播器，不携带 payload 本身——订阅者
// 收到信号后自己调用 Manager.GetTask 拉增量，复用已经跑通的
// snapshot(fromSample) 逻辑，不用另起一套推送数据的并发路径去维护一致性。
type Hub struct {
	mu          sync.Mutex
	subscribers map[string]map[chan struct{}]struct{}
}

// DefaultHub 是默认的任务通知广播器，跟 DefaultManager 配对使用。
var DefaultHub = NewHub()

// NewHub 创建一个空的任务通知广播器。
func NewHub() *Hub {
	return &Hub{
		subscribers: make(map[string]map[chan struct{}]struct{}),
	}
}

// Subscribe 订阅指定任务的"有更新"信号。
func (h *Hub) Subscribe(taskID string) (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)

	h.mu.Lock()
	if h.subscribers[taskID] == nil {
		h.subscribers[taskID] = make(map[chan struct{}]struct{})
	}
	h.subscribers[taskID][ch] = struct{}{}
	h.mu.Unlock()

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()

			if subs := h.subscribers[taskID]; subs != nil {
				delete(subs, ch)
				if len(subs) == 0 {
					delete(h.subscribers, taskID)
				}
			}
			close(ch)
		})
	}

	return ch, unsubscribe
}

// Notify 唤醒指定任务的所有订阅者。信号缓冲区只有 1，唤醒从不阻塞——
// 订阅者还没消费上一次信号时，重复 Notify 会被丢弃，反正订阅者下次醒来
// 会用 GetTask 把期间所有新样本一次性拿走，不会丢数据。
func (h *Hub) Notify(taskID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for ch := range h.subscribers[taskID] {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
