// Package scheduler — 调度器单测 (Wave 1 G4)
package scheduler

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/event"
)

// fakeScanner 假扫描器
type fakeScanner struct {
	warranty []WarrantyRow
	overdue  []OverdueRow
	wErr     error
	oErr     error
}

func (s *fakeScanner) ScanWarrantyExpiring(ctx context.Context, days int) ([]WarrantyRow, error) {
	return s.warranty, s.wErr
}
func (s *fakeScanner) ScanOverdueAssignments(ctx context.Context) ([]OverdueRow, error) {
	return s.overdue, s.oErr
}

// fakeBus 假事件总线
type fakeBus struct {
	mu     sync.Mutex
	events []*event.Event
}

func (b *fakeBus) Publish(ctx context.Context, e *event.Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, e)
	return nil
}

func (b *fakeBus) snapshot() []*event.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]*event.Event, len(b.events))
	copy(out, b.events)
	return out
}

// fakeAudit 假审计写入器
type fakeAudit struct {
	mu      sync.Mutex
	entries []struct {
		orgID, actorID, action string
		summary                any
	}
}

func (a *fakeAudit) WriteSummary(ctx context.Context, orgID, actorID, action string, summary any) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.entries = append(a.entries, struct {
		orgID, actorID, action string
		summary                any
	}{orgID, actorID, action, summary})
	return nil
}

// fakeLDAP 假 LDAP 同步器
type fakeLDAP struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (l *fakeLDAP) RunSyncOnce(ctx context.Context, actorID, orgID string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls++
	return l.err
}

func TestRunOnce_PublishesEventsAndAudit(t *testing.T) {
	scanner := &fakeScanner{
		warranty: []WarrantyRow{
			{AssetID: "a1", AssetTag: "AST-1", Name: "N1", OrgID: "org1", WarrantyUntil: time.Now().Add(10 * 24 * time.Hour), Expired: false},
			{AssetID: "a2", AssetTag: "AST-2", Name: "N2", OrgID: "org1", WarrantyUntil: time.Now().Add(-1 * 24 * time.Hour), Expired: true},
		},
		overdue: []OverdueRow{
			{AssignmentID: "as1", AssetID: "a3", AssetTag: "AST-3", AssetName: "N3", OrgID: "org2", DueDate: time.Now().Add(-5 * 24 * time.Hour)},
		},
	}
	bus := &fakeBus{}
	auditW := &fakeAudit{}
	ldap := &fakeLDAP{}

	sched := New(Config{Interval: time.Hour, WarrantyDays: 30, EnableLDAPSync: true, DefaultOrgID: "org-default"},
		scanner, bus, auditW, ldap)

	// 直接调用 runOnce (跳过 ticker 循环)
	sched.runOnce(context.Background())

	evts := bus.snapshot()
	if len(evts) != 3 {
		t.Fatalf("expected 3 events, got %d", len(evts))
	}
	// 校验事件类型
	types := map[string]int{}
	for _, e := range evts {
		types[e.Type]++
	}
	if types[EventWarrantyExpiring] != 1 {
		t.Errorf("warranty_expiring events = %d, want 1", types[EventWarrantyExpiring])
	}
	if types[EventWarrantyExpired] != 1 {
		t.Errorf("warranty_expired events = %d, want 1", types[EventWarrantyExpired])
	}
	if types[EventAssignmentOverdue] != 1 {
		t.Errorf("assignment_overdue events = %d, want 1", types[EventAssignmentOverdue])
	}

	// 校验事件 OrgID 透传
	for _, e := range evts {
		if e.OrgID == "" {
			t.Errorf("event %s missing org_id", e.Type)
		}
		if e.ID == "" {
			t.Errorf("event %s missing id", e.Type)
		}
	}

	// LDAP 同步应被调用一次
	if ldap.calls != 1 {
		t.Errorf("ldap calls = %d, want 1", ldap.calls)
	}

	// 审计应写一条摘要
	if len(auditW.entries) != 1 {
		t.Fatalf("audit entries = %d, want 1", len(auditW.entries))
	}
	ae := auditW.entries[0]
	if ae.action != "scheduler_scan" {
		t.Errorf("audit action = %q, want scheduler_scan", ae.action)
	}
	// audit_log.user_id 列为 UUID FK, 调度器写空串 (→ NULL), 归属记录在 summary.actor
	if ae.actorID != "" {
		t.Errorf("audit actor = %q, want empty (NULL in DB)", ae.actorID)
	}
	// summary 应带 actor=system 归属
	rv := reflect.ValueOf(ae.summary)
	actorField := rv.FieldByName("Actor")
	if !actorField.IsValid() || actorField.String() != SystemActorID {
		t.Errorf("summary.actor = %q, want %s", actorField.String(), SystemActorID)
	}
}

func TestRunOnce_LDAPErrorsRecorded(t *testing.T) {
	scanner := &fakeScanner{}
	bus := &fakeBus{}
	auditW := &fakeAudit{}
	ldap := &fakeLDAP{err: errors.New("ldap down")}

	sched := New(Config{Interval: time.Hour, WarrantyDays: 30, EnableLDAPSync: true, DefaultOrgID: "org"},
		scanner, bus, auditW, ldap)
	sched.runOnce(context.Background())

	if ldap.calls != 1 {
		t.Errorf("ldap calls = %d, want 1", ldap.calls)
	}
	// 审计摘要应记录 ldap_sync_err
	if len(auditW.entries) != 1 {
		t.Fatalf("audit entries = %d, want 1", len(auditW.entries))
	}
	// summary 是匿名 struct, 用反射读取 LDAPSyncErr 字段
	rv := reflect.ValueOf(auditW.entries[0].summary)
	if !rv.IsValid() {
		t.Fatal("summary invalid")
	}
	errField := rv.FieldByName("LDAPSyncErr")
	if !errField.IsValid() || errField.String() == "" {
		t.Errorf("LDAPSyncErr not recorded in summary")
	}
}

func TestParseInterval(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
		ok   bool
	}{
		{"off", 0, true},
		{"", 0, true},
		{"30m", 30 * time.Minute, true},
		{"1h", time.Hour, true},
		{"24h", 24 * time.Hour, true},
		{"60", 60 * time.Second, true},
		{"garbage", 0, false},
	}
	for _, c := range cases {
		got, err := ParseInterval(c.in)
		if c.ok && err != nil {
			t.Errorf("ParseInterval(%q) err = %v, want nil", c.in, err)
			continue
		}
		if !c.ok && err == nil {
			t.Errorf("ParseInterval(%q) err = nil, want error", c.in)
			continue
		}
		if got != c.want {
			t.Errorf("ParseInterval(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestRun_ContextCancelStops(t *testing.T) {
	sched := New(Config{Interval: 10 * time.Millisecond, WarrantyDays: 30},
		&fakeScanner{}, &fakeBus{}, &fakeAudit{}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		sched.Run(ctx)
		close(done)
	}()
	// 给 runOnce 5s 初始延迟 + ticker 触发机会
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("scheduler did not stop after ctx cancel")
	}
}
