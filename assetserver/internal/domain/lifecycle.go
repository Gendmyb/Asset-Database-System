// Package domain — 资产生命周期状态机
// 对应架构文档 §5.4
package domain

import "fmt"

// LifecycleState 生命周期状态
type LifecycleState string

const (
	StateProcurement LifecycleState = "procurement"
	StateDeployment  LifecycleState = "deployment"
	StateUtilization LifecycleState = "utilization"
	StateMaintenance LifecycleState = "maintenance"
	StateRetirement  LifecycleState = "retirement"
)

// ValidTransitions 合法转换矩阵
// 对应架构文档 §5.4
var ValidTransitions = map[LifecycleState][]LifecycleState{
	StateProcurement: {StateDeployment, StateRetirement},
	StateDeployment:  {StateUtilization, StateMaintenance, StateRetirement},
	StateUtilization: {StateMaintenance, StateRetirement},
	StateMaintenance: {StateUtilization, StateRetirement},
	StateRetirement:  {}, // 终态
}

// IsTerminal 是否为终态
func (s LifecycleState) IsTerminal() bool {
	return s == StateRetirement
}

// CanTransitionTo 检查是否可转换到目标状态
func (s LifecycleState) CanTransitionTo(target LifecycleState) bool {
	targets, ok := ValidTransitions[s]
	if !ok {
		return false
	}
	for _, t := range targets {
		if t == target {
			return true
		}
	}
	return false
}

// ValidateTransition 验证状态转换 (返回 error 如果非法)
func ValidateTransition(from, to LifecycleState) error {
	if from == to {
		return nil // 允许相同状态 (幂等)
	}
	if from.IsTerminal() {
		return fmt.Errorf("cannot transition from terminal state %s", from)
	}
	if !from.CanTransitionTo(to) {
		return fmt.Errorf("invalid transition: %s → %s", from, to)
	}
	return nil
}

// AllStates 所有合法状态
var AllStates = []LifecycleState{
	StateProcurement,
	StateDeployment,
	StateUtilization,
	StateMaintenance,
	StateRetirement,
}
