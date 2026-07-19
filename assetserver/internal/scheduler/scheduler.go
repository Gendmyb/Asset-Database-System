// Package scheduler — 轻量定时调度器
// Wave 1 G4: 到期主动提醒
//
// 职责:
//   - 定期扫描资产质保 (warranty_until) 临近到期/已过期 → 发事件 asset.warranty_expiring / asset.warranty_expired
//   - 定期扫描临时领用逾期未还 → 发事件 assignment.overdue
//   - 定时调用 LDAP 同步 (若启用) 保持 AD 用户与本地一致
//   - 每次运行写一条审计摘要 (audit_log, actor="system")
//
// 启动: 由 main.go 拉起; 间隔由 env SCHEDULER_INTERVAL 控制 (默认 "off" 不启动)。
// 停止: ctx cancel 优雅退出。
//
// 设计: 通过 Scanner / Bus / AuditWriter / LDAPSyncer 接口注入依赖,
// 便于单测用假实现替换。
package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/audit"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/event"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// 事件类型 (扩展 event 包)
const (
	EventWarrantyExpiring  = "asset.warranty_expiring"
	EventWarrantyExpired   = "asset.warranty_expired"
	EventAssignmentOverdue = "assignment.overdue"
)

// SystemActorID 调度器使用的系统审计 actor 标识 (写入审计摘要 JSON 用于归属)
// 注意: audit_log.user_id 列为 UUID 外键, "system" 非合法 UUID, 故 DB 列存 NULL,
// 仅在 summary JSON 中保留 "system" 归属信息。
const SystemActorID = "system"

// auditActorDB 是写入 audit_log.user_id 列的值 (空串 → NULL, 避免 FK 违约)
const auditActorDB = ""

// WarrantyRow 质保扫描行 (与 repository.WarrantyExpiringRow 对齐)
type WarrantyRow struct {
	AssetID       string    `json:"asset_id"`
	AssetTag      string    `json:"asset_tag"`
	Name          string    `json:"name"`
	OrgID         string    `json:"org_id"`
	WarrantyUntil time.Time `json:"warranty_until"`
	Expired       bool      `json:"expired"`
}

// OverdueRow 逾期领用扫描行 (与 repository.OverdueAssignmentRow 对齐)
type OverdueRow struct {
	AssignmentID string    `json:"assignment_id"`
	AssetID      string    `json:"asset_id"`
	AssetTag     string    `json:"asset_tag"`
	AssetName    string    `json:"asset_name"`
	OrgID        string    `json:"org_id"`
	AssignedTo   string    `json:"assigned_to"`
	DueDate      time.Time `json:"due_date"`
}

// Scanner 数据扫描接口 (生产由 repo 实现, 测试用 fake)
type Scanner interface {
	ScanWarrantyExpiring(ctx context.Context, days int) ([]WarrantyRow, error)
	ScanOverdueAssignments(ctx context.Context) ([]OverdueRow, error)
}

// Bus 事件总线接口 (event.EventBus 的子集)
type Bus interface {
	Publish(ctx context.Context, event *event.Event) error
}

// AuditWriter 审计写入接口: 每次扫描运行写一条摘要
type AuditWriter interface {
	WriteSummary(ctx context.Context, orgID, actorID, action string, summary any) error
}

// LDAPSyncer LDAP 同步接口 (ldap.SyncService.RunSyncOnce 的适配)
type LDAPSyncer interface {
	RunSyncOnce(ctx context.Context, actorID, orgID string) error
}

// Config 调度器配置
type Config struct {
	Interval       time.Duration // 扫描间隔; 0 表示不启动
	WarrantyDays   int           // 质保临近到期阈值 (默认 30)
	EnableLDAPSync bool          // 是否在循环中调用 LDAP 同步
	DefaultOrgID   string        // 系统级默认 org (审计/同步用)
}

