package clitree

// Node models a CLI command tree node.
type Node struct {
	Name        string
	Synopsis    string
	Description string
	Subcommands []Node
}

// Tree returns the static CLI command tree used by shell completion generators.
// This mirrors the currently vendored completion files under cmd/ploy/autocomplete/.
func Tree() []Node {
	return []Node{
		{
			Name:     "mod",
			Synopsis: "Plan and run Mods workflows",
			Subcommands: []Node{
				{Name: "run", Synopsis: "Submit a Mods run to the control plane"},
				{Name: "cancel", Synopsis: "Cancel a Mods ticket via the control plane"},
				{Name: "resume", Synopsis: "Resume a paused Mods ticket"},
				{Name: "inspect", Synopsis: "Show summary for a Mods ticket"},
				{Name: "artifacts", Synopsis: "List ticket artifacts by stage"},
				{Name: "diffs", Synopsis: "List diffs or download newest patch"},
			},
		},
		{
			Name:     "mods",
			Synopsis: "Observe Mods execution (logs, events)",
			Subcommands: []Node{
				{Name: "logs", Synopsis: "logs <ticket>", Description: "Stream Mods logs via SSE (raw|structured formats, auto-retry)"},
			},
		},
		{
			Name:     "runs",
			Synopsis: "Inspect and follow individual runs",
			Subcommands: []Node{
				{Name: "follow", Synopsis: "follow <run-id>", Description: "Follow run logs via SSE with retry semantics"},
				{Name: "inspect", Synopsis: "Show details for a run"},
			},
		},
		{Name: "upload", Synopsis: "Upload artifact bundle to a run (HTTPS)"},
		{
			Name:     "cluster",
			Synopsis: "Manage local cluster descriptors",
			Subcommands: []Node{
				{Name: "add", Synopsis: "Bootstrap the control-plane node or join workers over SSH"},
				{Name: "https", Synopsis: "Set HTTPS endpoints and CA on a descriptor"},
				{Name: "connect", Synopsis: "Cache beacon metadata and trust bundles locally"},
				{Name: "list", Synopsis: "Show locally cached cluster descriptors"},
				{Name: "cert", Synopsis: "Inspect cluster certificate authority state", Subcommands: []Node{
					{Name: "status", Synopsis: "Show the active CA version, expiry, and worker count"},
				}},
			},
		},
		{
			Name:     "config",
			Synopsis: "Inspect or update cluster configuration",
			Subcommands: []Node{
				{Name: "gitlab", Synopsis: "Manage GitLab integration credentials", Subcommands: []Node{
					{Name: "show", Synopsis: "show [--cluster-id <id>]", Description: "Display the current GitLab configuration"},
					{Name: "set", Synopsis: "set --file <path> [--cluster-id <id>]", Description: "Apply a GitLab configuration JSON file"},
					{Name: "validate", Synopsis: "validate --file <path>", Description: "Validate a GitLab configuration without saving"},
				}},
			},
		},
		{Name: "manifest", Synopsis: "Inspect and validate integration manifests", Subcommands: []Node{{Name: "schema", Synopsis: "Print the integration manifest JSON schema"}, {Name: "validate", Synopsis: "Validate manifests and optionally rewrite them to v2"}}},
		{Name: "knowledge-base", Synopsis: "Curate knowledge base fixtures", Subcommands: []Node{{Name: "ingest", Synopsis: "Append incidents to the knowledge base catalog"}, {Name: "evaluate", Synopsis: "Evaluate knowledge base classifier accuracy"}}},
		{Name: "server", Synopsis: "Manage control plane server", Subcommands: []Node{{Name: "deploy", Synopsis: "Deploy and configure a control plane server"}}},
		{Name: "node", Synopsis: "Manage worker nodes", Subcommands: []Node{{Name: "add", Synopsis: "Add a worker node to the cluster"}}},
		{Name: "help", Synopsis: "Show help for commands"},
	}
}
