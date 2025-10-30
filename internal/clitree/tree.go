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
				{
					Name:        "cancel",
					Description: "Cancel a Mods ticket via the control plane",
					Usage:       "ploy mod cancel --ticket <ticket> [--reason <text>]",
				},
				{
					Name:        "resume",
					Description: "Resume a paused Mods ticket",
					Usage:       "ploy mod resume <ticket>",
				},
				{
					Name:        "inspect",
					Description: "Show summary for a Mods ticket",
					Usage:       "ploy mod inspect <ticket>",
				},
				{
					Name:        "artifacts",
					Description: "List ticket artifacts by stage",
					Usage:       "ploy mod artifacts <ticket>",
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
				{
					Name:        "ls",
					Description: "List jobs for a Mods ticket",
					Usage:       "ploy jobs ls --ticket <ticket>",
				},
				{
					Name:        "inspect",
					Description: "Show details for a job",
					Usage:       "ploy jobs inspect --ticket <ticket> <job-id>",
				},
				{
					Name:        "retry",
					Description: "Request a retry for a failed job",
					Usage:       "ploy jobs retry --ticket <ticket> <job-id>",
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
		Name:        "registry",
		Description: "Push/pull/delete OCI blobs and manifests",
		Usage:       "ploy registry <command>",
		Subcommands: []Node{
			{Name: "push-blob", Description: "Upload an OCI blob via SSH slots", Usage: "ploy registry push-blob --repo <name> [--media-type <type>] <path>"},
			{Name: "get-blob", Description: "Download an OCI blob by digest", Usage: "ploy registry get-blob --repo <name> --digest <sha256:...> --output <path>"},
			{Name: "rm-blob", Description: "Delete an OCI blob by digest", Usage: "ploy registry rm-blob --repo <name> --digest <sha256:...>"},
			{Name: "put-manifest", Description: "Store an OCI manifest at a tag or digest", Usage: "ploy registry put-manifest --repo <name> --reference <ref> <manifest.json>"},
			{Name: "get-manifest", Description: "Fetch an OCI manifest", Usage: "ploy registry get-manifest --repo <name> --reference <ref> --output <path>"},
			{Name: "rm-manifest", Description: "Delete an OCI manifest or untag", Usage: "ploy registry rm-manifest --repo <name> --reference <ref>"},
			{Name: "tags", Description: "List tags for a repository", Usage: "ploy registry tags --repo <name>"},
		},
	},
	{
		Name:        "upload",
		Description: "Upload repository or log bundles via SSH",
		Usage:       "ploy upload --job-id <id> <path>",
	},
	{
		Name:        "report",
		Description: "Download reports or artifacts via SSH",
		Usage:       "ploy report --job-id <id> --output <path>",
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
						Synopsis:    "show [--cluster-id <id>]",
						Usage:       "ploy config gitlab show [--cluster-id <id>]",
					},
					{
						Name:        "set",
						Synopsis:    "set --file <path> [--cluster-id <id>]",
						Description: "Apply a GitLab configuration JSON file",
						Usage:       "ploy config gitlab set --file <path> [--cluster-id <id>]",
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
						Synopsis:    "status [--limit <n>] [--cluster-id <id>]",
						Usage:       "ploy config gitlab status [--limit <n>] [--cluster-id <id>]",
					},
					{
						Name:        "rotate",
						Synopsis:    "rotate --secret <name> --api-key <token> [--cluster-id <id>]",
						Description: "Rotate a GitLab secret and trigger node refresh",
						Usage:       "ploy config gitlab rotate --secret <name> [--scopes <scope,...>] [--cluster-id <id>]",
					},
				},
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
                Description: "Materialize integration environments from manifests",
                Usage:       "ploy environment materialize <commit-sha> --app <app> [--flags]",
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

// LegacyCommands removed; legacy aliases are no longer routed.

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
