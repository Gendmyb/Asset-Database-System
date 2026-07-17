// Package collector — 采集器接口与平台实现
// 对应架构文档 §6.5 Agent 采集器, §9.4 采集模块
package collector

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
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
// 通用工具函数
// ===================================================================

// runCommand 执行命令并返回 stdout 字符串 (去首尾空白)
func runCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// makeModule 创建 ModuleData，自动计算 Checksum
func makeModule(name string, data interface{}) (ModuleData, error) {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return ModuleData{}, fmt.Errorf("marshal module %q: %w", name, err)
	}
	h := sha256.Sum256(jsonBytes)
	return ModuleData{
		Name:     name,
		Checksum: hex.EncodeToString(h[:]),
		Data:     jsonBytes,
	}, nil
}

// readFileLines 读取文件内容并按行返回
func readFileLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	return lines, nil
}

// ===================================================================
// Linux 采集器 — 真实系统数据采集
// ===================================================================

// LinuxCollector Linux 平台采集器
// 读取 /proc, /sys, /etc/os-release 等系统信息
type LinuxCollector struct{}

func (c *LinuxCollector) Platform() string { return "linux" }

func (c *LinuxCollector) ListModules() []string {
	return []string{"cpu", "memory", "disk", "network", "os", "bios", "processes", "packages"}
}

func (c *LinuxCollector) Collect(moduleName string) ([]ModuleData, error) {
	var allModules []ModuleData
	var errs []string

	collectors := map[string]func() (ModuleData, error){
		"cpu":       c.collectCPU,
		"memory":    c.collectMemory,
		"disk":      c.collectDisk,
		"network":   c.collectNetwork,
		"os":        c.collectOS,
		"bios":      c.collectBIOS,
		"processes": c.collectProcesses,
		"packages":  c.collectPackages,
	}

	if moduleName != "" {
		if fn, ok := collectors[moduleName]; ok {
			md, err := fn()
			if err != nil {
				return nil, fmt.Errorf("collect %q: %w", moduleName, err)
			}
			return []ModuleData{md}, nil
		}
		return nil, fmt.Errorf("module %q not supported on linux", moduleName)
	}

	// 采集所有模块
	for name, fn := range collectors {
		md, err := fn()
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		allModules = append(allModules, md)
	}

	if len(allModules) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("all modules failed: %s", strings.Join(errs, "; "))
	}

	return allModules, nil
}

func (c *LinuxCollector) collectCPU() (ModuleData, error) {
	lines, err := readFileLines("/proc/cpuinfo")
	if err != nil {
		return makeModule("cpu", map[string]interface{}{
			"model":    "unknown",
			"platform": "linux",
			"error":    err.Error(),
		})
	}

	var modelName string
	physicalCores := make(map[string]bool)
	logicalCores := 0

	for _, line := range lines {
		if strings.HasPrefix(line, "model name") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				modelName = strings.TrimSpace(parts[1])
			}
		}
		if strings.HasPrefix(line, "physical id") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				physicalCores[strings.TrimSpace(parts[1])] = true
			}
		}
		if strings.HasPrefix(line, "processor") {
			logicalCores++
		}
	}

	// Fallback: 读取 /sys/devices/system/cpu/ 目录
	if physicalCount := len(physicalCores); physicalCount == 0 {
		entries, _ := os.ReadDir("/sys/devices/system/cpu/")
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), "cpu") && len(e.Name()) > 3 {
				if _, err := strconv.Atoi(e.Name()[3:]); err == nil {
					logicalCores++
				}
			}
		}
	}

	return makeModule("cpu", map[string]interface{}{
		"model":          modelName,
		"physical_cores": len(physicalCores),
		"logical_cores":  logicalCores,
		"platform":       "linux",
	})
}

