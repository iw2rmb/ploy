# JVM-based OpenRewrite Runner with SeaweedFS Caching
# This image runs OpenRewrite transformations using Maven/Gradle with dynamic recipe loading
# Recipes are cached in SeaweedFS to avoid repeated downloads from Maven Central

FROM --platform=linux/amd64 eclipse-temurin:17-jre-alpine

# Install required tools
RUN apk add --no-cache \
    bash \
    curl \
    git \
    maven \
    tar \
    gzip \
    jq

# Install Gradle
ENV GRADLE_VERSION=8.5
RUN curl -L https://services.gradle.org/distributions/gradle-${GRADLE_VERSION}-bin.zip -o gradle.zip && \
    unzip gradle.zip && \
    mv gradle-${GRADLE_VERSION} /opt/gradle && \
    rm gradle.zip
ENV PATH="/opt/gradle/bin:${PATH}"

# Create workspace directory
WORKDIR /workspace
RUN mkdir -p /workspace/.m2/repository

# Create the OpenRewrite runner script and workspace setup
COPY runner.sh /usr/local/bin/openrewrite
COPY setup-workspace.sh /usr/local/bin/setup-workspace
RUN chmod 755 /usr/local/bin/openrewrite /usr/local/bin/setup-workspace

# Set up Maven settings for better performance
RUN mkdir -p /root/.m2 && \
    echo '<?xml version="1.0" encoding="UTF-8"?>' > /root/.m2/settings.xml && \
    echo '<settings>' >> /root/.m2/settings.xml && \
    echo '  <localRepository>/workspace/.m2/repository</localRepository>' >> /root/.m2/settings.xml && \
    echo '  <offline>false</offline>' >> /root/.m2/settings.xml && \
    echo '</settings>' >> /root/.m2/settings.xml

# Pre-download the OpenRewrite Maven plugin to speed up first run
RUN mvn org.openrewrite.maven:rewrite-maven-plugin:6.17.0:help \
    -Ddetail=true -DskipTests 2>/dev/null || true

# Use setup script as entrypoint (which then calls openrewrite)
ENTRYPOINT ["/usr/local/bin/setup-workspace"]