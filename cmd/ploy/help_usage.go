package main

import (
	"fmt"
	"io"
)

func printNodeUsage(w io.Writer) {
	lines := []string{
		"Usage: ploy node <command>",
		"",
		"Commands:",
		"  add       Register a new node with the cluster beacon",
		"  remove    Deregister a node after draining workloads",
		"  list      List registered nodes with health summaries",
		"  heal      Run automated remediation on a node",
		"  logs      Stream node daemon logs via SSE",
	}
	writeUsageLines(w, lines)
}

func printDeployUsage(w io.Writer) {
	lines := []string{
		"Usage: ploy deploy <command>",
		"",
		"Commands:",
		"  bootstrap  Bootstrap a new cluster from a configuration file",
		"  upgrade    Roll out binary or configuration updates to nodes",
	}
	writeUsageLines(w, lines)
}

func printClusterUsage(w io.Writer) {
	lines := []string{
		"Usage: ploy cluster <command>",
		"",
		"Commands:",
		"  connect  Cache beacon metadata and trust bundles locally",
		"  list     Show locally cached cluster descriptors",
	}
	writeUsageLines(w, lines)
}

func printBeaconUsage(w io.Writer) {
	lines := []string{
		"Usage: ploy beacon <command>",
		"",
		"Commands:",
		"  promote    Promote a node to become the active beacon",
		"  rotate-ca  Rotate the cluster certificate authority bundle",
		"  sync       Refresh beacon discovery data and trust material",
	}
	writeUsageLines(w, lines)
}

func printConfigUsage(w io.Writer) {
	lines := []string{
		"Usage: ploy config <command>",
		"",
		"Commands:",
		"  gitlab <command>  Manage GitLab integration credentials",
		"  show              Display the effective cluster configuration",
		"  set               Update a configuration key/value pair",
	}
	writeUsageLines(w, lines)
}

func printLogsUsage(w io.Writer) {
	lines := []string{
		"Usage: ploy logs <command>",
		"",
		"Commands:",
		"  job <job-id>    Stream logs for a specific job via SSE",
		"  node <node-id>  Stream daemon logs from a node via SSE",
	}
	writeUsageLines(w, lines)
}

func printStatusUsage(w io.Writer) {
	lines := []string{
		"Usage: ploy status",
		"",
		"Summarize control plane health, node status, and Mods activity.",
	}
	writeUsageLines(w, lines)
}

func printDoctorUsage(w io.Writer) {
	lines := []string{
		"Usage: ploy doctor",
		"",
		"Run workstation diagnostics covering Docker, beacon reachability, and trust bundles.",
	}
	writeUsageLines(w, lines)
}

func writeUsageLines(w io.Writer, lines []string) {
	for _, line := range lines {
		_, _ = fmt.Fprintln(w, line)
	}
}
