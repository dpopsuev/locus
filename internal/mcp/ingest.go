package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"time"

	scribebridge "github.com/dpopsuev/locus/bridges/scribe"
	"github.com/dpopsuev/oculus/v3/engine"
)

const ndjsonType = "type"

func (h *handler) postScanToScribe(ctx context.Context, scanResult *engine.ScanResult, path string) {
	project := filepath.Base(path)
	result := scribebridge.TranslateScan(scanResult.Report, project)

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)

	for _, r := range result.Records {
		_ = enc.Encode(map[string]any{
			ndjsonType: "node",
			"id":       r.ID,
			"kind":     r.Kind,
			"title":    r.Title,
			"labels":   r.Labels,
			"extra":    r.Extra,
			"sections": r.Sections,
		})
	}
	for _, e := range result.Edges {
		_ = enc.Encode(map[string]any{
			ndjsonType: "edge",
			"from":     e.From,
			"to":       e.To,
			"relation": e.Relation,
		})
	}
	_ = enc.Encode(map[string]any{
		ndjsonType:    "meta",
		"source":      "locus",
		"scanned_at":  time.Now().UTC().Format(time.RFC3339),
		"total_nodes": len(result.Records),
		"total_edges": len(result.Edges),
	})

	url := fmt.Sprintf("%s?source=locus", h.ingestURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		slog.WarnContext(ctx, "scribe ingest: build request", "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/x-ndjson")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.WarnContext(ctx, "scribe ingest: POST failed", "error", err, "url", h.ingestURL)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	slog.InfoContext(ctx, "scribe ingest: complete",
		"status", resp.StatusCode,
		"nodes", len(result.Records),
		"edges", len(result.Edges),
		"project", project,
	)
}
