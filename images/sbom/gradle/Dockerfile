FROM gradle:8.8-jdk17

LABEL org.opencontainers.image.source="https://github.com/iw2rmb/ploy" \
    org.opencontainers.image.title="sbom-gradle" \
    org.opencontainers.image.description="Ploy SBOM runner: Gradle lane with deterministic Gradle/JDK toolchain" \
    org.opencontainers.image.licenses="MIT"

USER root
RUN mkdir -p /workspace /in /out
COPY --chmod=755 images/install_ploy_ca_bundle.sh /usr/local/lib/ploy/install_ploy_ca_bundle.sh
COPY images/gates/gradle/gradle.properties /root/.gradle/gradle.properties
COPY images/gates/gradle/cache.init.gradle /root/.gradle/init.d/cache.init.gradle

WORKDIR /workspace
ENV WORKSPACE=/workspace \
    INDIR=/in \
    OUTDIR=/out

CMD ["bash"]
