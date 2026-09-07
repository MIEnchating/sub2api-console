package systeminfo

import (
	"context"
	"runtime"
	"strings"
	"testing"
)

func TestCollectorReadsHostMetricsForTemporaryFilesystem(t *testing.T) {
	snapshot, err := New(t.TempDir()).Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CPU.LogicalCores != runtime.NumCPU() || snapshot.CPU.UsagePercent < 0 || snapshot.CPU.UsagePercent > 100 {
		t.Fatalf("unexpected CPU snapshot: %#v", snapshot.CPU)
	}
	if snapshot.Memory.TotalBytes == 0 || snapshot.Memory.UsedBytes > snapshot.Memory.TotalBytes {
		t.Fatalf("unexpected memory snapshot: %#v", snapshot.Memory)
	}
	if snapshot.Disk.TotalBytes == 0 || snapshot.Disk.UsedBytes > snapshot.Disk.TotalBytes {
		t.Fatalf("unexpected disk snapshot: %#v", snapshot.Disk)
	}
}

func TestParseCPUTimesAndUsage(t *testing.T) {
	previous, err := parseCPUTimes(strings.NewReader("cpu  100 20 30 400 10 5 3 2 0 0\n"))
	if err != nil {
		t.Fatal(err)
	}
	current, err := parseCPUTimes(strings.NewReader("cpu  130 20 40 440 10 5 3 2 0 0\n"))
	if err != nil {
		t.Fatal(err)
	}
	usage := cpuUsagePercent(previous, current)
	if usage != 50 {
		t.Fatalf("usage=%v want=50", usage)
	}
}

func TestParseMemoryUsesAvailableMemory(t *testing.T) {
	memory, err := parseMemory(strings.NewReader(`MemTotal:       1000 kB
MemFree:         100 kB
MemAvailable:    250 kB
Buffers:          20 kB
Cached:          100 kB
`))
	if err != nil {
		t.Fatal(err)
	}
	if memory.TotalBytes != 1_024_000 || memory.AvailableBytes != 256_000 || memory.UsedBytes != 768_000 {
		t.Fatalf("unexpected memory snapshot: %#v", memory)
	}
	if memory.UsagePercent != 75 {
		t.Fatalf("usage=%v want=75", memory.UsagePercent)
	}
}

func TestParseMemoryRejectsMissingTotal(t *testing.T) {
	if _, err := parseMemory(strings.NewReader("MemAvailable: 100 kB\n")); err == nil {
		t.Fatal("expected missing total error")
	}
}
