# mig-shell: Generic shell runner for Migs.
#
# This image executes a user-provided shell script inside the mounted
# Git workspace. It is intended for lightweight preprocessing steps
# such as generating rewrite.yml or other derived configs.
#
# See images/README.md for the Migs runtime contract.

FROM --platform=$TARGETOS/$TARGETARCH bash:5.2

ARG BUILD_TS
LABEL org.opencontainers.image.source="https://github.com/iw2rmb/ploy" \
    org.opencontainers.image.title="mig-shell" \
    org.opencontainers.image.description="Ploy Migs: generic shell script runner" \
    org.opencontainers.image.created="$BUILD_TS"

WORKDIR /workspace

ENV WORKSPACE=/workspace \
    OUTDIR=/out \
    MOD_SHELL_SCRIPT=""

RUN mkdir -p /workspace /out

COPY --chmod=755 mig-shell.sh /usr/local/bin/mig-shell
RUN ln -s /usr/local/bin/mig-shell /usr/local/bin/shell

ENTRYPOINT ["mig-shell"]
CMD ["--dir", "/workspace", "--out", "/out"]
