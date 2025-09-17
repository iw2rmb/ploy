FROM alpine:3.19
# Print canary lines to both stdout and stderr, then trigger a build-time error
RUN echo "KANIKO-LOG-CANARY: stdout" && \
    echo "KANIKO-LOG-CANARY: stderr" 1>&2 && \
    echo "Simulated build steps..."

# Force Kaniko build failure by copying a missing file
COPY definitely-missing /tmp/