// Scheduler 调度器
type Scheduler struct {
	cfg     Config
	scanner Scanner
	bus     Bus
	audit   AuditWriter
	ldap    LDAPSyncer
}

// New 构造调度器。ldap 可为 nil (未启用 LDAP 时)
func New(cfg Config, scanner Scanner, bus Bus, auditW AuditWriter, ldap LDAPSyncer) *Scheduler {
	if cfg.WarrantyDays <= 0 {
		cfg.WarrantyDays = 30
	}
	return &Scheduler{cfg: cfg, scanner: scanner, bus: bus, audit: auditW, ldap: ldap}
}

// Run 启动调度循环; ctx cancel 后优雅退出。
// 立即执行一次, 然后按 Interval 周期执行。
func (s *Scheduler) Run(ctx context.Context) {
	if s.cfg.Interval <= 0 {
		slog.Info("scheduler: disabled (interval <= 0)")
		return
	}
	slog.Info("scheduler: started", "interval", s.cfg.Interval, "warranty_days", s.cfg.WarrantyDays)

	// 立即跑一次 (启动后等待一个短延迟便于服务就绪)
	select {
	case <-ctx.Done():
		return
	case <-time.After(5 * time.Second):
	}
	s.runOnce(ctx)

	ticker := time.NewTicker(s.cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("scheduler: stopped")
			return
		case <-ticker.C:
			s.runOnce(ctx)
		}
	}
}

// runOnce 执行一次完整扫描 + 同步 + 审计摘要
func (s *Scheduler) runOnce(ctx context.Context) {
	start := time.Now()
	slog.Info("scheduler: scan run start")

	summary := struct {
		Actor             string `json:"actor"`
		WarrantyExpiring  int    `json:"warranty_expiring"`
		WarrantyExpired   int    `json:"warranty_expired"`
		AssignmentOverdue int    `json:"assignment_overdue"`
		LDAPSynced        bool   `json:"ldap_synced"`
		LDAPSyncErr       string `json:"ldap_sync_err,omitempty"`
		DurationMs        int64  `json:"duration_ms"`
	}{
		Actor: SystemActorID,
	}

	// 1. 质保到期扫描
	if warranties, err := s.scanner.ScanWarrantyExpiring(ctx, s.cfg.WarrantyDays); err != nil {
		slog.Warn("scheduler: scan warranty failed", "error", err)
	} else {
		for _, w := range warranties {
			s.publishWarrantyEvent(ctx, &w)
			if w.Expired {
				summary.WarrantyExpired++
			} else {
				summary.WarrantyExpiring++
			}
		}
	}

	// 2. 逾期领用扫描
	if overdue, err := s.scanner.ScanOverdueAssignments(ctx); err != nil {
		slog.Warn("scheduler: scan overdue assignments failed", "error", err)
	} else {
		for _, o := range overdue {
			s.publishOverdueEvent(ctx, &o)
			summary.AssignmentOverdue++
		}
	}

	// 3. LDAP 同步 (若启用)
	if s.cfg.EnableLDAPSync && s.ldap != nil {
		summary.LDAPSynced = true
		// actorID 传空串: ldap sync 内部写 audit_log.user_id, "system" 非 UUID 会触发 FK 违约;
		// 归属信息已记录在本次调度摘要的 actor 字段中。
		if err := s.ldap.RunSyncOnce(ctx, auditActorDB, s.cfg.DefaultOrgID); err != nil {
			summary.LDAPSyncErr = err.Error()
			slog.Warn("scheduler: ldap sync failed", "error", err)
		}
	}

	summary.DurationMs = time.Since(start).Milliseconds()

	// 4. 审计摘要 (一次运行一条)
	if s.audit != nil {
		if err := s.audit.WriteSummary(ctx, s.cfg.DefaultOrgID, auditActorDB, "scheduler_scan", summary); err != nil {
			slog.Warn("scheduler: write audit summary failed", "error", err)
		}
	}

	slog.Info("scheduler: scan run complete",
		"warranty_expiring", summary.WarrantyExpiring,
		"warranty_expired", summary.WarrantyExpired,
		"assignment_overdue", summary.AssignmentOverdue,
		"ldap_synced", summary.LDAPSynced,
		"duration_ms", summary.DurationMs)
}

