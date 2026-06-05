# -------------------------
# Rubix node image for the integration test stack.
# Self-contained under test/docker/ — but the BUILD CONTEXT is the
# repo root (see docker-compose.stress.yml: context: ../..), because the Go
# binary is compiled from the repo's source tree (COPY . .).
# -------------------------

# -------------------------
# STAGE 1 — BUILD
# -------------------------
FROM golang:1.22-alpine AS builder

ARG TARGETARCH

WORKDIR /app

RUN apk add --no-cache build-base git

# Context is the repo root, so this copies the full Go project.
COPY . .

RUN GOARCH=${TARGETARCH} go build -o rubixgoplatform

# -------------------------
# STAGE 2 — KUBO (ipfs binary)
# Rubix shells out to ./ipfs for init and daemon lifecycle.
# Downloaded at build time (not committed) — TARGETARCH is set automatically by
# Docker BuildKit: amd64, arm64, or 386.
# -------------------------
FROM alpine:latest AS kubo-extractor

ARG TARGETARCH
ARG KUBO_VERSION=v0.19.1

RUN apk add --no-cache curl tar \
 && curl -fsSL "https://dist.ipfs.tech/kubo/${KUBO_VERSION}/kubo_${KUBO_VERSION}_linux-${TARGETARCH}.tar.gz" -o /tmp/kubo.tar.gz \
 && tar -xzf /tmp/kubo.tar.gz -C /tmp \
 && chmod +x /tmp/kubo/ipfs

# -------------------------
# STAGE 3 — RUNTIME
# -------------------------
FROM alpine:latest

WORKDIR /app

RUN apk add --no-cache bash postgresql-client gettext

COPY --from=builder /app/rubixgoplatform /app/rubixgoplatform
COPY --from=kubo-extractor /tmp/kubo/ipfs /app/ipfs

# entrypoint + config template live alongside this Dockerfile (relative to the
# repo-root build context: test/docker/rubix/).
COPY test/docker/rubix/entrypoint.sh /app/entrypoint.sh
COPY test/docker/rubix/config.template.toml /app/config.template.toml

RUN chmod +x /app/entrypoint.sh /app/ipfs

ENTRYPOINT ["/app/entrypoint.sh"]
