package systeminfo

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

type CPU struct {
	UsagePercent float64 `json:"usage_percent"`
	LogicalCores int     `json:"logical_cores"`
}

type Storage struct {
	TotalBytes     uint64  `json:"total_bytes"`
	UsedBytes      uint64  `json:"used_bytes"`
	AvailableBytes uint64  `json:"available_bytes"`
	UsagePercent   float64 `json:"usage_percent"`
}

type Snapshot struct {
	SampledAt string  `json:"sampled_at"`
	CPU       CPU     `json:"cpu"`
	Memory    Storage `json:"memory"`
	Disk      Storage `json:"disk"`
}

type cpuTimes struct {
	total uint64
	idle  uint64
}

type Collector struct {
	diskPath string

	mu       sync.Mutex
	previous cpuTimes
}

func New(diskPath string) *Collector {
	collector := &Collector{diskPath: diskPath}
	if current, err := readCPUTimes(); err == nil {
		collector.previous = current
	}
	return collector
}

func (c *Collector) Snapshot(ctx context.Context) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	c.mu.Lock()
	current, err := readCPUTimes()
	if err != nil {
		c.mu.Unlock()
		return Snapshot{}, err
	}
	usage := cpuUsagePercent(c.previous, current)
	c.previous = current
	c.mu.Unlock()

	memoryFile, err := os.Open("/proc/meminfo")
	if err != nil {
		return Snapshot{}, fmt.Errorf("读取内存统计失败: %w", err)
	}
	memory, memoryErr := parseMemory(memoryFile)
	closeErr := memoryFile.Close()
	if memoryErr != nil {
		return Snapshot{}, memoryErr
	}
	if closeErr != nil {
		return Snapshot{}, fmt.Errorf("关闭内存统计失败: %w", closeErr)
	}
	disk, err := diskUsage(c.diskPath)
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{
		SampledAt: time.Now().UTC().Format(time.RFC3339Nano),
		CPU:       CPU{UsagePercent: usage, LogicalCores: runtime.NumCPU()},
		Memory:    memory,
		Disk:      disk,
	}, nil
}

func readCPUTimes() (cpuTimes, error) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return cpuTimes{}, fmt.Errorf("读取 CPU 统计失败: %w", err)
	}
	defer file.Close()
	return parseCPUTimes(file)
}

func parseCPUTimes(reader io.Reader) (cpuTimes, error) {
	scanner := bufio.NewScanner(reader)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return cpuTimes{}, fmt.Errorf("读取 CPU 统计失败: %w", err)
		}
		return cpuTimes{}, errors.New("CPU 统计为空")
	}
	fields := strings.Fields(scanner.Text())
	if len(fields) < 5 || fields[0] != "cpu" {
		return cpuTimes{}, errors.New("CPU 统计格式无效")
	}
	values := make([]uint64, 0, min(len(fields)-1, 8))
	for _, field := range fields[1:min(len(fields), 9)] {
		value, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return cpuTimes{}, errors.New("CPU 统计格式无效")
		}
		values = append(values, value)
	}
	var total uint64
	for _, value := range values {
		total += value
	}
	idle := values[3]
	if len(values) > 4 {
		idle += values[4]
	}
	return cpuTimes{total: total, idle: idle}, nil
}

func cpuUsagePercent(previous, current cpuTimes) float64 {
	if current.total <= previous.total || current.idle < previous.idle {
		return 0
	}
	totalDelta := current.total - previous.total
	idleDelta := min(current.idle-previous.idle, totalDelta)
	return roundedPercent(totalDelta-idleDelta, totalDelta)
}

func parseMemory(reader io.Reader) (Storage, error) {
	values := map[string]uint64{}
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		key := strings.TrimSuffix(fields[0], ":")
		if key != "MemTotal" && key != "MemAvailable" {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return Storage{}, errors.New("内存统计格式无效")
		}
		values[key] = value * 1024
	}
	if err := scanner.Err(); err != nil {
		return Storage{}, fmt.Errorf("读取内存统计失败: %w", err)
	}
	total := values["MemTotal"]
	available := values["MemAvailable"]
	if total == 0 {
		return Storage{}, errors.New("内存总量统计缺失")
	}
	available = min(available, total)
	used := total - available
	return Storage{
		TotalBytes: total, UsedBytes: used, AvailableBytes: available,
		UsagePercent: roundedPercent(used, total),
	}, nil
}

func diskUsage(path string) (Storage, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return Storage{}, fmt.Errorf("读取硬盘统计失败: %w", err)
	}
	blockSize := uint64(stat.Bsize)
	total := stat.Blocks * blockSize
	free := stat.Bfree * blockSize
	available := stat.Bavail * blockSize
	free = min(free, total)
	return Storage{
		TotalBytes: total, UsedBytes: total - free, AvailableBytes: available,
		UsagePercent: roundedPercent(total-free, total),
	}, nil
}

func roundedPercent(used, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return math.Round(float64(used)/float64(total)*1000) / 10
}
