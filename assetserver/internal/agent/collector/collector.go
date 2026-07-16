// Package collector — 采集器接口与平台实现
// 对应架构文档 §6.5 Agent 采集器
package collector

import (
	"fmt"
	"runtime"
)

// ModuleData 采集的模块数据
// 每个模块代表一个被监控的资产属性组 (如: packages, processes, disks)
type ModuleData struct {
	Name     string // 模块名称 (e.g. "packages", "processes", "disks")
	Checksum string // 模块数据 SHA256 校验和
	Data     []byte // 序列化后的模块数据 (JSON/MessagePack)
}

// Collector 采集器接口
// 每个平台 (linux/darwin/windows) 实现自己的采集逻辑
type Collector interface {
	// Collect 采集指定模块的数据
	// moduleName 为空时采集所有模块
	Collect(moduleName string) ([]ModuleData, error)

	// ListModules 返回该平台支持的模块列表
	ListModules() []string

	// Platform 返回平台标识
	Platform() string
}

// NewCollector 根据当前运行时平台创建采集器
// 自动检测 GOOS 并返回对应实现
func NewCollector() (Collector, error) {
	switch runtime.GOOS {
	case "linux":
		return &LinuxCollector{}, nil
	case "darwin":
		return &DarwinCollector{}, nil
	case "windows":
		return &WindowsCollector{}, nil
	default:
		return nil, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// ===================================================================
// Linux 采集器 (简化实现 — mock 数据)
// ===================================================================

// LinuxCollector Linux 平台采集器
// 生产实现: 调用 dpkg/rpm, /proc, sysfs 等
type LinuxCollector struct{}

func (c *LinuxCollector) Platform() string { return "linux" }

func (c *LinuxCollector) ListModules() []string {
	return []string{"packages", "processes", "disks", "network"}
}

func (c *LinuxCollector) Collect(moduleName string) ([]ModuleData, error) {
	// 简化: 返回 mock 数据
	// 生产: dpkg-query -W 读取已安装包、/proc/meminfo 读取内存等
	allModules := []ModuleData{
		{Name: "packages", Checksum: "mock-sha256-linux-pkgs", Data: []byte(`{"total": 0, "platform": "linux"}`)},
		{Name: "processes", Checksum: "mock-sha256-linux-procs", Data: []byte(`{"count": 0, "platform": "linux"}`)},
		{Name: "disks", Checksum: "mock-sha256-linux-disks", Data: []byte(`{"devices": [], "platform": "linux"}`)},
		{Name: "network", Checksum: "mock-sha256-linux-net", Data: []byte(`{"interfaces": [], "platform": "linux"}`)},
	}

	if moduleName == "" {
		return allModules, nil
	}
	for _, m := range allModules {
		if m.Name == moduleName {
			return []ModuleData{m}, nil
		}
	}
	return nil, fmt.Errorf("module %q not supported on linux", moduleName)
}

// ===================================================================
// Darwin 采集器 (简化实现 — mock 数据)
// ===================================================================

// DarwinCollector macOS 平台采集器
// 生产实现: system_profiler, pkgutil, IOKit 等
type DarwinCollector struct{}

func (c *DarwinCollector) Platform() string { return "darwin" }

func (c *DarwinCollector) ListModules() []string {
	return []string{"packages", "processes", "disks", "network"}
}

func (c *DarwinCollector) Collect(moduleName string) ([]ModuleData, error) {
	// 简化: 返回 mock 数据
	// 生产: system_profiler SPApplicationsDataType, diskutil list 等
	allModules := []ModuleData{
		{Name: "packages", Checksum: "mock-sha256-darwin-pkgs", Data: []byte(`{"total": 0, "platform": "darwin"}`)},
		{Name: "processes", Checksum: "mock-sha256-darwin-procs", Data: []byte(`{"count": 0, "platform": "darwin"}`)},
		{Name: "disks", Checksum: "mock-sha256-darwin-disks", Data: []byte(`{"devices": [], "platform": "darwin"}`)},
		{Name: "network", Checksum: "mock-sha256-darwin-net", Data: []byte(`{"interfaces": [], "platform": "darwin"}`)},
	}

	if moduleName == "" {
		return allModules, nil
	}
	for _, m := range allModules {
		if m.Name == moduleName {
			return []ModuleData{m}, nil
		}
	}
	return nil, fmt.Errorf("module %q not supported on darwin", moduleName)
}

// ===================================================================
// Windows 采集器 (简化实现 — mock 数据)
// ===================================================================

// WindowsCollector Windows 平台采集器
// 生产实现: WMI, winget, registry 等
type WindowsCollector struct{}

func (c *WindowsCollector) Platform() string { return "windows" }

func (c *WindowsCollector) ListModules() []string {
	return []string{"packages", "processes", "disks", "network"}
}

func (c *WindowsCollector) Collect(moduleName string) ([]ModuleData, error) {
	// 简化: 返回 mock 数据
	// 生产: Get-WmiObject Win32_Product, Get-Process 等
	allModules := []ModuleData{
		{Name: "packages", Checksum: "mock-sha256-windows-pkgs", Data: []byte(`{"total": 0, "platform": "windows"}`)},
		{Name: "processes", Checksum: "mock-sha256-windows-procs", Data: []byte(`{"count": 0, "platform": "windows"}`)},
		{Name: "disks", Checksum: "mock-sha256-windows-disks", Data: []byte(`{"devices": [], "platform": "windows"}`)},
		{Name: "network", Checksum: "mock-sha256-windows-net", Data: []byte(`{"interfaces": [], "platform": "windows"}`)},
	}

	if moduleName == "" {
		return allModules, nil
	}
	for _, m := range allModules {
		if m.Name == moduleName {
			return []ModuleData{m}, nil
		}
	}
	return nil, fmt.Errorf("module %q not supported on windows", moduleName)
}
