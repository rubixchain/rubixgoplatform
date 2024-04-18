compile-linux:
	echo "Compiling for Linux OS"
	go env -w GOOS=linux
	go env -w CGO_ENABLED=1
	go build -o linux/rubixgoplatform
compile-windows:
	echo "Compiling for Windows OS"
	go env -w GOOS=windows
	go env -w CGO_ENABLED=1
	go build -o windows/rubixgoplatform.exe

compile-mac:
	echo "Compiling for MacOS arm64"
	go env -w GOOS=darwin
	go env -w GOARCH=arm64
	go env -w CGO_ENABLED=1
	go build -o mac/rubixgoplatform

clean:
	rm -f linux/rubixgoplatform windows/rubixgoplatform.exe mac/rubixgoplatform

all: compile-linux compile-windows compile-mac

############################ Release ##################################

GORELEASER_VERSION := 1.20
GORELEASER_IMAGE := ghcr.io/goreleaser/goreleaser-cross:v$(GORELEASER_VERSION)

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
		--clean
else
release:
	@echo "Error: GITHUB_TOKEN is not defined. Please define it before running 'make release'."
endif

# Generate binaries in local 
release-dry-run:
	docker run \
		--rm \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v `pwd`:/go/src/rubixgoplatform \
		-w /go/src/rubixgoplatform \
		$(GORELEASER_IMAGE) \
		release \
		--clean \
		--skip-publish \
		--skip-validate
