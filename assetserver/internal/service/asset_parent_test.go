// Package service — G8 资产关系服务层单测 (防循环)
package service

import (
	"context"
	"errors"
	"testing"
)

// TestMountAsset_SelfReference 挂载到自身应立即返回 ErrCycleDetected, 不触碰数据库。
func TestMountAsset_SelfReference(t *testing.T) {
	svc := NewAssetService(nil, nil)
	_, err := svc.MountAsset(context.Background(), nil, "asset-1", "asset-1", "org-1", "user-1")
	if !errors.Is(err, ErrCycleDetected) {
		t.Fatalf("self-mount: want ErrCycleDetected, got %v", err)
	}
}

// TestUnmountAsset_NilPool 未触达 DB 前的路径不在此测试; 此用例仅验证
// MountAsset 的自引用守卫不依赖任何依赖注入 (nil repo/pool)。
func TestMountAsset_SelfReferenceNoDeps(t *testing.T) {
	svc := NewAssetService(nil, nil)
	_, err := svc.MountAsset(context.Background(), nil, "X", "X", "org", "u")
	if !errors.Is(err, ErrCycleDetected) {
		t.Fatalf("expected ErrCycleDetected, got %v", err)
	}
}

// TestSentinelErrorsDefined 确保 G8 sentinel errors 已定义且可比较
func TestSentinelErrorsDefined(t *testing.T) {
	if ErrInvalidParent == nil {
		t.Fatal("ErrInvalidParent must be defined")
	}
	if ErrCycleDetected == nil {
		t.Fatal("ErrCycleDetected must be defined")
	}
	// 验证 errors.Is 链路
	if !errors.Is(ErrCycleDetected, ErrCycleDetected) {
		t.Fatal("ErrCycleDetected must match itself")
	}
}
