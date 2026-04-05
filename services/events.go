package services

import (
	"encoding/json"
	"sync"
)

// EventDispatcher 事件分发接口
type EventDispatcher interface {
	Emit(event string, data interface{})
}

// SSEEventDispatcher 基于 SSE 的事件分发器
type SSEEventDispatcher struct {
	mu       sync.RWMutex
	clients  map[chan string]struct{}
}

// NewSSEEventDispatcher 创建 SSE 事件分发器
func NewSSEEventDispatcher() *SSEEventDispatcher {
	return &SSEEventDispatcher{
		clients: make(map[chan string]struct{}),
	}
}

func (d *SSEEventDispatcher) Emit(event string, data interface{}) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return
	}
	msg := "event: " + event + "\ndata: " + string(jsonData) + "\n\n"

	d.mu.RLock()
	defer d.mu.RUnlock()
	for ch := range d.clients {
		select {
		case ch <- msg:
		default:
			// 客户端缓冲满，跳过
		}
	}
}

// Subscribe 订阅事件流
func (d *SSEEventDispatcher) Subscribe() chan string {
	ch := make(chan string, 64)
	d.mu.Lock()
	d.clients[ch] = struct{}{}
	d.mu.Unlock()
	return ch
}

// Unsubscribe 取消订阅
func (d *SSEEventDispatcher) Unsubscribe(ch chan string) {
	d.mu.Lock()
	delete(d.clients, ch)
	d.mu.Unlock()
	close(ch)
}

// 全局事件分发器
var GlobalEvents EventDispatcher = &noopDispatcher{}

type noopDispatcher struct{}

func (n *noopDispatcher) Emit(event string, data interface{}) {}
