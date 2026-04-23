FROM gradle:8.8-jdk17

LABEL org.opencontainers.image.source="https://github.com/iw2rmb/ploy" \
    org.opencontainers.image.title="sbom-gradle" \
    org.opencontainers.image.description="Ploy SBOM runner: Gradle lane with deterministic Gradle/JDK toolchain" \
    org.opencontainers.image.licenses="MIT"

USER root
RUN rm -rf /root/.gradle \
    && mkdir -p /root/.gradle /workspace /in /out

WORKDIR /workspace
ENV WORKSPACE=/workspace \
    INDIR=/in \
    OUTDIR=/out \
    LANG=C.UTF-8 \
    LC_ALL=C.UTF-8 \
    GRADLE_USER_HOME=/root/.gradle

COPY --chmod=755 images/install_ploy_ca_bundle.sh /usr/local/lib/ploy/install_ploy_ca_bundle.sh
RUN --mount=type=secret,id=ploy_ca_certs,required=false \
    bash -euxo pipefail -c 'if [[ -s /run/secrets/ploy_ca_certs ]]; then mkdir -p /etc/ploy/certs; cp /run/secrets/ploy_ca_certs /etc/ploy/certs/ca.crt; source /usr/local/lib/ploy/install_ploy_ca_bundle.sh; PLOY_CA_IMPORT_JAVA=1 install_ploy_ca_bundle; fi'
COPY --chmod=755 images/sbom/shared/collect-java-classpath-gradle.sh /usr/local/lib/ploy/sbom/collect-java-classpath-gradle.sh
COPY --chmod=755 images/sbom/shared/collect-java-classpath-maven.sh /usr/local/lib/ploy/sbom/collect-java-classpath-maven.sh
COPY images/sbom/shared/gradle-write-java-classpath.init.gradle /usr/local/lib/ploy/sbom/gradle-write-java-classpath.init.gradle
COPY images/gates/gradle/gradle.properties /root/.gradle/gradle.properties
COPY images/gates/gradle/cache.init.gradle /root/.gradle/init.d/cache.init.gradle

CMD ["bash"]
