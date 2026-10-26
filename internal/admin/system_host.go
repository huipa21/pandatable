package admin

import (
	"context"
	"math"
	"os"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/process"
)

type cpuSample struct{ total, idle float64 }

func cpuCounters(value cpu.TimesStat) cpuSample {
	// Guest counters are already included in user/nice on Linux.
	return cpuSample{total: value.User + value.Nice + value.System + value.Idle +
		value.Iowait + value.Irq + value.Softirq + value.Steal,
		idle: value.Idle + value.Iowait}
}

func cpuDelta(at time.Time, before *cpuSample, after cpuSample) Metric[float64] {
	if before == nil {
		return unavailable[float64](at, "initializing")
	}
	total, idle := after.total-before.total, after.idle-before.idle
	if total <= 0 || idle < 0 || idle > total || math.IsNaN(total) || math.IsInf(total, 0) ||
		math.IsNaN(idle) || math.IsInf(idle, 0) {
		return unavailable[float64](at, "initializing")
	}
	return ready(at, 100*(total-idle)/total)
}

func (source *systemSource) host(ctx context.Context, result *SystemSnapshot) {
	at := result.CollectedAt
	times, err := cpu.TimesWithContext(ctx, false)
	if err != nil || len(times) != 1 {
		result.CPUPercent = unavailable[float64](at, "io")
		source.previous = nil
	} else {
		current := cpuCounters(times[0])
		result.CPUPercent = cpuDelta(at, source.previous, current)
		source.previous = &current
	}
	if runtime.GOOS == "windows" {
		// gopsutil's Windows load average starts a process-global sampling worker.
		result.Load = unavailable[LoadValues](at, "unsupported")
	} else {
		average, err := load.AvgWithContext(ctx)
		if err != nil {
			result.Load = unavailable[LoadValues](at, filesystemReason(err))
		} else {
			result.Load = ready(at, LoadValues{One: average.Load1, Five: average.Load5, Fifteen: average.Load15})
		}
	}
	memory, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		result.Memory = unavailable[MemoryValues](at, "io")
	} else {
		result.Memory = ready(at, MemoryValues{TotalBytes: memory.Total, UsedBytes: memory.Used, AvailableBytes: memory.Available})
	}
	app, err := process.NewProcessWithContext(ctx, int32(os.Getpid()))
	if err != nil {
		result.AppRSSBytes = unavailable[uint64](at, "io")
	} else if memory, err := app.MemoryInfoWithContext(ctx); err != nil {
		result.AppRSSBytes = unavailable[uint64](at, "io")
	} else {
		result.AppRSSBytes = ready(at, memory.RSS)
	}
	usage, err := disk.UsageWithContext(ctx, source.dataDir)
	if err != nil {
		result.Disk = unavailable[DiskValues](at, filesystemReason(err))
	} else {
		result.Disk = ready(at, DiskValues{TotalBytes: usage.Total, UsedBytes: usage.Used, FreeBytes: usage.Free})
	}
	result.Container = containerMemory(ctx, at)
	for _, metric := range []struct{ name, reason string }{
		{"cpu", result.CPUPercent.Reason}, {"load", result.Load.Reason}, {"memory", result.Memory.Reason},
		{"appRss", result.AppRSSBytes.Reason}, {"disk", result.Disk.Reason}, {"container", result.Container.Reason},
	} {
		if metric.reason != "" && metric.reason != "initializing" && metric.reason != "unsupported" {
			source.logger.WarnContext(ctx, "system metric unavailable", "section", metric.name, "reason", metric.reason)
		}
	}
}
