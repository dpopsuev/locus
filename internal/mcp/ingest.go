package mcp

import (
	"context"
	"log/slog"
	"path/filepath"
	"time"

	scribebridge "github.com/dpopsuev/locus/bridges/scribe"
	"github.com/dpopsuev/oculus/v3/engine"
)

const ingestTimeout = 120 * time.Second

func (h *handler) postScanToScribe(_ context.Context, scanResult *engine.ScanResult, path string) {
	ctx, cancel := context.WithTimeout(context.Background(), ingestTimeout)
	defer cancel()

	project := filepath.Base(path)

	sg, err := h.proto.GetSymbolGraph(ctx, path)
	if err != nil {
		slog.WarnContext(ctx, "symbol graph unavailable, ingesting without symbols",
			"error", err, "project", project)
	}

	if err := scribebridge.IngestScan(ctx, scanResult.Report, sg, project, h.ingestURL); err != nil {
		slog.WarnContext(ctx, "scribe ingest failed", "error", err, "project", project)
	}
}
