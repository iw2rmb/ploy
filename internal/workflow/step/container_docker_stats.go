package step

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/moby/moby/client"
)

func collectDockerResourceUsage(
	ctx context.Context,
	rt ContainerRuntime,
	h ContainerHandle,
	spec ContainerSpec,
) *contracts.BuildGateResourceUsage {
	d, ok := rt.(*DockerContainerRuntime)
	if !ok || d == nil || d.stats == nil {
		return nil
	}

	stats, err := d.stats.ContainerStats(ctx, string(h), dockerStatsOptions())
	if err != nil || stats.Body == nil {
		return nil
	}
	defer func() { _ = stats.Body.Close() }()

	var sj struct {
		MemoryStats struct{ Usage, MaxUsage uint64 } `json:"memory_stats"`
		CPUStats    struct {
			CPUUsage struct{ TotalUsage uint64 } `json:"cpu_usage"`
		} `json:"cpu_stats"`
		BlkioStats struct {
			IoServiceBytesRecursive []struct {
				Op    string
				Value uint64
			}
		} `json:"blkio_stats"`
	}
	_ = json.NewDecoder(stats.Body).Decode(&sj)

	var readBytes, writeBytes uint64
	for _, rec := range sj.BlkioStats.IoServiceBytesRecursive {
		switch strings.ToLower(rec.Op) {
		case "read":
			readBytes += rec.Value
		case "write":
			writeBytes += rec.Value
		}
	}

	var sizeRw *int64
	if inspect, ierr := d.client.ContainerInspect(ctx, string(h), dockerInspectOptionsWithSize()); ierr == nil {
		if inspect.Container.SizeRw != nil {
			size := *inspect.Container.SizeRw
			sizeRw = &size
		}
	}

	return &contracts.BuildGateResourceUsage{
		LimitNanoCPUs:    spec.LimitNanoCPUs,
		LimitMemoryBytes: spec.LimitMemoryBytes,
		CPUTotalNs:       sj.CPUStats.CPUUsage.TotalUsage,
		MemUsageBytes:    sj.MemoryStats.Usage,
		MemMaxBytes:      sj.MemoryStats.MaxUsage,
		BlkioReadBytes:   readBytes,
		BlkioWriteBytes:  writeBytes,
		SizeRwBytes:      sizeRw,
	}
}

func dockerStatsOptions() client.ContainerStatsOptions {
	return client.ContainerStatsOptions{Stream: false}
}

func dockerInspectOptionsWithSize() client.ContainerInspectOptions {
	return client.ContainerInspectOptions{Size: true}
}
