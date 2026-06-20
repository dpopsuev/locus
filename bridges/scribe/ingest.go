package scribe

import (
	"context"

	scribeclient "github.com/dpopsuev/scribe/client"
	oculus "github.com/dpopsuev/oculus/v3"
)

// IngestScan translates a scan result (with symbols) and POSTs to the Scribe ingest URL.
// When sg is non-nil, the full SymbolGraph (including private symbols and inter-symbol
// edges) is included. When sg is nil, only exported symbols from the architecture
// projection are posted.
func IngestScan(ctx context.Context, report *oculus.ContextReport, sg *oculus.SymbolGraph, project, ingestURL string) error {
	result := TranslateScanWithSymbols(report, sg, project)
	return scribeclient.Post(ctx, result.Records, result.Edges, "locus", ingestURL)
}
