default: build

build:
    go build -o ./bin/locus ./cmd/locus

test:
    go test ./...

install:
    go install ./cmd/locus