// publishWarrantyEvent 发布质保到期事件
func (s *Scheduler) publishWarrantyEvent(ctx context.Context, w *WarrantyRow) {
	evtType := EventWarrantyExpiring
	if w.Expired {
		evtType = EventWarrantyExpired
	}
	data, _ := json.Marshal(map[string]any{
		"asset_id":       w.AssetID,
		"asset_tag":      w.AssetTag,
		"name":           w.Name,
		"warranty_until": w.WarrantyUntil.Format(time.RFC3339),
		"expired":        w.Expired,
	})
	evt := &event.Event{
		ID:      uuid.NewString(),
		Type:    evtType,
		AssetID: w.AssetID,
		OrgID:   w.OrgID,
		Data:    data,
	}
	if err := s.bus.Publish(ctx, evt); err != nil {
		slog.Warn("scheduler: publish warranty event failed", "type", evtType, "error", err)
	}
}

// publishOverdueEvent 发布逾期领用事件
func (s *Scheduler) publishOverdueEvent(ctx context.Context, o *OverdueRow) {
	data, _ := json.Marshal(map[string]any{
		"assignment_id": o.AssignmentID,
		"asset_id":      o.AssetID,
		"asset_tag":     o.AssetTag,
		"asset_name":    o.AssetName,
		"assigned_to":   o.AssignedTo,
		"due_date":      o.DueDate.Format(time.RFC3339),
	})
	evt := &event.Event{
		ID:      uuid.NewString(),
		Type:    EventAssignmentOverdue,
		AssetID: o.AssetID,
		OrgID:   o.OrgID,
		Data:    data,
	}
	if err := s.bus.Publish(ctx, evt); err != nil {
		slog.Warn("scheduler: publish overdue event failed", "error", err)
	}
}

// ParseInterval 解析调度间隔字符串。
// 支持: "off"/"" → 0 (不启动); "30m"/"1h"/"24h" 等标准 Go duration; 数字 → 秒。
func ParseInterval(s string) (time.Duration, error) {
	if s == "" || s == "off" {
		return 0, nil
	}
	d, err := time.ParseDuration(s)
	if err == nil {
		return d, nil
	}
	// 纯数字 → 秒
	var secs int
	if _, err := fmt.Sscanf(s, "%d", &secs); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second, nil
	}
	return 0, fmt.Errorf("invalid scheduler interval %q (use 'off', '30m', '1h', etc.)", s)
}

// PoolAuditWriter 基于 pgxpool 的审计摘要写入器
// 在独立事务中写一条 audit_log 摘要 (不绑资产生命周期)。
type PoolAuditWriter struct {
	pool *pgxpool.Pool
}

// NewPoolAuditWriter 构造 (传入 *pgxpool.Pool)
func NewPoolAuditWriter(pool *pgxpool.Pool) *PoolAuditWriter {
	return &PoolAuditWriter{pool: pool}
}

// WriteSummary 写一条审计摘要
func (w *PoolAuditWriter) WriteSummary(ctx context.Context, orgID, actorID, action string, summary any) error {
	if w.pool == nil {
		return nil
	}
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	summaryJSON, err := json.Marshal(summary)
	if err != nil {
		return fmt.Errorf("marshal summary: %w", err)
	}

	if err := audit.Record(ctx, tx, audit.Entry{
		TableName: "scheduler",
		RecordID:  "",
		Action:    action,
		OrgID:     orgID,
		ActorID:   actorID,
		NewValues: summaryJSON,
	}); err != nil {
		return fmt.Errorf("record audit: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit audit: %w", err)
	}
	return nil
}
