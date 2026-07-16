// Package ingest — 背压控制
// 对应架构文档 §6.4 摄入管道背压
package ingest

import (
	"fmt"
	"net/http"
)

// BackpressureChecker 背压检测器
// 监控 Go channel 的负载情况，满载时拒绝新请求
type BackpressureChecker struct {
	channelLen int     // channel 当前长度
	channelCap int     // channel 容量
	threshold  float64 // 满载阈值 (默认 0.8)
}

// NewBackpressureChecker 创建背压检测器
func NewBackpressureChecker(channelLen, channelCap int) *BackpressureChecker {
	return &BackpressureChecker{
		channelLen: channelLen,
		channelCap: channelCap,
		threshold:  0.8,
	}
}

// WithThreshold 设置自定义阈值
func (b *BackpressureChecker) WithThreshold(t float64) *BackpressureChecker {
	b.threshold = t
	return b
}

// Usage 当前使用率 (0.0 ~ 1.0)
func (b *BackpressureChecker) Usage() float64 {
	if b.channelCap == 0 {
		return 0
	}
	return float64(b.channelLen) / float64(b.channelCap)
}

// IsOverloaded 是否满载 (>80%)
// 满载时返回 true，调用方应返回 503
func (b *BackpressureChecker) IsOverloaded() bool {
	return b.Usage() >= b.threshold
}

// Check 执行背压检查
// 返回 nil 表示可以继续处理，返回 error 表示应拒绝请求 (503)
func (b *BackpressureChecker) Check() error {
	if b.IsOverloaded() {
		return &BackpressureError{
			Usage:      b.Usage(),
			Threshold:  b.threshold,
			StatusCode: http.StatusServiceUnavailable,
		}
	}
	return nil
}

// BackpressureError 背压错误
type BackpressureError struct {
	Usage      float64
	Threshold  float64
	StatusCode int
}

// Error 实现 error 接口
func (e *BackpressureError) Error() string {
	return fmt.Sprintf("backpressure: usage %.2f exceeds threshold %.2f",
		e.Usage, e.Threshold)
}

// UpdateLen 更新 channel 长度 (每次上报前调用)
func (b *BackpressureChecker) UpdateLen(newLen int) {
	b.channelLen = newLen
}
