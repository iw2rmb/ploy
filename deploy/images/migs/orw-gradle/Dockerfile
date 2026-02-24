# orw-gradle: Minimal OpenRewrite image for Gradle-only workspaces.
#
# This image applies OpenRewrite recipes to Gradle projects via the
# OpenRewrite Gradle plugin. It expects build.gradle or build.gradle.kts
# in /workspace and recipe coordinates via environment variables.
#
# See deploy/images/migs/README.md for the Migs runtime contract.

FROM --platform=$TARGETOS/$TARGETARCH eclipse-temurin:17-jdk

ARG BUILD_TS
LABEL org.opencontainers.image.source="https://github.com/iw2rmb/ploy" \
    org.opencontainers.image.title="orw-gradle" \
    org.opencontainers.image.description="Ploy Migs: OpenRewrite apply runner (Gradle-only)" \
    org.opencontainers.image.created="$BUILD_TS"

# Install minimal dependencies for git operations.
RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    bash git jq unzip curl \
    && rm -rf /var/lib/apt/lists/*

# Install a modern Gradle distribution so we have a system gradle binary
# compatible with the OpenRewrite Gradle plugin. Version 8.5 aligns with
# typical project Gradle wrapper versions.
ARG GRADLE_VERSION=8.5
RUN curl -sSL "https://services.gradle.org/distributions/gradle-${GRADLE_VERSION}-bin.zip" -o /tmp/gradle.zip \
    && mkdir -p /opt/gradle \
    && unzip -d /opt/gradle /tmp/gradle.zip \
    && rm /tmp/gradle.zip
ENV GRADLE_HOME=/opt/gradle/gradle-${GRADLE_VERSION}
ENV PATH="${GRADLE_HOME}/bin:${PATH}"

# Workdir is where the step runner mounts the working tree.
WORKDIR /workspace

# Install the entry script and create the canonical symlink.
COPY --chmod=755 orw-gradle.sh /usr/local/bin/orw-gradle
RUN ln -s /usr/local/bin/orw-gradle /usr/local/bin/migs-orw-gradle

ENTRYPOINT ["migs-orw-gradle"]
CMD ["--apply", "--dir", "/workspace", "--out", "/out"]

# The runner invokes: migs-orw-gradle --apply --dir /workspace --out /out