func (c *LinuxCollector) collectMemory() (ModuleData, error) {
	lines, err := readFileLines("/proc/meminfo")
	if err != nil {
		return makeModule("memory", map[string]interface{}{
			"total_kb": 0,
			"platform": "linux",
			"error":    err.Error(),
		})
	}

	var totalKB int64
	for _, line := range lines {
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				totalKB, _ = strconv.ParseInt(fields[1], 10, 64)
			}
			break
		}
	}

	return makeModule("memory", map[string]interface{}{
		"total_kb": totalKB,
		"platform": "linux",
	})
}

func (c *LinuxCollector) collectDisk() (ModuleData, error) {
	out, err := runCommand("df", "-h")
	if err != nil {
		// 备选: 读取 /proc/mounts
		mounts, readErr := readFileLines("/proc/mounts")
		if readErr != nil {
			return makeModule("disk", map[string]interface{}{
				"output":   "",
				"platform": "linux",
				"error":    err.Error(),
			})
		}
		out = strings.Join(mounts, "\n")
	}

	return makeModule("disk", map[string]interface{}{
		"output":   out,
		"platform": "linux",
	})
}

func (c *LinuxCollector) collectNetwork() (ModuleData, error) {
	// 优先使用 ip addr，降级到 ifconfig
	out, err := runCommand("ip", "addr")
	if err != nil {
		out, err = runCommand("ifconfig")
		if err != nil {
			return makeModule("network", map[string]interface{}{
				"output":   "",
				"platform": "linux",
				"error":    err.Error(),
			})
		}
	}

	return makeModule("network", map[string]interface{}{
		"output":   out,
		"platform": "linux",
	})
}

func (c *LinuxCollector) collectOS() (ModuleData, error) {
	lines, err := readFileLines("/etc/os-release")
	if err != nil {
		// 备选: lsb_release
		out, lsbErr := runCommand("lsb_release", "-a")
		if lsbErr != nil {
			return makeModule("os", map[string]interface{}{
				"name":     "unknown",
				"platform": "linux",
				"error":    err.Error(),
			})
		}
		return makeModule("os", map[string]interface{}{
			"output":   out,
			"platform": "linux",
		})
	}

	info := map[string]string{}
	for _, line := range lines {
		// 跳过注释行
		if strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		val := strings.Trim(parts[1], `"`)
		info[parts[0]] = val
	}

	return makeModule("os", map[string]interface{}{
		"name":     info["PRETTY_NAME"],
		"id":       info["ID"],
		"version":  info["VERSION_ID"],
		"platform": "linux",
	})
}

func (c *LinuxCollector) collectBIOS() (ModuleData, error) {
	// dmidecode 需要 root 权限，失败时回退到 /sys
	out, err := runCommand("dmidecode", "-t", "bios")
	if err != nil {
		// 读取 DMI 信息 (无需 root)
		sysData := map[string]string{}
		for _, f := range []string{
			"/sys/class/dmi/id/bios_vendor",
			"/sys/class/dmi/id/bios_version",
			"/sys/class/dmi/id/bios_date",
			"/sys/class/dmi/id/product_name",
			"/sys/class/dmi/id/product_version",
		} {
			data, readErr := os.ReadFile(f)
			if readErr == nil {
				key := f[strings.LastIndex(f, "/")+1:]
				sysData[key] = strings.TrimSpace(string(data))
			}
		}
		if len(sysData) > 0 {
			return makeModule("bios", map[string]interface{}{
				"output":   fmt.Sprintf("%+v", sysData),
				"sys_data": sysData,
				"platform": "linux",
			})
		}
		return makeModule("bios", map[string]interface{}{
			"output":   "",
			"platform": "linux",
			"error":    err.Error(),
		})
	}

	return makeModule("bios", map[string]interface{}{
		"output":   out,
		"platform": "linux",
	})
}

