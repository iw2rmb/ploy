# JVM-based OpenRewrite Runner with SeaweedFS Caching
# This image runs OpenRewrite transformations using Maven with dynamic recipe loading
# Recipes are cached in SeaweedFS to avoid repeated downloads from Maven Central

FROM --platform=linux/amd64 eclipse-temurin:17-jdk

# Build metadata to force fresh image digests on rebuilds
ARG ORW_BUILD_TS="dev"
LABEL org.ploy.orw_build_ts="${ORW_BUILD_TS}"

# Install required tools
# Install required tools (Debian base)
RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
       bash curl git maven tar gzip jq unzip ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Ensure JAVA_HOME points to the JDK and add to PATH
ENV JAVA_HOME=/opt/java/openjdk
ENV PATH="$JAVA_HOME/bin:${PATH}"

## Gradle removed: runner is Maven-only

# Create workspace directory
WORKDIR /workspace
RUN mkdir -p /workspace/.m2/repository

# Create the OpenRewrite runner script and workspace setup
COPY runner.sh /usr/local/bin/openrewrite
COPY setup-workspace.sh /usr/local/bin/setup-workspace
RUN chmod 755 /usr/local/bin/openrewrite /usr/local/bin/setup-workspace

## Diff generation helper removed; runner uses git diff directly

# Set up Maven settings for better performance
RUN mkdir -p /root/.m2 && \
    echo '<?xml version="1.0" encoding="UTF-8"?>' > /root/.m2/settings.xml && \
    echo '<settings>' >> /root/.m2/settings.xml && \
    echo '  <localRepository>/workspace/.m2/repository</localRepository>' >> /root/.m2/settings.xml && \
    echo '  <offline>false</offline>' >> /root/.m2/settings.xml && \
    echo '</settings>' >> /root/.m2/settings.xml

# NOTE: Do not pre-download a specific plugin version here.
# The runner selects the Maven plugin version via MAVEN_PLUGIN_VERSION env.
# We only verify toolchain presence.
RUN java -version && javac -version && mvn -version || true

# Use setup script as entrypoint (which then calls openrewrite)
ENTRYPOINT ["/usr/local/bin/setup-workspace"]
