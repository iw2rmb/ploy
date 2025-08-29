job "analysis-pylint-batch" {
  datacenters = ["dc1"]
  type = "batch"
  priority = 70

  group "analysis" {
    count = 1
    
    ephemeral_disk {
      size = 512
      migrate = false
      sticky = false
    }

    restart {
      attempts = 2
      delay = "10s"
      interval = "1m"
      mode = "fail"
    }

    task "pylint" {
      driver = "docker"

      config {
        image = "python:3.11-slim"
        volumes = ["local:/workspace"]
        readonly_rootfs = false
        network_mode = "host"
      }

      env {
        PYTHONDONTWRITEBYTECODE = "1"
        PYTHONUNBUFFERED = "1"
      }

      template {
        data = <<EOF
#!/bin/bash
set -e

echo "Starting Pylint analysis job..."

# Install pylint
pip install --no-cache-dir pylint

# Download input archive from SeaweedFS
echo "Downloading input from: ${INPUT_URL}"
wget -q -O /workspace/input.tar.gz "${INPUT_URL}"

# Extract archive
cd /workspace
tar -xzf input.tar.gz
rm input.tar.gz

# Find all Python files
PYTHON_FILES=$(find . -name "*.py" -type f | head -100)

if [ -z "$PYTHON_FILES" ]; then
  echo "No Python files found"
  # Create empty result
  cat > /workspace/output.json <<EOJSON
{
  "language": "python",
  "analyzer": "pylint",
  "success": true,
  "issues": [],
  "metrics": {
    "total_issues": 0,
    "files_analyzed": 0
  }
}
EOJSON
else
  echo "Analyzing $(echo $PYTHON_FILES | wc -w) Python files..."
  
  # Run Pylint
  pylint --output-format=json --reports=no --score=no $PYTHON_FILES > /workspace/pylint.json 2>&1 || true

  # Convert Pylint output to our format
  python3 <<EOPYTHON
import json
import sys

try:
    with open('/workspace/pylint.json', 'r') as f:
        content = f.read().strip()
        if content:
            pylint_output = json.loads(content)
        else:
            pylint_output = []
except Exception as e:
    print(f"Error reading pylint output: {e}")
    pylint_output = []

issues = []
severity_map = {
    'fatal': 'critical',
    'error': 'high',
    'warning': 'medium',
    'convention': 'low',
    'refactor': 'low',
    'info': 'info'
}

category_map = {
    'E': 'bug',           # Error
    'W': 'maintenance',   # Warning
    'C': 'style',         # Convention
    'R': 'complexity',    # Refactor
    'I': 'style',         # Information
    'F': 'bug'            # Fatal
}

for msg in pylint_output:
    severity = severity_map.get(msg.get('type', 'info'), 'info')
    rule = msg.get('message-id', '')
    category = category_map.get(rule[0] if rule else 'I', 'maintenance')
    
    issues.append({
        'id': f"{msg.get('path', '')}:{msg.get('line', 0)}:{msg.get('column', 0)}",
        'file': msg.get('path', ''),
        'line': msg.get('line', 0),
        'column': msg.get('column', 0),
        'severity': severity,
        'category': category,
        'rule_name': rule,
        'message': msg.get('message', ''),
        'arf_compatible': rule in ['unused-import', 'unused-variable', 'missing-docstring', 'trailing-whitespace']
    })

result = {
    'language': 'python',
    'analyzer': 'pylint',
    'success': True,
    'issues': issues,
    'metrics': {
        'total_issues': len(issues),
        'files_analyzed': len(set(i['file'] for i in issues if i['file']))
    }
}

with open('/workspace/output.json', 'w') as f:
    json.dump(result, f, indent=2)
EOPYTHON
fi

# Upload result to SeaweedFS
echo "Uploading result to: ${OUTPUT_URL}"
curl -X PUT "${OUTPUT_URL}" --data-binary @/workspace/output.json

# Update job status in Consul
if [ -n "$CONSUL_HTTP_ADDR" ]; then
  consul kv put "ploy/analysis/jobs/${JOB_ID}/status" "completed"
  consul kv put "ploy/analysis/jobs/${JOB_ID}/completed_at" "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  consul kv put "ploy/analysis/jobs/${JOB_ID}/result" "$(cat /workspace/output.json)"
fi

echo "Analysis completed successfully"
EOF
        destination = "local/run.sh"
        perms = "0755"
      }

      config {
        command = "/bin/bash"
        args = ["local/run.sh"]
      }

      resources {
        cpu = 256
        memory = 256
      }

      kill_signal = "SIGTERM"
      kill_timeout = "30s"
    }
  }

  reschedule {
    attempts = 2
    interval = "1m"
    delay = "10s"
    unlimited = false
  }
}