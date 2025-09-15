FROM debian:bookworm-slim

# Tools needed: bash/jq for glue; coreutils; git for --no-index diffs; curl for uploads/events
RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
         bash coreutils jq git curl ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /workspace

COPY entrypoint.sh /usr/local/bin/langgraph-runner
RUN chmod +x /usr/local/bin/langgraph-runner

ENV OUTPUT_DIR=/workspace/out \
    CONTEXT_DIR=/workspace/context \
    RUN_ID=""

ENTRYPOINT ["/usr/local/bin/langgraph-runner"]
