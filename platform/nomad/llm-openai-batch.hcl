job "llm-openai-batch" {
  datacenters = ["dc1"]
  type = "batch"
  priority = 75

  group "llm-transform" {
    count = 1
    
    ephemeral_disk {
      size = 1024
      migrate = false
      sticky = false
    }

    restart {
      attempts = 2
      delay = "30s"
      interval = "5m"
      mode = "fail"
    }

    task "openai-transform" {
      driver = "docker"

      config {
        image = "python:3.11-slim"
        volumes = ["local:/workspace"]
        network_mode = "host"
        readonly_rootfs = false
      }

      env {
        PYTHONDONTWRITEBYTECODE = "1"
        PYTHONUNBUFFERED = "1"
      }

      template {
        data = <<EOF
#!/bin/bash
set -e

echo "Starting OpenAI LLM transformation job..."

# Install required packages
pip install --no-cache-dir openai requests jq

# Download input archive from SeaweedFS
echo "Downloading input from: ${INPUT_URL}"
wget -q -O /workspace/input.tar.gz "${INPUT_URL}"

# Extract archive
cd /workspace
tar -xzf input.tar.gz
rm input.tar.gz

# Create Python script for OpenAI transformation
cat > transform.py <<'PYTHON'
import os
import sys
import base64
import json
import tarfile
import glob
from openai import OpenAI

def main():
    # Initialize OpenAI client
    api_key = os.environ.get('OPENAI_API_KEY')
    if not api_key:
        print("Error: OPENAI_API_KEY not set")
        sys.exit(1)
    
    client = OpenAI(api_key=api_key)
    
    # Get parameters from environment
    model = os.environ.get('MODEL', 'gpt-4')
    prompt_base64 = os.environ.get('PROMPT', '')
    temperature = float(os.environ.get('TEMPERATURE', '0.1'))
    max_tokens = int(os.environ.get('MAX_TOKENS', '2048'))
    language = os.environ.get('LANGUAGE', 'java')
    job_id = os.environ.get('JOB_ID', 'unknown')
    
    # Decode prompt
    try:
        prompt = base64.b64decode(prompt_base64).decode('utf-8')
    except Exception as e:
        print(f"Error decoding prompt: {e}")
        sys.exit(1)
    
    # Map language to file extensions
    ext_map = {
        'java': ['*.java'],
        'python': ['*.py'],
        'javascript': ['*.js', '*.jsx'],
        'typescript': ['*.ts', '*.tsx'],
        'go': ['*.go'],
        'csharp': ['*.cs'],
        'cpp': ['*.cpp', '*.cc', '*.h', '*.hpp'],
        'rust': ['*.rs']
    }
    
    extensions = ext_map.get(language, ['*'])
    
    # Collect code files
    code_files = []
    for ext in extensions:
        for file_path in glob.glob(f'**/{ext}', recursive=True):
            if len(code_files) >= 5:  # Limit number of files
                break
            try:
                with open(file_path, 'r', encoding='utf-8') as f:
                    content = f.read(2000)  # Limit content per file
                    code_files.append({
                        'path': file_path,
                        'content': content
                    })
            except Exception as e:
                print(f"Warning: Could not read {file_path}: {e}")
    
    # Build code context
    code_context = ""
    for file_info in code_files:
        code_context += f"\nFile: {file_info['path']}\n"
        code_context += f"```{language}\n{file_info['content']}\n```\n"
    
    # Create messages for OpenAI
    messages = [
        {
            "role": "system",
            "content": "You are an expert code transformation assistant. Provide clear, working code transformations based on the requirements."
        },
        {
            "role": "user",
            "content": f"{prompt}\n\nHere is the code to transform:\n{code_context}"
        }
    ]
    
    print(f"Calling OpenAI API with model {model}...")
    
    try:
        # Call OpenAI API
        response = client.chat.completions.create(
            model=model,
            messages=messages,
            temperature=temperature,
            max_tokens=max_tokens,
            top_p=0.9,
            frequency_penalty=0.0,
            presence_penalty=0.0
        )
        
        # Extract transformed code
        transformed_content = response.choices[0].message.content
        
        # Save transformed code
        with open('/workspace/transformed.txt', 'w') as f:
            f.write(transformed_content)
        
        # Create metadata
        metadata = {
            'job_id': job_id,
            'model': model,
            'provider': 'openai',
            'language': language,
            'temperature': temperature,
            'max_tokens': max_tokens,
            'usage': {
                'prompt_tokens': response.usage.prompt_tokens,
                'completion_tokens': response.usage.completion_tokens,
                'total_tokens': response.usage.total_tokens
            },
            'finish_reason': response.choices[0].finish_reason,
            'timestamp': response.created
        }
        
        with open('/workspace/metadata.json', 'w') as f:
            json.dump(metadata, f, indent=2)
        
        print("Transformation completed successfully")
        
    except Exception as e:
        print(f"Error calling OpenAI API: {e}")
        sys.exit(1)

if __name__ == "__main__":
    main()
PYTHON

# Run the transformation
python transform.py

# Package result
tar -czf output.tar.gz transformed.txt metadata.json

# Upload result to SeaweedFS
echo "Uploading result to: ${OUTPUT_URL}"
curl -X PUT "${OUTPUT_URL}" --data-binary @output.tar.gz

# Update job status in Consul
if [ -n "$CONSUL_HTTP_ADDR" ]; then
  consul kv put "ploy/llm/jobs/${JOB_ID}/status" "completed"
  consul kv put "ploy/llm/jobs/${JOB_ID}/completed_at" "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  
  # Store metadata in Consul
  consul kv put "ploy/llm/jobs/${JOB_ID}/result" "$(cat metadata.json)"
fi

echo "OpenAI LLM transformation completed successfully"
EOF
        destination = "local/run.sh"
        perms = "0755"
      }

      config {
        command = "/bin/bash"
        args = ["local/run.sh"]
      }

      resources {
        cpu = 500
        memory = 1024
      }

      kill_signal = "SIGTERM"
      kill_timeout = "30s"
    }
  }

  reschedule {
    attempts = 2
    interval = "5m"
    delay = "30s"
    unlimited = false
  }
}