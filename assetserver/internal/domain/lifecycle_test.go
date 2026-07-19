// Package domain — 生命周期状态机测试
package domain

import "testing"

func TestValidTransitions(t *testing.T) {
	tests := []struct {
		from, to LifecycleState
		valid    bool
	}{
		{StateDeployment, StateUtilization, true},
		{StateDeployment, StateMaintenance, true},
		{StateDeployment, StateRetirement, true},
		{StateUtilization, StateMaintenance, true},
		{StateUtilization, StateRetirement, true},
		{StateMaintenance, StateUtilization, true},
		{StateMaintenance, StateRetirement, true},
		// 终态不可转换
		{StateRetirement, StateUtilization, false},
		{StateRetirement, StateDeployment, false},
		// 相同状态允许 (幂等)
		{StateUtilization, StateUtilization, true},
	}

	for _, tc := range tests {
		err := ValidateTransition(tc.from, tc.to)
		got := err == nil
		if got != tc.valid {
			t.Errorf("ValidateTransition(%s, %s) = %v, want %v (err=%v)",
				tc.from, tc.to, got, tc.valid, err)
		}
	}
}

func TestTerminalState(t *testing.T) {
	if !StateRetirement.IsTerminal() {
		t.Error("StateRetirement should be terminal")
	}
	for _, s := range AllStates {
		if s == StateRetirement {
			continue
		}
		if s.IsTerminal() {
			t.Errorf("%s should not be terminal", s)
		}
	}
}
