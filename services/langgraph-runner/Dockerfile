FROM alpine:3.19

RUN apk add --no-cache bash coreutils jq

WORKDIR /workspace

COPY entrypoint.sh /usr/local/bin/langgraph-runner
RUN chmod +x /usr/local/bin/langgraph-runner

ENV OUTPUT_DIR=/workspace/out \
    CONTEXT_DIR=/workspace/context \
    RUN_ID=""

ENTRYPOINT ["/usr/local/bin/langgraph-runner"]

