FROM maven:3.9.11-eclipse-temurin-17

LABEL org.opencontainers.image.source="https://github.com/iw2rmb/ploy" \
    org.opencontainers.image.title="sbom-maven" \
    org.opencontainers.image.description="Ploy SBOM runner: Maven lane with deterministic Maven/JDK toolchain" \
    org.opencontainers.image.licenses="MIT"

RUN mkdir -p /workspace /in /out

WORKDIR /workspace
ENV WORKSPACE=/workspace \
    INDIR=/in \
    OUTDIR=/out

COPY --chmod=755 images/install_ploy_ca_bundle.sh /usr/local/lib/ploy/install_ploy_ca_bundle.sh

CMD ["bash"]
