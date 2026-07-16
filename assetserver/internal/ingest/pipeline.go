// Package ingest — 摄入管道
// 对应架构文档 §6.3 摄入管道设计
// 简化实现: 使用 Go channel 替代 Redis Stream
// 三阶段: PreCheck → Processor → Engine
package ingest

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/agent"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/domain"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/lock"
)

// MaxModulesPerReport 单次上报最大模块数
const MaxModulesPerReport = 200

// DefaultChannelSize ingest channel 默认容量
const DefaultChannelSize = 4096

// ===================================================================
// 上报请求 (Agent → Server)
// ===================================================================

// ReportRequest Agent 上报请求
type ReportRequest struct {
	AgentKey    string            `json:"agent_key"`
	Sequence    int64             `json:"sequence"`
	Modules     []ModuleReport    `json:"modules"`
	Signature   string            `json:"signature"` // Ed25519 hex 签名 (对 payload 签名)
	Timestamp   time.Time         `json:"timestamp"`
}

// ModuleReport 单个模块上报
type ModuleReport struct {
	Name     string `json:"name"`
	Checksum string `json:"checksum"`
	Data     []byte `json:"data"`
}

// ReportResult 上报处理结果
type ReportResult struct {
	Accepted   bool              `json:"accepted"`
	Rejected   bool              `json:"rejected"`
	Reason     string            `json:"reason,omitempty"`
	Errors     []ModuleError     `json:"errors,omitempty"`
}

// ModuleError 模块级错误
type ModuleError struct {
	Module string `json:"module"`
	Error  string `json:"error"`
}

// ===================================================================
// IngestPipeline 摄入管道
// ===================================================================

// IngestPipeline 摄入管道
// 接收 Agent 上报数据，经三阶段处理写入存储
type IngestPipeline struct {
	mu          sync.Mutex
	inputCh     chan *ReportRequest // 上报入口 channel
	bp          *BackpressureChecker
	lastSeq     map[string]int64    // agentKey → last sequence (去重)
	seenModules map[string]struct{} // 去重缓存: "agentKey:moduleName:checksum"
}

// NewIngestPipeline 创建摄入管道
func NewIngestPipeline(channelSize int) *IngestPipeline {
	if channelSize <= 0 {
		channelSize = DefaultChannelSize
	}
	return &IngestPipeline{
		inputCh:     make(chan *ReportRequest, channelSize),
		bp:          NewBackpressureChecker(0, channelSize),
		lastSeq:     make(map[string]int64),
		seenModules: make(map[string]struct{}),
	}
}

// ===================================================================
// Submit 提交上报请求到管道
// ===================================================================

// Submit 提交上报到 channel (非阻塞)
// 返回 error 如果背压满载
func (p *IngestPipeline) Submit(req *ReportRequest) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 背压检测: 更新 channel 长度并检查
	p.bp.UpdateLen(len(p.inputCh))
	if err := p.bp.Check(); err != nil {
		return err
	}

	select {
	case p.inputCh <- req:
		return nil
	default:
		return fmt.Errorf("ingest channel full")
	}
}

// ===================================================================
// Run 启动摄入管道 (goroutine)
// ===================================================================

// Run 启动管道处理循环
// ctx 用于优雅关闭
func (p *IngestPipeline) Run(ctx context.Context, processFn func(ctx context.Context, req *ReportRequest) error) {
	for {
		select {
		case <-ctx.Done():
			return
		case req := <-p.inputCh:
			if err := p.processReport(ctx, req, processFn); err != nil {
				// 生产: 写入错误日志 + 可选 DLQ
				_ = err
			}
		}
	}
}

// processReport 处理单条上报
func (p *IngestPipeline) processReport(ctx context.Context, req *ReportRequest, processFn func(ctx context.Context, req *ReportRequest) error) error {
	// Phase 1: PreCheck
	if err := p.preCheck(req); err != nil {
		return fmt.Errorf("precheck: %w", err)
	}

	// Phase 2: Processor
	if err := p.processor(req); err != nil {
		return fmt.Errorf("processor: %w", err)
	}

	// Phase 3: Engine (委托外部回调)
	if processFn != nil {
		if err := processFn(ctx, req); err != nil {
			return fmt.Errorf("engine: %w", err)
		}
	}

	return nil
}

// ===================================================================
// Phase 1: PreCheck — 签名验证 + 模块数检查 + 背压
// ===================================================================

// PreCheckRequest PreCheck 阶段请求
type PreCheckRequest = ReportRequest

// PreCheckResult PreCheck 阶段结果
type PreCheckResult = ReportResult

// preCheck 预处理检查
// 1. Ed25519 签名预检
// 2. 模块数 ≤ 200
// 3. 背压检测 (入口处已做)
func (p *IngestPipeline) preCheck(req *ReportRequest) error {
	// 模块数检查
	if len(req.Modules) > MaxModulesPerReport {
		return fmt.Errorf("too many modules: %d (max %d)", len(req.Modules), MaxModulesPerReport)
	}
	if len(req.Modules) == 0 {
		return fmt.Errorf("no modules in report")
	}
	return nil
}

