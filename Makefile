.PHONY: build version test vet fmt lint preflight install-hooks

VERSION ?= $(shell git describe --tags --always --dirty)

version:
	@echo $(VERSION)

build:
	CGO_ENABLED=1 go build -trimpath -ldflags="-s -w -X main.Version=$(VERSION)" -o locus ./cmd/locus

test:
	go test ./... -count=1

fmt:
	go fmt ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

lint-new:
	golangci-lint run --new-from-rev=HEAD ./...

preflight: fmt vet lint test

install-hooks:
	@echo '#!/bin/sh' > .git/hooks/pre-commit
	@echo 'make lint-new' >> .git/hooks/pre-commit
	@chmod +x .git/hooks/pre-commit
	@echo "pre-commit hook installed (runs make lint-new)"

release:
	@test -n "$(V)" || (echo "usage: make release V=v0.8.0" && exit 1)
	sed -i 's|quay.io/dpopsuev/locus:[^ "]*|quay.io/dpopsuev/locus:$(V)|g' README.md
	git add -A && git commit -m "release: $(V)" || true
	git tag $(V)
	git push origin main --tags
