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
	if !ok || d == nil || d.client == nil {
		return nil
	}

	// Gather resource usage via Docker stats when available.
	// Moby Engine v29 SDK uses client.ContainerStatsOptions{Stream: false} instead
	// of a boolean stream parameter. ContainerInspect requires ContainerInspectOptions
	// with Size: true to populate SizeRw, and returns ContainerInspectResult with
	// Container field containing the InspectResponse.
	stats, err := d.client.ContainerStats(ctx, h.ID, dockerStatsOptions())
	if err != nil || stats.Body == nil {
		return nil
	}
	defer func() { _ = stats.Body.Close() }()

	var sj struct {
		MemoryStats struct{ Usage, MaxUsage uint64 } `json:"memory_stats"`
		CPUStats    struct {
			CPUUsage struct{ TotalUsage uint64 } `json:"cpu_usage"`
		} `json:"cpu_stats"`
		// Docker v1.41 Stats JSON fields
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

	// Inspect for SizeRw if available. Moby v29 SDK requires Size: true in
	// options and returns ContainerInspectResult with Container.SizeRw.
	var sizeRw *int64
	if inspect, ierr := d.client.ContainerInspect(ctx, h.ID, dockerInspectOptionsWithSize()); ierr == nil {
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

// dockerStatsOptions returns the moby client.ContainerStatsOptions for a
// one-shot (non-streaming) stats call. Stream: false tells the daemon to
// return a single stats sample and close the connection.
func dockerStatsOptions() client.ContainerStatsOptions {
	return client.ContainerStatsOptions{Stream: false}
}

// dockerInspectOptionsWithSize returns the moby client.ContainerInspectOptions
// with Size: true to populate SizeRw and SizeRootFs in the response.
func dockerInspectOptionsWithSize() client.ContainerInspectOptions {
	return client.ContainerInspectOptions{Size: true}
}