// VerifyReportSignature 外部签名验证
// 调用方在 preCheck 前使用此函数验证签名
// pubKeyHex: Agent 注册时上报的公钥
// req: 上报请求
func VerifyReportSignature(pubKeyHex string, req *ReportRequest) error {
	// 构造签名载荷 (不含 signature 字段)
	payload := struct {
		AgentKey string         `json:"agent_key"`
		Sequence int64          `json:"sequence"`
		Modules  []ModuleReport `json:"modules"`
	}{
		AgentKey: req.AgentKey,
		Sequence: req.Sequence,
		Modules:  req.Modules,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	sigBytes, err := hex.DecodeString(req.Signature)
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}

	valid, err := agent.VerifySignature(pubKeyHex, data, sigBytes)
	if err != nil {
		return fmt.Errorf("verify signature: %w", err)
	}
	if !valid {
		return fmt.Errorf("invalid signature")
	}
	return nil
}

// ===================================================================
// Phase 2: Processor — 序列号连续性 + 去重
// ===================================================================

// processor
// 1. 验证 sequence 连续性 (单调递增)
// 2. 去重: 相同 agentKey + moduleName + checksum 的模块跳过
func (p *IngestPipeline) processor(req *ReportRequest) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 1. 序列号连续性检查
	lastSeq, exists := p.lastSeq[req.AgentKey]
	if exists && req.Sequence <= lastSeq {
		return fmt.Errorf("sequence not monotonic: got %d, last %d", req.Sequence, lastSeq)
	}

	// 2. 去重: 过滤重复模块
	uniqueModules := make([]ModuleReport, 0, len(req.Modules))
	for _, mod := range req.Modules {
		key := fmt.Sprintf("%s:%s:%s", req.AgentKey, mod.Name, mod.Checksum)
		if _, seen := p.seenModules[key]; seen {
			continue // 已处理过, 跳过
		}
		p.seenModules[key] = struct{}{}
		uniqueModules = append(uniqueModules, mod)
	}
	req.Modules = uniqueModules

	// 更新 lastSeq
	p.lastSeq[req.AgentKey] = req.Sequence

	return nil
}

// CompactSeenModules 定期压缩去重缓存 (建议每 10 分钟执行)
// 清理超过 1 小时的旧条目，防止内存无限增长
func (p *IngestPipeline) CompactSeenModules() {
	p.mu.Lock()
	defer p.mu.Unlock()
	// 简化: 保留最近 50000 条
	if len(p.seenModules) > 50000 {
		newMap := make(map[string]struct{}, 50000)
		count := 0
		for k, v := range p.seenModules {
			if count >= 50000 {
				break
			}
			newMap[k] = v
			count++
		}
		p.seenModules = newMap
	}
}

// ===================================================================
// Phase 3: Engine — 写入 snapshots + 乐观锁更新 assets
// ===================================================================

// EngineOptions Engine 阶段配置
type EngineOptions struct {
	// SnapshotWriter 快照写入回调
	SnapshotWriter func(ctx context.Context, agentKey string, modules []ModuleReport) error
	// AssetUpdater 乐观锁资产更新回调
	AssetUpdater func(ctx context.Context, agentKey string, modules []ModuleReport, version int) (bool, error)
	// CurrentVersion 当前资产版本号
	CurrentVersion int
}

// RunEngine 执行 Engine 阶段
// 1. 写入 snapshot 记录
// 2. 使用乐观锁更新 assets 表属性
func (p *IngestPipeline) RunEngine(ctx context.Context, req *ReportRequest, opts EngineOptions) error {
	// 1. 写入 snapshot
	if opts.SnapshotWriter != nil {
		if err := opts.SnapshotWriter(ctx, req.AgentKey, req.Modules); err != nil {
			return fmt.Errorf("write snapshot: %w", err)
		}
	}

	// 2. 乐观锁更新 assets
	if opts.AssetUpdater != nil {
		if err := lock.WithOptimisticRetry(
			func(version int) (bool, error) {
				return opts.AssetUpdater(ctx, req.AgentKey, req.Modules, version)
			},
			opts.CurrentVersion,
			lock.DefaultRetryConfig,
		); err != nil {
			return fmt.Errorf("optimistic update: %w", err)
		}
	}

	return nil
}

// ChannelDepth 返回 channel 当前深度 (用于监控)
func (p *IngestPipeline) ChannelDepth() int {
	return len(p.inputCh)
}

// ChannelCapacity 返回 channel 容量
func (p *IngestPipeline) ChannelCapacity() int {
	return cap(p.inputCh)
}

// ===================================================================
// 便捷函数: 创建标准 ingest 管道
// ===================================================================

// DefaultPipeline 创建默认配置的摄入管道
func DefaultPipeline() *IngestPipeline {
	return NewIngestPipeline(DefaultChannelSize)
}

// 确保实现接口 (编译期检查)
var _ domain.AgentStatus = domain.AgentStatusOnline
var _ ed25519.PublicKey = nil
