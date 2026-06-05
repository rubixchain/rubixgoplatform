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
# STAGE 2 — RUNTIME
# -------------------------
FROM alpine:latest

WORKDIR /app

# gcompat: the kubo (ipfs) release binaries for linux are glibc-linked
# (interpreter /lib64/ld-linux-*.so), but Alpine is musl. Without the glibc
# shim, `exec ./ipfs` fails with the misleading "no such file or directory"
# (missing ELF interpreter), so the node can't init IPFS and exits on startup.
RUN apk add --no-cache bash postgresql-client gettext gcompat

COPY --from=builder /app/rubixgoplatform /app/rubixgoplatform

# Vendored kubo (ipfs) release — kubo v0.19.1, linux-amd64. The release tarball
# is committed alongside this Dockerfile instead of downloaded at build time, so
# the image builds with no network dependency on dist.ipfs.tech. To bump the
# version, replace the tarball and update the filename below.
COPY test/docker/rubix/kubo_v0.19.1_linux-amd64.tar.gz /tmp/kubo.tar.gz
RUN tar -xzf /tmp/kubo.tar.gz -C /tmp \
 && cp /tmp/kubo/ipfs /app/ipfs \
 && rm -rf /tmp/kubo /tmp/kubo.tar.gz

# entrypoint + config template live alongside this Dockerfile (relative to the
# repo-root build context: test/docker/rubix/).
COPY test/docker/rubix/entrypoint.sh /app/entrypoint.sh
COPY test/docker/rubix/config.template.toml /app/config.template.toml

RUN chmod +x /app/entrypoint.sh /app/ipfs

ENTRYPOINT ["/app/entrypoint.sh"]
