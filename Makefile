.PHONY: build build-image build-image-slim push-image run restart version test vet lint

VERSION ?= $(shell git describe --tags --always --dirty)

version:
	@echo $(VERSION)

build:
	CGO_ENABLED=1 go build -trimpath -ldflags="-s -w -X main.Version=$(VERSION)" -o locus ./cmd/locus

test:
	go test ./... -count=1

vet:
	go vet ./...

lint:
	golangci-lint run ./...

build-image:
	podman build --build-arg VERSION=$(VERSION) -t quay.io/dpopsuev/locus:$(VERSION) -t quay.io/dpopsuev/locus:latest .

build-image-slim:
	podman build -f Dockerfile.slim --build-arg VERSION=$(VERSION) -t quay.io/dpopsuev/locus:$(VERSION)-slim .

push-image:
	podman push quay.io/dpopsuev/locus:$(VERSION)
	podman push quay.io/dpopsuev/locus:latest

LOCUS_DATA ?= locus-data
LOCUS_WORKSPACE ?= $(HOME)/Workspace

run:
	podman rm -f locus 2>/dev/null || true
	podman run -d --name locus -p 8081:8081 \
		-v $(LOCUS_DATA):/data \
		-v $(LOCUS_WORKSPACE):/workspace:ro \
		quay.io/dpopsuev/locus:latest
	@sleep 1 && podman logs locus 2>&1 | tail -3

restart: build-image run
