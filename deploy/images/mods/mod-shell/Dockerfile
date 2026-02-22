# mod-shell: Generic shell runner for Mods.
#
# This image executes a user-provided shell script inside the mounted
# Git workspace. It is intended for lightweight preprocessing steps
# such as generating rewrite.yml or other derived configs.
#
# See deploy/images/mods/README.md for the Mods runtime contract.

FROM --platform=$TARGETOS/$TARGETARCH bash:5.2

ARG BUILD_TS
LABEL org.opencontainers.image.source="https://github.com/iw2rmb/ploy" \
    org.opencontainers.image.title="mod-shell" \
    org.opencontainers.image.description="Ploy Mods: generic shell script runner" \
    org.opencontainers.image.created="$BUILD_TS"

WORKDIR /workspace

ENV WORKSPACE=/workspace \
    OUTDIR=/out \
    MOD_SHELL_SCRIPT=""

RUN mkdir -p /workspace /out

COPY --chmod=755 mod-shell.sh /usr/local/bin/mod-shell
RUN ln -s /usr/local/bin/mod-shell /usr/local/bin/mods-shell

ENTRYPOINT ["mod-shell"]
CMD ["--dir", "/workspace", "--out", "/out"]
