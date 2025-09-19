package analysis

import (
	"fmt"
	"text/template"
)

func (d *AnalysisDispatcher) loadJobTemplates() error {
	templates := map[string]string{
		"pylint":        pylintTemplate,
		"eslint":        eslintTemplate,
		"golangci-lint": golangciTemplate,
	}

	for name, content := range templates {
		tmpl, err := template.New(name).Parse(content)
		if err != nil {
			return fmt.Errorf("failed to parse %s template: %w", name, err)
		}
		d.jobTemplates[name] = tmpl
	}

	return nil
}

const pylintTemplate = `
job "analysis-pylint-{{.JobID}}" {
  datacenters = ["dc1"]
  type = "batch"
  priority = 70
  
  group "analyze" {
    count = 1
    
    ephemeral_disk {
      size = 512
    }
    
    task "pylint" {
      driver = "docker"
      
      config {
        image = "registry.dev.ployman.app/analysis-pylint:latest"
        volumes = ["local:/workspace"]
        readonly_rootfs = true
      }
      
      env {
        JOB_ID = "{{.JobID}}"
        ANALYZER = "pylint"
        INPUT_URL = "{{.InputURL}}"
        OUTPUT_URL = "{{.OutputURL}}"
        CONSUL_HTTP_ADDR = "{{.ConsulAddr}}"
        CONFIG = "{{.ConfigJSON}}"
      }
      
      template {
        data = <<EOF
#!/bin/sh
set -e

# Download input code archive
wget -q -O /workspace/input.tar.gz "$INPUT_URL"

# Extract code
cd /workspace
tar -xzf input.tar.gz

# Run Pylint analysis
pylint --output-format=json --reports=no --score=no \
  $(find . -name "*.py" -type f) > /workspace/analysis.json 2>&1 || true

# Convert to our format
python3 -c "
import json
import sys

try:
    with open('/workspace/analysis.json', 'r') as f:
        pylint_output = json.load(f)
except:
    pylint_output = []

issues = []
for msg in pylint_output:
    issues.append({
        'file': msg.get('path', ''),
        'line': msg.get('line', 0),
        'column': msg.get('column', 0),
        'severity': msg.get('type', 'info'),
        'rule': msg.get('message-id', ''),
        'message': msg.get('message', ''),
        'category': msg.get('category', 'general')
    })

result = {
    'language': 'python',
    'analyzer': 'pylint',
    'success': True,
    'issues': issues,
    'metrics': {
        'total_issues': len(issues)
    }
}

with open('/workspace/output.json', 'w') as f:
    json.dump(result, f)
"

# Upload output
curl -X PUT "$OUTPUT_URL" --data-binary @/workspace/output.json

# Update job status in Consul
consul kv put "ploy/analysis/jobs/$JOB_ID/status" "completed"
consul kv put "ploy/analysis/jobs/$JOB_ID/completed_at" "$(date -u +%Y-%m-%dT%H:%M:%SZ)"

# Store result in Consul
consul kv put "ploy/analysis/jobs/$JOB_ID/result" "$(cat /workspace/output.json)"
EOF
        destination = "local/run.sh"
        perms = "0755"
      }
      
      config {
        command = "/bin/sh"
        args = ["local/run.sh"]
      }
      
      resources {
        cpu = 256
        memory = 256
      }
      
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
`

const eslintTemplate = `
job "analysis-eslint-{{.JobID}}" {
  datacenters = ["dc1"]
  type = "batch"
  priority = 70
  
  group "analyze" {
    count = 1
    
    ephemeral_disk {
      size = 512
    }
    
    task "eslint" {
      driver = "docker"
      
      config {
        image = "registry.dev.ployman.app/analysis-eslint:latest"
        volumes = ["local:/workspace"]
        readonly_rootfs = true
      }
      
      env {
        JOB_ID = "{{.JobID}}"
        ANALYZER = "eslint"
        INPUT_URL = "{{.InputURL}}"
        OUTPUT_URL = "{{.OutputURL}}"
        CONSUL_HTTP_ADDR = "{{.ConsulAddr}}"
        CONFIG = "{{.ConfigJSON}}"
      }
      
      template {
        data = <<EOF
#!/bin/sh
set -e

# Download and extract
wget -q -O /workspace/input.tar.gz "$INPUT_URL"
cd /workspace
tar -xzf input.tar.gz

# Run ESLint
npx eslint --format json --ext .js,.jsx,.ts,.tsx . > /workspace/analysis.json 2>&1 || true

# Convert to our format and upload
node -e "
const fs = require('fs');
const analysis = JSON.parse(fs.readFileSync('/workspace/analysis.json', 'utf8'));

const issues = [];
for (const file of analysis) {
  for (const msg of file.messages) {
    issues.push({
      file: file.filePath,
      line: msg.line || 0,
      column: msg.column || 0,
      severity: msg.severity === 2 ? 'error' : 'warning',
      rule: msg.ruleId || '',
      message: msg.message
    });
  }
}

const result = {
  language: 'javascript',
  analyzer: 'eslint',
  success: true,
  issues: issues,
  metrics: { total_issues: issues.length }
};

fs.writeFileSync('/workspace/output.json', JSON.stringify(result));
"

# Upload and update status
curl -X PUT "$OUTPUT_URL" --data-binary @/workspace/output.json
consul kv put "ploy/analysis/jobs/$JOB_ID/status" "completed"
consul kv put "ploy/analysis/jobs/$JOB_ID/result" "$(cat /workspace/output.json)"
EOF
        destination = "local/run.sh"
        perms = "0755"
      }
      
      config {
        command = "/bin/sh"
        args = ["local/run.sh"]
      }
      
      resources {
        cpu = 256
        memory = 512
      }
    }
  }
}
`

const golangciTemplate = `
job "analysis-golangci-{{.JobID}}" {
  datacenters = ["dc1"]
  type = "batch"
  priority = 70
  
  group "analyze" {
    count = 1
    
    ephemeral_disk {
      size = 1024
    }
    
    task "golangci" {
      driver = "docker"
      
      config {
        image = "golangci/golangci-lint:latest"
        volumes = ["local:/workspace"]
        readonly_rootfs = false
      }
      
      env {
        JOB_ID = "{{.JobID}}"
        ANALYZER = "golangci-lint"
        INPUT_URL = "{{.InputURL}}"
        OUTPUT_URL = "{{.OutputURL}}"
        CONSUL_HTTP_ADDR = "{{.ConsulAddr}}"
      }
      
      template {
        data = <<EOF
#!/bin/sh
set -e

# Download and extract
wget -q -O /workspace/input.tar.gz "$INPUT_URL"
cd /workspace
tar -xzf input.tar.gz

# Run GolangCI-Lint
golangci-lint run --out-format json > /workspace/analysis.json 2>&1 || true

# Process and upload results
curl -X PUT "$OUTPUT_URL" --data-binary @/workspace/analysis.json

# Update status
consul kv put "ploy/analysis/jobs/$JOB_ID/status" "completed"
EOF
        destination = "local/run.sh"
        perms = "0755"
      }
      
      config {
        command = "/bin/sh"
        args = ["local/run.sh"]
      }
      
      resources {
        cpu = 500
        memory = 1024
      }
    }
  }
}
`
