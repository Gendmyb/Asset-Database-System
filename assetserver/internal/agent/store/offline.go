// Package store — Agent 离线队列
// 对应架构文档 §6.7 离线存储与重试
// 简化实现: 内存队列 (slice + mutex)
// 生产使用: SQLite (modernc.org/sqlite, 零CGO)
package store

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// MaxQueueSize 队列最大容量
const MaxQueueSize = 10000

// DLQThreshold 死信队列阈值
// 单条消息重试超过此次数后移入 DLQ
const DLQThreshold = 100

// QueueItem 队列项
type QueueItem struct {
	ID         string          `json:"id"`
	Payload    json.RawMessage `json:"payload"`
	Retries    int             `json:"retries"`
	CreatedAt  time.Time       `json:"created_at"`
	NextRetry  time.Time       `json:"next_retry"`
}

// OfflineQueue 离线队列
// 网络不可用时暂存采集数据，恢复后自动重放
type OfflineQueue struct {
	mu       sync.Mutex
	items    []*QueueItem
	dlq      []*QueueItem // 死信队列 (超过重试阈值)
	maxSize  int          // 默认 MaxQueueSize
}

// NewOfflineQueue 创建离线队列
func NewOfflineQueue() *OfflineQueue {
	return &OfflineQueue{
		items:   make([]*QueueItem, 0),
		dlq:     make([]*QueueItem, 0),
		maxSize: MaxQueueSize,
	}
}

// Enqueue 入队
// 队列满时返回 error，不丢弃数据
func (q *OfflineQueue) Enqueue(payload json.RawMessage) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.items) >= q.maxSize {
		return fmt.Errorf("offline queue full: %d/%d items", len(q.items), q.maxSize)
	}

	item := &QueueItem{
		ID:        fmt.Sprintf("q-%d", time.Now().UnixNano()),
		Payload:   payload,
		Retries:   0,
		CreatedAt: time.Now(),
		NextRetry: time.Now(),
	}
	q.items = append(q.items, item)
	return nil
}

// Dequeue 出队 (FIFO)
// 返回最早一条未超过重试阈值的消息
func (q *OfflineQueue) Dequeue() *QueueItem {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.items) == 0 {
		return nil
	}

	// 找出第一条可重试的消息
	for i, item := range q.items {
		if time.Now().Before(item.NextRetry) {
			continue // 尚未到重试时间
		}
		// 移除该元素
		q.items = append(q.items[:i], q.items[i+1:]...)
		return item
	}
	return nil
}

// IncrementRetry 增加重试计数
// 超过 DLQThreshold 时移入死信队列
func (q *OfflineQueue) IncrementRetry(item *QueueItem) {
	q.mu.Lock()
	defer q.mu.Unlock()

	item.Retries++
	if item.Retries >= DLQThreshold {
		q.dlq = append(q.dlq, item)
		return
	}
	// 指数退避重试间隔: 2^retries 秒
	backoff := time.Duration(1<<item.Retries) * time.Second
	if backoff > 5*time.Minute {
		backoff = 5 * time.Minute // 最大 5 分钟退避
	}
	item.NextRetry = time.Now().Add(backoff)
	q.items = append(q.items, item)
}

// Requeue 重新入队 (发送失败时)
func (q *OfflineQueue) Requeue(item *QueueItem) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = append(q.items, item)
}

// Len 当前队列长度
func (q *OfflineQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

// DLQLen 死信队列长度
func (q *OfflineQueue) DLQLen() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.dlq)
}

// DrainDLQ 清空死信队列并返回所有项 (管理员手动处理)
func (q *OfflineQueue) DrainDLQ() []*QueueItem {
	q.mu.Lock()
	defer q.mu.Unlock()
	items := q.dlq
	q.dlq = make([]*QueueItem, 0)
	return items
}

// Stats 返回队列统计信息
func (q *OfflineQueue) Stats() map[string]int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return map[string]int{
		"pending":  len(q.items),
		"dead":     len(q.dlq),
		"capacity": q.maxSize,
	}
}
