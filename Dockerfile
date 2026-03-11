FROM golang:1.25 AS build
RUN apt-get update && apt-get install -y --no-install-recommends gcc libc6-dev && rm -rf /var/lib/apt/lists/*
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w -X main.Version=${VERSION}" -o /locus ./cmd/locus

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends git ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=build /locus /locus
ENV LOCUS_CACHE_DIR=/data/cache
ENV LOCUS_HISTORY_DIR=/data/history
ENV LOCUS_TRANSPORT=http
ENV LOCUS_ADDR=:8081
VOLUME /data
EXPOSE 8081
ENTRYPOINT ["/locus", "serve"]
