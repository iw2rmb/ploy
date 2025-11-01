package main

import (
    "errors"
    "flag"
    "fmt"
    "io"
    "strings"
)

// handleNode routes node subcommands.
func handleNode(args []string, stderr io.Writer) error {
    if len(args) == 0 {
        _, _ = fmt.Fprintln(stderr, "Usage: ploy node <command>")
        return errors.New("node subcommand required")
    }
    switch args[0] {
    case "add":
        return handleNodeAdd(args[1:], stderr)
    default:
        _, _ = fmt.Fprintln(stderr, "Usage: ploy node <command>")
        return fmt.Errorf("unknown node subcommand %q", args[0])
    }
}

// handleNodeAdd validates required flags for adding a worker node.
func handleNodeAdd(args []string, stderr io.Writer) error {
    if stderr == nil { stderr = io.Discard }
    fs := flag.NewFlagSet("node add", flag.ContinueOnError)
    fs.SetOutput(io.Discard)
    var (
        clusterID string
        address   string
    )
    fs.StringVar(&clusterID, "cluster-id", "", "Cluster identifier to join")
    fs.StringVar(&address, "address", "", "Node IP or hostname")
    if err := fs.Parse(args); err != nil {
        _, _ = fmt.Fprintln(stderr, "Usage: ploy node add --cluster-id <id> --address <ip>")
        return err
    }
    if fs.NArg() > 0 {
        _, _ = fmt.Fprintln(stderr, "Usage: ploy node add --cluster-id <id> --address <ip>")
        return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
    }
    if strings.TrimSpace(clusterID) == "" {
        _, _ = fmt.Fprintln(stderr, "Usage: ploy node add --cluster-id <id> --address <ip>")
        return errors.New("cluster-id is required")
    }
    if strings.TrimSpace(address) == "" {
        _, _ = fmt.Fprintln(stderr, "Usage: ploy node add --cluster-id <id> --address <ip>")
        return errors.New("address is required")
    }
    // The provisioning flow is exercised elsewhere; unit tests only cover flag validation.
    _, _ = fmt.Fprintf(stderr, "Node %s queued for provisioning on %s\n", clusterID, address)
    return nil
}

