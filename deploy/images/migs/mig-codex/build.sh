#/bin/bash

cd ../amata
env GOOS=linux GOARCH=amd64 go build -o ../ploy/deploy/images/migs/mig-codex ./cmd/amata

cd ../ploy

if [ -n "${PLOY_CA_CERTS:-}" ]; then
    docker buildx build \
        --platform linux/amd64 \
        -f deploy/images/migs/mig-codex/Dockerfile \
        -t "${PLOY_CONTAINER_REGISTRY}/migs-codex:latest" \
        --secret=id=ploy_ca_bundle,src="${PLOY_CA_CERTS}" \
        .
else
    docker buildx build \
        --platform linux/amd64 \
        -f deploy/images/migs/mig-codex/Dockerfile \
        -t "${PLOY_CONTAINER_REGISTRY}/migs-codex:latest" \
        .
fi
