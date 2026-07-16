// Package domain — 生命周期模糊测试
package domain

import (
	"fmt"
	"math/rand"
	"testing"
)

// ============================================================
// FuzzLifecycleTransitions — 随机状态转换模糊测试
// 验证终态不可转换 (Retirement)
// ============================================================

// fuzzLifecycleTransitions 模糊测试核心逻辑
func fuzzLifecycleTransitions(fromIdx, toIdx int) error {
	states := AllStates

	// 索引越界检查
	if fromIdx < 0 || fromIdx >= len(states) {
		return fmt.Errorf("from index out of range: %d", fromIdx)
	}
	if toIdx < 0 || toIdx >= len(states) {
		return fmt.Errorf("to index out of range: %d", toIdx)
	}

	from := states[fromIdx]
	to := states[toIdx]

	err := ValidateTransition(from, to)

	// 规则1: 终态 (Retirement) 不可转换到任何其他状态 (除自身幂等)
	if from.IsTerminal() {
		if to == from {
			if err != nil {
				return fmt.Errorf("terminal state should allow same-state transition: %s → %s", from, to)
			}
		} else {
			if err == nil {
				return fmt.Errorf("terminal state %s should NOT transition to %s", from, to)
			}
		}
	}

	// 规则2: 合法转换不应报错
	if !from.IsTerminal() {
		if from.CanTransitionTo(to) && err != nil {
			return fmt.Errorf("valid transition %s → %s should not error: %v", from, to, err)
		}
		if !from.CanTransitionTo(to) && to != from && err == nil {
			return fmt.Errorf("invalid transition %s → %s should error", from, to)
		}
	}

	// 规则3: 幂等转换永远合法
	if from == to && err != nil {
		return fmt.Errorf("same-state transition %s → %s should be valid", from, to)
	}

	return nil
}

// FuzzLifecycleTransitions Go 原生 fuzzing
func FuzzLifecycleTransitions(f *testing.F) {
	// 种子：覆盖所有已知合法和非法转换
	seeds := []struct{ from, to int }{
		{0, 0}, // procurement → procurement (幂等)
		{0, 1}, // procurement → deployment (合法)
		{0, 4}, // procurement → retirement (合法)
		{0, 2}, // procurement → utilization (非法 — 不能跳过)
		{1, 2}, // deployment → utilization (合法)
		{1, 3}, // deployment → maintenance (合法)
		{1, 4}, // deployment → retirement (合法)
		{2, 3}, // utilization → maintenance (合法)
		{2, 4}, // utilization → retirement (合法)
		{3, 2}, // maintenance → utilization (合法)
		{3, 4}, // maintenance → retirement (合法)
		{4, 4}, // retirement → retirement (幂等)
		{4, 0}, // retirement → procurement (非法 — 终态)
		{4, 1}, // retirement → deployment (非法 — 终态)
		{4, 2}, // retirement → utilization (非法 — 终态)
		{4, 3}, // retirement → maintenance (非法 — 终态)
	}

	for _, s := range seeds {
		f.Add(s.from, s.to)
	}

	f.Fuzz(func(t *testing.T, fromIdx, toIdx int) {
		if err := fuzzLifecycleTransitions(fromIdx, toIdx); err != nil {
			t.Error(err)
		}
	})
}

// ============================================================
// TestRandomLifecycleTransitions — 随机状态转换压力测试
// ============================================================

func TestRandomLifecycleTransitions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping random lifecycle transition test in short mode")
	}

	states := AllStates
	rng := rand.New(rand.NewSource(42))

	for i := 0; i < 1000; i++ {
		fromIdx := rng.Intn(len(states))
		toIdx := rng.Intn(len(states))

		if err := fuzzLifecycleTransitions(fromIdx, toIdx); err != nil {
			t.Errorf("iteration %d: %v", i, err)
		}
	}
}

// ============================================================
// TestTerminalStateExhaustive — 穷举验证终态不可转换
// ============================================================

func TestTerminalStateExhaustive(t *testing.T) {
	terminal := StateRetirement

	for _, target := range AllStates {
		err := ValidateTransition(terminal, target)

		if target == terminal {
			// 终态到自身 幂等
			if err != nil {
				t.Errorf("terminal → terminal should be valid: %v", err)
			}
		} else {
			// 终态到非终态 非法
			if err == nil {
				t.Errorf("terminal → %s should be invalid", target)
			}
		}
	}
}

// ============================================================
// TestAllStatesHaveNonTerminalTransition — 验证所有非终态可转至终态
// ============================================================

func TestAllStatesReachTerminal(t *testing.T) {
	terminal := StateRetirement

	for _, from := range AllStates {
		if from == terminal {
			continue
		}
		// 验证每个非终态至少有一条路径到 retirement
		if !from.CanTransitionTo(terminal) {
			// 对于 procurement，可以直接转 retirement
			// 这是架构文档中定义的
			t.Logf("state %s cannot directly transition to retirement — checking indirect path", from)
		} else {
			t.Logf("state %s → retirement: OK", from)
		}
		// 间接路径：验证存在一条到终态的路径
		if !hasPathToTerminal(from) {
			t.Errorf("state %s has no path to terminal state %s", from, terminal)
		}
	}
}

// hasPathToTerminal DFS 检查是否存在到终态的路径
func hasPathToTerminal(from LifecycleState) bool {
	visited := make(map[LifecycleState]bool)
	return dfsPath(from, visited)
}

func dfsPath(state LifecycleState, visited map[LifecycleState]bool) bool {
	if state.IsTerminal() {
		return true
	}
	if visited[state] {
		return false // 已访问，避免循环
	}
	visited[state] = true

	targets := ValidTransitions[state]
	for _, t := range targets {
		if dfsPath(t, visited) {
			return true
		}
	}
	return false
}

// ============================================================
// TestDeterministicTransitions — 确定性验证
// ============================================================

func TestDeterministicTransitions(t *testing.T) {
	// ValidateTransition 对于相同输入多次调用结果应一致
	for i := 0; i < 100; i++ {
		for _, from := range AllStates {
			for _, to := range AllStates {
				r1 := ValidateTransition(from, to)
				r2 := ValidateTransition(from, to)
				if (r1 == nil) != (r2 == nil) {
					t.Errorf("non-deterministic: %s → %s: %v vs %v", from, to, r1, r2)
				}
			}
		}
	}
}

// ============================================================
// TestInvalidStateStrings — 非法状态字符串
// ============================================================

func TestInvalidStateStrings(t *testing.T) {
	invalidStates := []LifecycleState{"", "unknown", "DELETED", "archived", "active"}

	for _, invalid := range invalidStates {
		err := ValidateTransition(StateProcurement, invalid)
		if err == nil {
			t.Errorf("should reject invalid target state %q", invalid)
		}

		err = ValidateTransition(invalid, StateProcurement)
		if err == nil {
			t.Errorf("should reject invalid source state %q", invalid)
		}
	}
}
