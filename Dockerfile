FROM golang:1.25-alpine AS build
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /locus ./cmd/locus

FROM scratch
COPY --from=build /locus /locus
ENV LOCUS_CACHE_DIR=/data/cache
ENV LOCUS_HISTORY_DIR=/data/history
ENV LOCUS_TRANSPORT=http
ENV LOCUS_ADDR=:8081
VOLUME /data
EXPOSE 8081
ENTRYPOINT ["/locus", "serve"]
