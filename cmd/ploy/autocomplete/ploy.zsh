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
            commands+=("hydration:Inspect and tune hydration policies")
            commands+=("jobs:Inspect and follow individual jobs")
            commands+=("artifact:Manage IPFS Cluster artifacts")
            commands+=("upload:Upload repository or log bundles via SSH")
            commands+=("report:Download reports or artifacts via SSH")
            commands+=("cluster:Manage local cluster descriptors")
            commands+=("config:Inspect or update cluster configuration")
            commands+=("environment:Materialize integration environments")
            commands+=("manifest:Inspect and validate integration manifests")
            commands+=("knowledge-base:Curate knowledge base fixtures")
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
                    _describe 'mod command' commands && ret=0
                    ;;
                'mods')
                    commands=()
                    commands+=("logs:logs <ticket> - Stream Mods logs via SSE (raw|structured formats, auto-retry)")
                    _describe 'mods command' commands && ret=0
                    ;;
                'hydration')
                    commands=()
                    commands+=("inspect:inspect <ticket> - Show hydration snapshot reuse policy for a Mods ticket")
                    commands+=("tune:tune [--ttl] [--replication-min] [--replication-max] [--share] <ticket> - Update hydration retention and sharing settings")
                    _describe 'hydration command' commands && ret=0
                    ;;
                'jobs')
                    commands=()
                    commands+=("follow:follow <job-id> - Follow job logs via SSE with retry semantics")
                    commands+=("ls:List jobs for a Mods ticket")
                    commands+=("inspect:Show details for a job")
                    commands+=("retry:Request a retry for a failed job")
                    _describe 'jobs command' commands && ret=0
                    ;;
                'artifact')
                    commands=()
                    commands+=("push:Upload an artifact to the configured IPFS Cluster")
                    commands+=("pull:Download an artifact by CID")
                    commands+=("status:Inspect replication state for a CID")
                    commands+=("rm:Unpin an artifact from the cluster")
                    _describe 'artifact command' commands && ret=0
                    ;;
                'cluster')
                    commands=()
                    commands+=("add:Bootstrap the control-plane node or join workers over SSH")
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
                'environment')
                    commands=()
                    commands+=("materialize:Materialize integration environments from manifests")
                    _describe 'environment command' commands && ret=0
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
                'help')
                    commands=()
                    
                    commands+=("mod:Plan and run Mods workflows")
                    commands+=("mods:Observe Mods execution (logs, events)")
                    commands+=("hydration:Inspect and tune hydration policies")
                    commands+=("jobs:Inspect and follow individual jobs")
                    commands+=("artifact:Manage IPFS Cluster artifacts")
                    commands+=("upload:Upload repository or log bundles via SSH")
                    commands+=("report:Download reports or artifacts via SSH")
                    commands+=("cluster:Manage local cluster descriptors")
                    commands+=("config:Inspect or update cluster configuration")
                    commands+=("environment:Materialize integration environments")
                    commands+=("manifest:Inspect and validate integration manifests")
                    commands+=("knowledge-base:Curate knowledge base fixtures")
                    _describe 'help command' commands && ret=0
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
                            commands+=("status:status [--limit <n>] [--cluster-id <id>] - Inspect signer health and recent rotation audit entries")
                            commands+=("rotate:rotate --secret <name> --api-key <token> [--cluster-id <id>] - Rotate a GitLab secret and trigger node refresh")
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
