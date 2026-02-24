# orw-maven: Minimal OpenRewrite image for Maven-only workspaces.
#
# This image applies OpenRewrite recipes to Maven projects via the
# rewrite-maven-plugin. It expects pom.xml in /workspace and recipe
# coordinates via environment variables.
#
# See deploy/images/migs/README.md for the Migs runtime contract.

FROM --platform=$TARGETOS/$TARGETARCH maven:3.9.11-eclipse-temurin-17

ARG BUILD_TS
LABEL org.opencontainers.image.source="https://github.com/iw2rmb/ploy" \
    org.opencontainers.image.title="orw-maven" \
    org.opencontainers.image.description="Ploy Migs: OpenRewrite apply runner (Maven-only)" \
    org.opencontainers.image.created="$BUILD_TS"

# Install minimal dependencies for git operations and TLS manipulation.
RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    bash git jq \
    && rm -rf /var/lib/apt/lists/*

# Workdir is where the step runner mounts the working tree.
WORKDIR /workspace

# Install the entry script and create the canonical symlink.
COPY --chmod=755 orw-maven.sh /usr/local/bin/orw-maven
RUN ln -s /usr/local/bin/orw-maven /usr/local/bin/migs-orw-maven

ENTRYPOINT ["migs-orw-maven"]
CMD ["--apply", "--dir", "/workspace", "--out", "/out"]

# The runner invokes: migs-orw-maven --apply --dir /workspace --out /out
