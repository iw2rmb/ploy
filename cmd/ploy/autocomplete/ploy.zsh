#compdef ploy

_ploy() {
    local state ret=1
    local -a commands
    _arguments -C \
        '1:command:->cmds' \
        '2:subcommand:->subs' \
        '3:subcommand:->third' && ret=0 || ret=$?
    case $state in
        cmds)
            commands=()
            commands+=("mod:Plan and run Mods workflows")
            commands+=("mods:Observe Mods execution (logs, events)")
            commands+=("runs:Inspect and follow individual runs")
            commands+=("upload:Upload artifact bundle to a run (HTTPS)")
            commands+=("cluster:Manage local cluster descriptors")
            commands+=("config:Inspect or update cluster configuration")
            commands+=("manifest:Inspect and validate integration manifests")
            commands+=("knowledge-base:Curate knowledge base fixtures")
            commands+=("server:Manage control plane server")
            commands+=("node:Manage worker nodes")
            commands+=("help:Show help for commands")
            _describe 'command' commands && ret=0
            ;;
        subs)
            case $words[2] in
                'mod')
                    commands=()
                    commands+=("run:Submit a Mods run to the control plane")
                    commands+=("cancel:Cancel a Mods ticket via the control plane")
                    commands+=("resume:Resume a paused Mods ticket")
                    commands+=("inspect:Show summary for a Mods ticket")
                    commands+=("artifacts:List ticket artifacts by stage")
                    commands+=("diffs:List diffs or download newest patch")
                    _describe 'mod command' commands && ret=0
                    ;;
                'mods')
                    commands=()
                    commands+=("logs:logs <ticket> - Stream Mods logs via SSE (raw|structured formats, auto-retry)")
                    _describe 'mods command' commands && ret=0
                    ;;
                'runs')
                    commands=()
                    commands+=("follow:follow <run-id> - Follow run logs via SSE with retry semantics")
                    commands+=("inspect:Show details for a run")
                    _describe 'runs command' commands && ret=0
                    ;;
                'cluster')
                    commands=()
                    commands+=("add:Bootstrap the control-plane node or join workers over SSH")
                    commands+=("https:Set HTTPS endpoints and CA on a descriptor")
                    commands+=("connect:Cache beacon metadata and trust bundles locally")
                    commands+=("list:Show locally cached cluster descriptors")
                    commands+=("cert:Inspect cluster certificate authority state")
                    _describe 'cluster command' commands && ret=0
                    ;;
                'config')
                    commands=()
                    commands+=("gitlab:Manage GitLab integration credentials")
                    _describe 'config command' commands && ret=0
                    ;;
                'manifest')
                    commands=()
                    commands+=("schema:Print the integration manifest JSON schema")
                    commands+=("validate:Validate manifests and optionally rewrite them to v2")
                    _describe 'manifest command' commands && ret=0
                    ;;
                'knowledge-base')
                    commands=()
                    commands+=("ingest:Append incidents to the knowledge base catalog")
                    commands+=("evaluate:Evaluate knowledge base classifier accuracy")
                    _describe 'knowledge-base command' commands && ret=0
                    ;;
                'server')
                    commands=()
                    commands+=("deploy:Deploy and configure a control plane server")
                    _describe 'server command' commands && ret=0
                    ;;
                'node')
                    commands=()
                    commands+=("add:Add a worker node to the cluster")
                    _describe 'node command' commands && ret=0
                    ;;
            esac
            ;;
        third)
            case $words[2] in
                'cluster')
                    case $words[3] in
                        'cert')
                            commands=()
                            commands+=("status:Show the active CA version, expiry, and worker count")
                            _describe 'cluster cert command' commands && ret=0
                            ;;
                    esac
                    ;;
                'config')
                    case $words[3] in
                        'gitlab')
                            commands=()
                            commands+=("show:show [--cluster-id <id>] - Display the current GitLab configuration")
                            commands+=("set:set --file <path> [--cluster-id <id>] - Apply a GitLab configuration JSON file")
                            commands+=("validate:validate --file <path> - Validate a GitLab configuration without saving")
                            _describe 'config gitlab command' commands && ret=0
                            ;;
                    esac
                    ;;
            esac
            ;;
    esac
    return ret
}

_ploy "$@"
