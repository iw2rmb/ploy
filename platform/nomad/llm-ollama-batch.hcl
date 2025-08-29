job "llm-ollama-batch" {
  datacenters = ["dc1"]
  type = "batch"
  priority = 75

  group "llm-transform" {
    count = 1
    
    ephemeral_disk {
      size = 2048
      migrate = false
      sticky = false
    }

    restart {
      attempts = 2
      delay = "30s"
      interval = "5m"
      mode = "fail"
    }

    task "ollama-transform" {
      driver = "docker"

      config {
        image = "ollama/ollama:latest"
        volumes = ["local:/workspace"]
        network_mode = "host"
        privileged = false
      }

      env {
        OLLAMA_HOST = "0.0.0.0:11434"
        OLLAMA_MODELS = "/workspace/models"
        OLLAMA_NUM_PARALLEL = "1"
        OLLAMA_MAX_LOADED_MODELS = "1"
        OLLAMA_KEEP_ALIVE = "5m"
      }

      template {
        data = <<EOF
#!/bin/bash
set -e

echo "Starting Ollama LLM transformation job..."

# Start Ollama server in background
ollama serve &
OLLAMA_PID=$!

# Wait for Ollama to be ready
echo "Waiting for Ollama server to start..."
for i in {1..30}; do
  if curl -s http://localhost:11434/api/tags >/dev/null 2>&1; then
    echo "Ollama server is ready"
    break
  fi
  sleep 2
done

# Pull the model if not already available
echo "Ensuring model ${MODEL} is available..."
ollama pull ${MODEL} || true

# Download input archive from SeaweedFS
echo "Downloading input from: ${INPUT_URL}"
wget -q -O /workspace/input.tar.gz "${INPUT_URL}"

# Extract archive
cd /workspace
tar -xzf input.tar.gz
rm input.tar.gz

# Decode the base64 encoded prompt
DECODED_PROMPT=$(echo "${PROMPT}" | base64 -d)

# Find code files based on language
case "${LANGUAGE}" in
  java)
    EXT="java"
    ;;
  python)
    EXT="py"
    ;;
  javascript|typescript)
    EXT="js ts jsx tsx"
    ;;
  go)
    EXT="go"
    ;;
  *)
    EXT="*"
    ;;
esac

# Collect code context (limited to prevent token overflow)
CODE_CONTEXT=""
FILE_COUNT=0
for ext in $EXT; do
  for file in $(find . -name "*.$ext" -type f | head -10); do
    if [ $FILE_COUNT -lt 5 ]; then
      echo "Processing file: $file"
      FILE_CONTENT=$(head -c 2000 "$file")
      CODE_CONTEXT="${CODE_CONTEXT}
File: $file
\`\`\`${LANGUAGE}
${FILE_CONTENT}
\`\`\`
"
      FILE_COUNT=$((FILE_COUNT + 1))
    fi
  done
done

# Create the full prompt
FULL_PROMPT="${DECODED_PROMPT}

Here is the code to transform:
${CODE_CONTEXT}"

# Call Ollama API for transformation
echo "Calling Ollama API with model ${MODEL}..."
RESPONSE=$(curl -s -X POST http://localhost:11434/api/generate \
  -H "Content-Type: application/json" \
  -d @- <<JSON
{
  "model": "${MODEL}",
  "prompt": "${FULL_PROMPT}",
  "stream": false,
  "options": {
    "temperature": ${TEMPERATURE},
    "num_predict": ${MAX_TOKENS},
    "top_k": 40,
    "top_p": 0.9,
    "repeat_penalty": 1.1
  }
}
JSON
)

# Extract the response text
echo "$RESPONSE" | jq -r '.response' > /workspace/transformed.txt

# Create metadata file
cat > /workspace/metadata.json <<META
{
  "job_id": "${JOB_ID}",
  "model": "${MODEL}",
  "provider": "ollama",
  "language": "${LANGUAGE}",
  "temperature": ${TEMPERATURE},
  "max_tokens": ${MAX_TOKENS},
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
META

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

# Cleanup Ollama server
kill $OLLAMA_PID 2>/dev/null || true

echo "LLM transformation completed successfully"
EOF
        destination = "local/run.sh"
        perms = "0755"
      }

      config {
        command = "/bin/bash"
        args = ["local/run.sh"]
      }

      resources {
        cpu = 1000
        memory = 2048
      }

      kill_signal = "SIGTERM"
      kill_timeout = "60s"
    }
  }

  reschedule {
    attempts = 2
    interval = "5m"
    delay = "30s"
    unlimited = false
  }
}