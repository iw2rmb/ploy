package clitree

import "strings"

// Node describes a CLI command and its nested subcommands.
type Node struct {
	Name        string
	Synopsis    string
	Description string
	Usage       string
	Note        string
	Hidden      bool
	Subcommands []Node
}

var commandTree = []Node{
	{
		Name:        "workflow",
		Description: "Manage workflow runs and cancellations",
		Usage:       "ploy workflow <command>",
		Note:        "Use 'ploy help workflow <command>' for command-specific details.",
		Subcommands: []Node{
			{
				Name:        "cancel",
				Description: "Cancel an in-flight workflow run",
				Usage:       "ploy workflow cancel --tenant <tenant> --run-id <run-id> [--workflow <workflow-id>] [--reason <text>]",
			},
		},
	},
	{
		Name:        "mod",
		Description: "Plan and run Mods workflows",
		Usage:       "ploy mod <command>",
		Note:        "Use 'ploy mod run --help' for flag details.",
		Subcommands: []Node{
			{
				Name:        "run",
				Description: "Submit a Mods run to the control plane",
				Usage:       "ploy mod run [--flags]",
			},
		},
	},
	{
		Name:        "mods",
		Description: "Observe Mods execution (logs, events)",
		Usage:       "ploy mods <command>",
		Note:        "Use 'ploy mods logs --help' for flag details.",
		Subcommands: []Node{
			{
				Name:        "logs",
				Synopsis:    "logs <ticket>",
				Description: "Stream Mods logs via SSE (raw|structured formats, auto-retry)",
				Usage:       "ploy mods logs [--format] [--max-retries] [--retry-wait] <ticket>",
			},
		},
	},
	{
		Name:        "jobs",
		Description: "Inspect and follow individual jobs",
		Usage:       "ploy jobs <command>",
		Note:        "Use 'ploy jobs follow --help' for flag details.",
		Subcommands: []Node{
			{
				Name:        "follow",
				Synopsis:    "follow <job-id>",
				Description: "Follow job logs via SSE with retry semantics",
				Usage:       "ploy jobs follow [--format] [--max-retries] [--retry-wait] <job-id>",
			},
		},
	},
	{
		Name:        "artifact",
		Description: "Manage IPFS Cluster artifacts",
		Usage:       "ploy artifact <command>",
		Note:        "Use 'ploy artifact <command> --help' for flag details.",
		Subcommands: []Node{
			{
				Name:        "push",
				Description: "Upload an artifact to the configured IPFS Cluster",
				Usage:       "ploy artifact push [--name <name>] [--kind <kind>] [--replication-min <n>] [--replication-max <n>] <path>",
			},
			{
				Name:        "pull",
				Description: "Download an artifact by CID",
				Usage:       "ploy artifact pull [--output <path>] <cid>",
			},
			{
				Name:        "status",
				Description: "Inspect replication state for a CID",
				Usage:       "ploy artifact status <cid>",
			},
			{
				Name:        "rm",
				Description: "Unpin an artifact from the cluster",
				Usage:       "ploy artifact rm <cid>",
			},
		},
	},
	{
		Name:        "cluster",
		Description: "Manage local cluster descriptors",
		Usage:       "ploy cluster <command>",
		Subcommands: []Node{
			{
				Name:        "add",
				Description: "Bootstrap the control-plane node or join workers over SSH",
				Usage:       "ploy cluster add --address <host> [--cluster-id <id>] [--identity <path>]",
			},
			{
				Name:        "connect",
				Description: "Cache beacon metadata and trust bundles locally",
				Usage:       "ploy cluster connect --beacon-ip <addr> --api-key <key>",
			},
			{
				Name:        "list",
				Description: "Show locally cached cluster descriptors",
				Usage:       "ploy cluster list",
			},
			{
				Name:        "cert",
				Description: "Inspect cluster certificate authority state",
				Usage:       "ploy cluster cert <command>",
				Subcommands: []Node{
					{
						Name:        "status",
						Description: "Show the active CA version, expiry, and worker count",
						Usage:       "ploy cluster cert status [--cluster-id <id>]",
					},
				},
			},
		},
	},
	{
		Name:        "config",
		Description: "Inspect or update cluster configuration",
		Usage:       "ploy config <command>",
		Note:        "Use 'ploy config gitlab <command> --help' for flag details.",
		Subcommands: []Node{
			{
				Name:        "gitlab",
				Description: "Manage GitLab integration credentials",
				Usage:       "ploy config gitlab <command>",
				Subcommands: []Node{
					{
						Name:        "show",
						Description: "Display the current GitLab configuration",
						Usage:       "ploy config gitlab show",
					},
					{
						Name:        "set",
						Synopsis:    "set --file <path>",
						Description: "Apply a GitLab configuration JSON file",
						Usage:       "ploy config gitlab set --file <path>",
					},
					{
						Name:        "validate",
						Synopsis:    "validate --file <path>",
						Description: "Validate a GitLab configuration without saving",
						Usage:       "ploy config gitlab validate --file <path>",
					},
					{
						Name:        "status",
						Description: "Inspect signer health and recent rotation audit entries",
						Usage:       "ploy config gitlab status [--limit <n>]",
					},
					{
						Name:        "rotate",
						Synopsis:    "rotate --secret <name>",
						Description: "Rotate a GitLab secret and trigger node refresh",
						Usage:       "ploy config gitlab rotate --secret <name> [--scopes <scope,...>]",
					},
				},
			},
		},
	},
	{
		Name:        "snapshot",
		Description: "Plan and capture workspace snapshots",
		Usage:       "ploy snapshot <command>",
		Note:        "Use 'ploy snapshot <command> --help' for flag details.",
		Subcommands: []Node{
			{
				Name:        "plan",
				Description: "Preview strip/mask/synthetic rules for a snapshot",
				Usage:       "ploy snapshot plan --snapshot <snapshot-name>",
			},
			{
				Name:        "capture",
				Description: "Execute snapshot capture and publish metadata",
				Usage:       "ploy snapshot capture --snapshot <snapshot-name> --tenant <tenant> --ticket <ticket-id>",
			},
		},
	},
	{
		Name:        "environment",
		Description: "Materialize integration environments",
		Usage:       "ploy environment <command>",
		Note:        "Use 'ploy environment materialize --help' for flag details.",
		Subcommands: []Node{
			{
				Name:        "materialize",
				Description: "Materialize integration environments from manifests and snapshots",
				Usage:       "ploy environment materialize <commit-sha> --app <app> --tenant <tenant> [--flags]",
			},
		},
	},
	{
		Name:        "manifest",
		Description: "Inspect and validate integration manifests",
		Usage:       "ploy manifest <command>",
		Note:        "Use 'ploy manifest <command> --help' for flag details.",
		Subcommands: []Node{
			{
				Name:        "schema",
				Description: "Print the integration manifest JSON schema",
				Usage:       "ploy manifest schema",
			},
			{
				Name:        "validate",
				Description: "Validate manifests and optionally rewrite them to v2",
				Usage:       "ploy manifest validate [--rewrite=v2] <path> [<path>...]",
			},
		},
	},
	{
		Name:        "knowledge-base",
		Description: "Curate knowledge base fixtures",
		Usage:       "ploy knowledge-base <command>",
		Note:        "Use 'ploy knowledge-base <command> --help' for flag details.",
		Subcommands: []Node{
			{
				Name:        "ingest",
				Description: "Append incidents to the knowledge base catalog",
				Usage:       "ploy knowledge-base ingest --from <fixture.json>",
			},
			{
				Name:        "evaluate",
				Description: "Evaluate knowledge base classifier accuracy",
				Usage:       "ploy knowledge-base evaluate --fixture <samples.json>",
			},
		},
	},
}

