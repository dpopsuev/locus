.PHONY: build version test test-e2e vet fmt lint preflight install-hooks docker test-container install

VERSION ?= $(shell git describe --tags --always --dirty)

version:
	@echo $(VERSION)

build:
	CGO_ENABLED=1 go build -trimpath -ldflags="-s -w -X main.Version=$(VERSION)" -o locus ./cmd/locus

install: docker

test:
	go test ./... -count=1

test-e2e:
	go test -tags=e2e ./internal/testkit/... -count=1 -v

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

docker: build
	docker build -t locus .

test-container: docker
	go test -tags=e2e -run TestContainer_E2E -timeout 300s -v .

release: docker
	@test -n "$(V)" || (echo "usage: make release V=v0.8.0" && exit 1)
	sed -i 's|quay.io/dpopsuev/locus:[^ "]*|quay.io/dpopsuev/locus:$(V)|g' README.md
	git add -A && git commit -m "release: $(V)" || true
	git tag $(V)
	git push origin main --tags
	docker tag locus quay.io/dpopsuev/locus:$(V)
	docker push quay.io/dpopsuev/locus:$(V)