func (c *LinuxCollector) collectProcesses() (ModuleData, error) {
	out, err := runCommand("ps", "-eo", "pid")
	if err != nil {
		// 备选: 读取 /proc 目录
		entries, readErr := os.ReadDir("/proc")
		if readErr != nil {
			return makeModule("processes", map[string]interface{}{
				"count":    0,
				"platform": "linux",
				"error":    err.Error(),
			})
		}
		count := 0
		for _, e := range entries {
			if _, err := strconv.Atoi(e.Name()); err == nil {
				count++
			}
		}
		return makeModule("processes", map[string]interface{}{
			"count":    count,
			"platform": "linux",
		})
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	// 去掉标题行
	count := 0
	if len(lines) > 1 {
		count = len(lines) - 1
	}

	return makeModule("processes", map[string]interface{}{
		"count":    count,
		"platform": "linux",
	})
}

func (c *LinuxCollector) collectPackages() (ModuleData, error) {
	// 优先 dpkg (Debian/Ubuntu)
	out, err := runCommand("dpkg-query", "-f", "${Package}\n", "-W")
	if err != nil {
		// 备选 rpm (RHEL/CentOS/Fedora)
		out, err = runCommand("rpm", "-qa")
		if err != nil {
			// 备选 apk (Alpine)
			out, err = runCommand("apk", "info")
			if err != nil {
				return makeModule("packages", map[string]interface{}{
					"count":    0,
					"platform": "linux",
					"error":    "no package manager found",
				})
			}
		}
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	count := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}

	return makeModule("packages", map[string]interface{}{
		"count":    count,
		"platform": "linux",
	})
}

// ===================================================================
// Darwin 采集器 — 真实 macOS 系统数据采集
// ===================================================================

// DarwinCollector macOS 平台采集器
// 调用 sysctl, sw_vers, system_profiler, pkgutil 等系统命令
type DarwinCollector struct{}

func (c *DarwinCollector) Platform() string { return "darwin" }

func (c *DarwinCollector) ListModules() []string {
	return []string{"cpu", "memory", "disk", "network", "os", "bios", "processes", "packages"}
}

func (c *DarwinCollector) Collect(moduleName string) ([]ModuleData, error) {
	var allModules []ModuleData
	var errs []string

	collectors := map[string]func() (ModuleData, error){
		"cpu":       c.collectCPU,
		"memory":    c.collectMemory,
		"disk":      c.collectDisk,
		"network":   c.collectNetwork,
		"os":        c.collectOS,
		"bios":      c.collectBIOS,
		"processes": c.collectProcesses,
		"packages":  c.collectPackages,
	}

	if moduleName != "" {
		if fn, ok := collectors[moduleName]; ok {
			md, err := fn()
			if err != nil {
				return nil, fmt.Errorf("collect %q: %w", moduleName, err)
			}
			return []ModuleData{md}, nil
		}
		return nil, fmt.Errorf("module %q not supported on darwin", moduleName)
	}

	// 采集所有模块
	for name, fn := range collectors {
		md, err := fn()
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		allModules = append(allModules, md)
	}

	if len(allModules) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("all modules failed: %s", strings.Join(errs, "; "))
	}

	return allModules, nil
}

func (c *DarwinCollector) collectCPU() (ModuleData, error) {
	brand, err := runCommand("sysctl", "-n", "machdep.cpu.brand_string")
	if err != nil {
		return makeModule("cpu", map[string]interface{}{
			"model":    "unknown",
			"platform": "darwin",
			"error":    err.Error(),
		})
	}

	physStr, physErr := runCommand("sysctl", "-n", "hw.physicalcpu")
	logicStr, logicErr := runCommand("sysctl", "-n", "hw.logicalcpu")

	physCores := 0
	logicCores := 0
	if physErr == nil {
		physCores, _ = strconv.Atoi(physStr)
	}
	if logicErr == nil {
		logicCores, _ = strconv.Atoi(logicStr)
	}

	return makeModule("cpu", map[string]interface{}{
		"model":          brand,
		"physical_cores": physCores,
		"logical_cores":  logicCores,
		"platform":       "darwin",
	})
}

