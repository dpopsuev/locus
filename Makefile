.PHONY: build version test test-e2e vet fmt lint preflight install-hooks image test-container install deploy

VERSION ?= $(shell git describe --tags --always --dirty)
IMAGE   ?= locus:$(VERSION)
CONTAINER_RT ?= podman

version:
	@echo $(VERSION)

build:
	CGO_ENABLED=1 go build -trimpath -ldflags="-s -w -X main.Version=$(VERSION)" -o locus ./cmd/locus

install: image

test:
	go test ./... -count=1

test-e2e:
	go test -tags=e2e ./internal/testkit/... -count=1 -v
	$(MAKE) build
	go test -tags=e2e -run 'TestBug56_TypeScriptMissingLanguageServer' -count=1 -v .
	@if command -v docker >/dev/null 2>&1; then \
		docker image inspect locus >/dev/null 2>&1 || docker build -t locus .; \
		go test -tags=e2e -run 'TestBug56_TypeScriptMonorepoNoProject' -count=1 -v .; \
	else \
		echo "docker not found; skipping TestBug56_TypeScriptMonorepoNoProject"; \
	fi

fmt:
	go fmt ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

lint-new:
	golangci-lint run --new-from-rev=HEAD ./...

preflight: fmt vet lint test test-e2e

install-hooks:
	@echo '#!/bin/sh' > .git/hooks/pre-commit
	@echo 'make lint-new' >> .git/hooks/pre-commit
	@chmod +x .git/hooks/pre-commit
	@echo "pre-commit hook installed (runs make lint-new)"

image: build
	$(CONTAINER_RT) build -t $(IMAGE) .

deploy: image
	-$(CONTAINER_RT) stop locus 2>/dev/null
	-$(CONTAINER_RT) rm locus 2>/dev/null
	$(CONTAINER_RT) run -d --name locus \
		--userns=keep-id \
		--user $(shell id -u):$(shell id -g) \
		-p 8081:8081 \
		-v $(HOME):$(HOME):rbind \
		$(IMAGE) \
		serve --transport http --addr :8081

test-container: image
	go test -tags=e2e -run TestContainer_E2E -timeout 300s -v .

release: image
	@test -n "$(V)" || (echo "usage: make release V=v0.8.0" && exit 1)
	sed -i 's|quay.io/dpopsuev/locus:[^ "]*|quay.io/dpopsuev/locus:$(V)|g' README.md
	git add -A && git commit -m "release: $(V)" || true
	git tag $(V)
	git push origin main --tags
	$(CONTAINER_RT) tag $(IMAGE) quay.io/dpopsuev/locus:$(V)
	$(CONTAINER_RT) push quay.io/dpopsuev/locus:$(V)
