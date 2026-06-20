package mcp

import (
	"context"
	"log/slog"
	"path/filepath"

	scribebridge "github.com/dpopsuev/locus/bridges/scribe"
	"github.com/dpopsuev/oculus/v3/engine"
)

func (h *handler) postScanToScribe(ctx context.Context, scanResult *engine.ScanResult, path string) {
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