func (c *DarwinCollector) collectMemory() (ModuleData, error) {
	memStr, err := runCommand("sysctl", "-n", "hw.memsize")
	if err != nil {
		return makeModule("memory", map[string]interface{}{
			"total_bytes": 0,
			"platform":    "darwin",
			"error":       err.Error(),
		})
	}

	totalBytes, _ := strconv.ParseUint(memStr, 10, 64)

	return makeModule("memory", map[string]interface{}{
		"total_bytes": totalBytes,
		"platform":    "darwin",
	})
}

func (c *DarwinCollector) collectDisk() (ModuleData, error) {
	out, err := runCommand("df", "-h")
	if err != nil {
		return makeModule("disk", map[string]interface{}{
			"output":   "",
			"platform": "darwin",
			"error":    err.Error(),
		})
	}

	return makeModule("disk", map[string]interface{}{
		"output":   out,
		"platform": "darwin",
	})
}

func (c *DarwinCollector) collectNetwork() (ModuleData, error) {
	// 采集网络接口信息
	ifconfigOut, ifconfigErr := runCommand("ifconfig")
	hwPortsOut, _ := runCommand("networksetup", "-listallhardwareports")
	dnsOut, _ := runCommand("scutil", "--dns")

	output := ""
	if ifconfigErr == nil {
		output = ifconfigOut
	}
	if hwPortsOut != "" {
		output += "\n--- Hardware Ports ---\n" + hwPortsOut
	}
	if dnsOut != "" {
		output += "\n--- DNS ---\n" + dnsOut
	}

	if output == "" {
		return makeModule("network", map[string]interface{}{
			"output":   "",
			"platform": "darwin",
			"error":    ifconfigErr.Error(),
		})
	}

	return makeModule("network", map[string]interface{}{
		"output":   output,
		"platform": "darwin",
	})
}

func (c *DarwinCollector) collectOS() (ModuleData, error) {
	out, err := runCommand("sw_vers")
	if err != nil {
		return makeModule("os", map[string]interface{}{
			"name":     "macOS",
			"platform": "darwin",
			"error":    err.Error(),
		})
	}

	// 解析 sw_vers 输出
	info := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			info[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}

	return makeModule("os", map[string]interface{}{
		"name":       info["ProductName"],
		"version":    info["ProductVersion"],
		"build":      info["BuildVersion"],
		"raw_output": out,
		"platform":   "darwin",
	})
}

func (c *DarwinCollector) collectBIOS() (ModuleData, error) {
	out, err := runCommand("system_profiler", "SPHardwareDataType")
	if err != nil {
		return makeModule("bios", map[string]interface{}{
			"output":   "",
			"platform": "darwin",
			"error":    err.Error(),
		})
	}

	return makeModule("bios", map[string]interface{}{
		"output":   out,
		"platform": "darwin",
	})
}

func (c *DarwinCollector) collectProcesses() (ModuleData, error) {
	out, err := runCommand("ps", "-eo", "pid")
	if err != nil {
		// 备选: ps aux
		out, err = runCommand("ps", "aux")
		if err != nil {
			return makeModule("processes", map[string]interface{}{
				"count":    0,
				"platform": "darwin",
				"error":    err.Error(),
			})
		}
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	count := 0
	if len(lines) > 1 {
		count = len(lines) - 1 // 去掉标题行
	}

	return makeModule("processes", map[string]interface{}{
		"count":    count,
		"platform": "darwin",
	})
}

func (c *DarwinCollector) collectPackages() (ModuleData, error) {
	out, err := runCommand("pkgutil", "--pkgs")
	if err != nil {
		// 备选: Homebrew (brew list)
		out, err = runCommand("brew", "list")
		if err != nil {
			return makeModule("packages", map[string]interface{}{
				"count":    0,
				"platform": "darwin",
				"error":    "no package manager found (pkgutil + brew failed)",
			})
		}
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	count := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}

	return makeModule("packages", map[string]interface{}{
		"count":    count,
		"platform": "darwin",
	})
}

// ===================================================================
// Windows 采集器 (mock 数据 — 保持不变)
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
