FROM --platform=$TARGETOS/$TARGETARCH maven:3.9.11-eclipse-temurin-17

ARG BUILD_TS
LABEL org.opencontainers.image.source="https://github.com/iw2rmb/ploy" \
    org.opencontainers.image.title="mod-orw" \
    org.opencontainers.image.description="Ploy Mods: OpenRewrite apply runner (Maven)" \
    org.opencontainers.image.created="$BUILD_TS"

RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    bash git tar gzip jq unzip curl \
    && rm -rf /var/lib/apt/lists/*

# Install a modern Gradle distribution explicitly so the system
# gradle binary is compatible with the OpenRewrite Gradle plugin.
# Align with the Gradle wrapper version used by typical target projects (8.5).
ARG GRADLE_VERSION=8.5
RUN curl -sSL "https://services.gradle.org/distributions/gradle-${GRADLE_VERSION}-bin.zip" -o /tmp/gradle.zip \
    && mkdir -p /opt/gradle \
    && unzip -d /opt/gradle /tmp/gradle.zip \
    && rm /tmp/gradle.zip
ENV GRADLE_HOME=/opt/gradle/gradle-${GRADLE_VERSION}
ENV PATH="${GRADLE_HOME}/bin:${PATH}"

# Workdir is where the step runner mounts the working tree
WORKDIR /workspace

COPY --chmod=755 mod-orw.sh /usr/local/bin/mod-orw
RUN ln -s /usr/local/bin/mod-orw /usr/local/bin/mods-orw

ENTRYPOINT ["mods-orw"]
CMD ["--apply", "--dir", "/workspace", "--out", "/out"]

# The runner invokes: mods-orw --apply --recipe-json /in/recipe.json --dir /workspace --out /out
