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
	if err := scribebridge.IngestScan(ctx, scanResult.Report, project, h.ingestURL); err != nil {
		slog.WarnContext(ctx, "scribe ingest failed", "error", err, "project", project)
	}
}
