// Package event — 事件总线 (Redis Pub/Sub + Outbox)
// 对应架构文档 §10.1 事件总线
package event

import (
	"context"
	"sync"
	"time"
)

// 事件类型常量
const (
	EventAssetCreated     = "asset.created"
	EventAssetUpdated     = "asset.updated"
	EventAssetDeleted     = "asset.deleted"
	EventAssetAssigned    = "asset.assigned"
	EventAssetReleased    = "asset.released"
	EventAssetTransferred = "asset.transferred"
	EventLifecycleChanged = "asset.lifecycle_changed"
	EventAgentRegistered  = "agent.registered"
	EventAgentOnline      = "agent.online"
	EventAgentOffline     = "agent.offline"
)

// Event 事件
type Event struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	AssetID   string    `json:"asset_id,omitempty"`
	OrgID     string    `json:"org_id"`
	UserID    string    `json:"user_id,omitempty"`
	Data      []byte    `json:"data"`
	Timestamp time.Time `json:"timestamp"`
}

// EventBus 事件总线接口
type EventBus interface {
	Publish(ctx context.Context, event *Event) error
	Subscribe(ctx context.Context, eventType string) (<-chan *Event, error)
}

// MemoryBus 内存事件总线 (开发环境)
// 生产环境: Redis Pub/Sub 多实例跨实例传播
type MemoryBus struct {
	mu          sync.RWMutex
	subscribers map[string][]chan *Event
}

// NewMemoryBus 返回新内存总线
func NewMemoryBus() *MemoryBus {
	return &MemoryBus{subscribers: make(map[string][]chan *Event)}
}

// DefaultBus 全局默认事件总线 (由 main.go 初始化订阅者)
var DefaultBus EventBus = NewMemoryBus()

func (b *MemoryBus) Publish(ctx context.Context, event *Event) error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	event.Timestamp = time.Now()

	// 发送给所有订阅者
	for _, ch := range b.subscribers[event.Type] {
		select {
		case ch <- event:
		default:
			// 订阅者 channel 满则丢弃 (生产用 Redis Stream 持久化)
		}
	}
	return nil
}

func (b *MemoryBus) Subscribe(ctx context.Context, eventType string) (<-chan *Event, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan *Event, 100)
	b.subscribers[eventType] = append(b.subscribers[eventType], ch)
	return ch, nil
}

// OutboxRepository 事件外发箱接口 (保证不丢失)
type OutboxRepository interface {
	InsertOutbox(ctx context.Context, event *Event) error
	FetchPending(ctx context.Context, limit int) ([]*Event, error)
	MarkSent(ctx context.Context, eventID string) error
}