// Tree returns a deep copy of the CLI command tree including the synthetic help node.
func Tree() []Node {
	nodes := cloneNodes(commandTree)
	helpNode := Node{
		Name:        "help",
		Description: "Show help for commands",
		Usage:       "ploy help <command>",
		Note:        "Use 'ploy help <command>' to view usage for a specific command.",
		Hidden:      true,
	}
	for _, node := range nodes {
		if node.Hidden {
			continue
		}
		helpNode.Subcommands = append(helpNode.Subcommands, Node{
			Name:        node.Name,
			Description: node.Description,
		})
	}
	nodes = append(nodes, helpNode)
	return nodes
}

// Lookup resolves the requested command path against the CLI tree.
func Lookup(path ...string) (Node, bool) {
	if len(path) == 0 {
		return Node{}, false
	}
	nodes := Tree()
	var current Node
	var found bool
	for _, segment := range path {
		found = false
		for _, candidate := range nodes {
			if candidate.Name == segment {
				current = candidate
				nodes = candidate.Subcommands
				found = true
				break
			}
		}
		if !found {
			return Node{}, false
		}
	}
	if len(current.Subcommands) == 0 {
		return current, true
	}
	current.Subcommands = cloneNodes(current.Subcommands)
	return current, true
}

// LegacyCommands returns known legacy Grid command aliases and their replacement guidance.
func LegacyCommands() map[string]string {
	return map[string]string{
		"apps":     "Apps commands moved under 'ploy manifest'. See docs/design/cli-command-tree/README.md for the refreshed layout.",
		"env":      "Environment commands moved to 'ploy environment'. See docs/design/cli-command-tree/README.md for details.",
		"grid":     "Grid commands were removed from the workstation CLI. See docs/design/cli-command-tree/README.md for the refreshed layout.",
		"gridctl":  "Grid commands were removed from the workstation CLI. See docs/design/cli-command-tree/README.md for the refreshed layout.",
		"lanes":    "Lane inspection now lives under 'ploy manifest' and 'ploy mod'. See docs/design/cli-command-tree/README.md for details.",
		"security": "Security commands moved to 'ploy config gitlab'. See docs/design/cli-command-tree/README.md for details.",
	}
}

// cloneNodes returns a deep copy of the supplied node slice.
func cloneNodes(nodes []Node) []Node {
	cloned := make([]Node, len(nodes))
	for i, node := range nodes {
		cloned[i] = node
		if len(node.Subcommands) > 0 {
			cloned[i].Subcommands = cloneNodes(node.Subcommands)
		}
	}
	return cloned
}

// PathString renders a command path for diagnostics.
func PathString(path ...string) string {
	return strings.Join(path, " ")
}
