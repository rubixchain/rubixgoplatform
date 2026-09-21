GIT_COMMIT := $(shell git rev-parse HEAD)
PREV_COMMIT := $(shell git rev-parse HEAD~1)
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null)
LDFLAGS := -ldflags "-X github.com/rubixchain/rubixgoplatform/command.version=$(VERSION) -X github.com/rubixchain/rubixgoplatform/command.currentCommit=$(GIT_COMMIT) -X github.com/rubixchain/rubixgoplatform/command.previousCommit=$(PREV_COMMIT)"

compile-linux:
	echo "Compiling for Linux OS"
	go env -w GOOS=linux
	go env -w CGO_ENABLED=1
	go build $(LDFLAGS) -o linux/rubixgoplatform
compile-windows:
	echo "Compiling for Windows OS"
	go env -w GOOS=windows
	go env -w CGO_ENABLED=1
	go build $(LDFLAGS) -o windows/rubixgoplatform.exe

compile-mac:
	echo "Compiling for MacOS arm64"
	go env -w GOOS=darwin
	go env -w GOARCH=arm64
	go env -w CGO_ENABLED=1
	go build $(LDFLAGS) -o mac/rubixgoplatform

clean:
	rm -f linux/rubixgoplatform windows/rubixgoplatform.exe mac/rubixgoplatform

all: compile-linux compile-windows compile-mac

############################ Release ##################################

# goreleaser-cross tag tracks the bundled Go version. Must be >= go.mod's `go`
# directive (1.22) — the code uses go1.22 stdlib (cmp, slices), so the older
# v1.20 image (Go 1.20) fails the release build with "package cmp/slices not in
# GOROOT". Keep this in sync with go.mod, docker/rubix/Dockerfile, and ci.yml.
GORELEASER_VERSION := 1.22
GORELEASER_IMAGE := ghcr.io/goreleaser/goreleaser-cross:v$(GORELEASER_VERSION)

# Custom release notes: when RELEASE_NOTES.md exists at the repo root, use it as
# the GitHub release body (overrides GoReleaser's auto-generated changelog for
# that release only). When the file is absent, GoReleaser auto-generates notes
# from the commit history. So: drop a RELEASE_NOTES.md at the tagged commit for a
# curated release; omit it for an auto-generated one.
RELEASE_NOTES_FILE := RELEASE_NOTES.md
ifneq ($(wildcard $(RELEASE_NOTES_FILE)),)
RELEASE_NOTES_FLAG := --release-notes=/go/src/rubixgoplatform/$(RELEASE_NOTES_FILE)
else
RELEASE_NOTES_FLAG :=
endif

# Publish binaries to Gitbub. It is used in `release` Github Action Workflow
ifdef GITHUB_TOKEN
release:
	docker run \
		--rm \
		-e GITHUB_TOKEN=$(GITHUB_TOKEN) \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v `pwd`:/go/src/rubixgoplatform \
		-w /go/src/rubixgoplatform \
		$(GORELEASER_IMAGE) \
		release \
		--clean \
		$(RELEASE_NOTES_FLAG)
else
release:
	@echo "Error: GITHUB_TOKEN is not defined. Please define it before running 'make release'."
endif
