# Variables
DOCKER_IMAGE := ghcr.io/mwieczorkiewicz/opcua_exporter
VERSION ?= latest
BINARY_NAME := opcua_exporter
GO_VERSION := 1.23

# Default target
.PHONY: all
all: build

# Build the Go binary
.PHONY: build
build:
	go build ./cmd/opcua_exporter

# Run tests
.PHONY: test
test:
	go test -short ./...

# Run e2e tests
.PHONY: e2e-test
e2e-test:
	go test ./e2e/...

# Run all tests
.PHONY: test-all
test-all: test e2e-test

# Build Docker image
.PHONY: docker-build
docker-build:
	docker build -t $(DOCKER_IMAGE):$(VERSION) .

# Build Docker image with buildx (multi-platform support)
.PHONY: docker-buildx
docker-buildx:
	docker buildx build --platform linux/amd64,linux/arm64 -t $(DOCKER_IMAGE):$(VERSION) .

# Push Docker image to GHCR
.PHONY: docker-push
docker-push:
	docker push $(DOCKER_IMAGE):$(VERSION)

# Build and push Docker image
.PHONY: docker-build-push
docker-build-push: docker-build docker-push

# Build and push Docker image with buildx (multi-platform)
.PHONY: docker-buildx-push
docker-buildx-push:
	docker buildx build --platform linux/amd64,linux/arm64 -t $(DOCKER_IMAGE):$(VERSION) --push .

# Login to GHCR (requires GHCR_TOKEN environment variable)
.PHONY: docker-login
docker-login:
	@echo "$$GHCR_TOKEN" | docker login ghcr.io -u $(shell echo "$$GITHUB_USER") --password-stdin

# Tag image with additional tags (latest, sha)
.PHONY: docker-tag-latest
docker-tag-latest:
	docker tag $(DOCKER_IMAGE):$(VERSION) $(DOCKER_IMAGE):latest

# Complete workflow: test, build, and push
.PHONY: release
release: test-all docker-build-push docker-tag-latest
	docker push $(DOCKER_IMAGE):latest

# Development workflow: build and test
.PHONY: dev
dev: build test

# Clean build artifacts
.PHONY: clean
clean:
	rm -f $(BINARY_NAME)
	docker image prune -f

# Help target
.PHONY: help
help:
	@echo "Available targets:"
	@echo "  build              - Build the Go binary"
	@echo "  test               - Run unit tests"
	@echo "  e2e-test          - Run e2e tests"
	@echo "  test-all          - Run all tests"
	@echo "  docker-build      - Build Docker image"
	@echo "  docker-buildx     - Build multi-platform Docker image"
	@echo "  docker-push       - Push Docker image to GHCR"
	@echo "  docker-build-push - Build and push Docker image"
	@echo "  docker-buildx-push - Build and push multi-platform Docker image"
	@echo "  docker-login      - Login to GHCR (requires GHCR_TOKEN and GITHUB_USER env vars)"
	@echo "  docker-tag-latest - Tag current image as latest"
	@echo "  release           - Complete release workflow (test, build, push)"
	@echo "  dev               - Development workflow (build and test)"
	@echo "  clean             - Clean build artifacts"
	@echo "  help              - Show this help message"
	@echo ""
	@echo "Variables:"
	@echo "  VERSION           - Docker image version (default: latest)"
	@echo "  DOCKER_IMAGE      - Docker image name (default: ghcr.io/mwieczorkiewicz/opcua_exporter)"