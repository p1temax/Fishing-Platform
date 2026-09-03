package utils

import (
	"os"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
)

type HostMetrics struct {
	Hostname           string  `json:"hostname"`
	OS                 string  `json:"os"`
	Arch               string  `json:"arch"`
	CPUCores           int     `json:"cpu_cores"`
	CPUUsagePercent    float64 `json:"cpu_usage_percent"`
	MemoryTotalMB      uint64  `json:"memory_total_mb"`
	MemoryUsedMB       uint64  `json:"memory_used_mb"`
	MemoryUsagePercent float64 `json:"memory_usage_percent"`
	DiskTotalGB        float64 `json:"disk_total_gb"`
	DiskUsedGB         float64 `json:"disk_used_gb"`
	DiskUsagePercent   float64 `json:"disk_usage_percent"`
	UptimeSeconds      int64   `json:"uptime_seconds"`
	CollectedAt        string  `json:"collected_at"`
}

func CollectHostMetrics() HostMetrics {
	hostname, _ := os.Hostname()
	metrics := HostMetrics{
		Hostname:           hostname,
		OS:                 runtime.GOOS,
		Arch:               runtime.GOARCH,
		CPUCores:           runtime.NumCPU(),
		CPUUsagePercent:    -1,
		MemoryUsagePercent: -1,
		DiskUsagePercent:   -1,
		CollectedAt:        time.Now().Format(time.RFC3339),
	}

	if cpuCount, err := cpu.Counts(false); err == nil && cpuCount > 0 {
		metrics.CPUCores = cpuCount
	}

	if percentages, err := cpu.Percent(200*time.Millisecond, false); err == nil && len(percentages) > 0 {
		metrics.CPUUsagePercent = roundFloat(percentages[0], 1)
	}

	if virtualMemory, err := mem.VirtualMemory(); err == nil {
		metrics.MemoryTotalMB = virtualMemory.Total / 1024 / 1024
		metrics.MemoryUsedMB = virtualMemory.Used / 1024 / 1024
		metrics.MemoryUsagePercent = roundFloat(virtualMemory.UsedPercent, 1)
	}

	if usage, err := disk.Usage(diskProbePath()); err == nil {
		metrics.DiskTotalGB = bytesToGB(usage.Total)
		metrics.DiskUsedGB = bytesToGB(usage.Used)
		metrics.DiskUsagePercent = roundFloat(usage.UsedPercent, 1)
	}

	if uptime, err := host.Uptime(); err == nil {
		metrics.UptimeSeconds = int64(uptime)
	}

	if info, err := host.Info(); err == nil {
		if info.Hostname != "" {
			metrics.Hostname = info.Hostname
		}
		if info.OS != "" {
			metrics.OS = info.OS
		}
		if info.KernelArch != "" {
			metrics.Arch = info.KernelArch
		}
	}

	return metrics
}

func diskProbePath() string {
	if cwd, err := os.Getwd(); err == nil && cwd != "" {
		return cwd
	}
	return "."
}

func bytesToGB(value uint64) float64 {
	return roundFloat(float64(value)/(1024*1024*1024), 1)
}

func roundFloat(value float64, precision int) float64 {
	factor := 1.0
	for i := 0; i < precision; i++ {
		factor *= 10
	}

	return float64(int(value*factor+0.5)) / factor
}
