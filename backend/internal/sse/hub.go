package sse

import "sync"

type Hub struct {
	mu      sync.RWMutex
	clients map[string]map[chan string]struct{}
}

func NewHub() *Hub {
	return &Hub{clients: map[string]map[chan string]struct{}{}}
}

func (h *Hub) Subscribe(topic string) (chan string, func()) {
	ch := make(chan string, 64)
	h.mu.Lock()
	if h.clients[topic] == nil {
		h.clients[topic] = map[chan string]struct{}{}
	}
	h.clients[topic][ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		if subscribers, ok := h.clients[topic]; ok {
			delete(subscribers, ch)
			if len(subscribers) == 0 {
				delete(h.clients, topic)
			}
		}
		h.mu.Unlock()
		close(ch)
	}
}

func (h *Hub) Publish(topic string, line string) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.clients[topic] {
		select {
		case ch <- line:
		default:
		}
	}
}

func DeployTopic(taskID uint64) string {
	return "deploy:" + uintToString(taskID)
}

func AppTopic(projectID uint64) string {
	return "app:" + uintToString(projectID)
}

func uintToString(v uint64) string {
	if v == 0 {
		return "0"
	}
	buf := [20]byte{}
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
