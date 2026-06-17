package scribe

import (
	"context"

	scribeclient "github.com/dpopsuev/scribe/client"
	oculus "github.com/dpopsuev/oculus/v3"
)

// IngestScan translates a scan result and POSTs to the Scribe ingest URL.
func IngestScan(ctx context.Context, report *oculus.ContextReport, project, ingestURL string) error {
	result := TranslateScan(report, project)
	return scribeclient.Post(ctx, result.Records, result.Edges, "locus", ingestURL)
}
