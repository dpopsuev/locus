FROM golang:1.25
RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc libc6-dev git ca-certificates universal-ctags \
    && rm -rf /var/lib/apt/lists/*
RUN go install golang.org/x/tools/gopls@latest

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w -X main.Version=${VERSION}" -o /usr/local/bin/locus ./cmd/locus

WORKDIR /
RUN rm -rf /build
ENV LOCUS_CACHE_DIR=/data/cache
ENV LOCUS_HISTORY_DIR=/data/history
ENV LOCUS_TRANSPORT=http
ENV LOCUS_ADDR=:8081
VOLUME /data
EXPOSE 8081
ENTRYPOINT ["locus", "serve"]
