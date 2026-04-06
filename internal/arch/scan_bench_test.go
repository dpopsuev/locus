package arch_test

import (
	"testing"

	"github.com/dpopsuev/locus/internal/arch"
)

// BenchmarkScanAndBuild benchmarks a full scan of the Locus repo itself.
func BenchmarkScanAndBuild(b *testing.B) {
	root := "../.."
	b.ReportAllocs()
	for b.Loop() {
		_, err := arch.ScanAndBuild(root, arch.ScanOpts{
			Intent:       arch.IntentHealth,
			ExcludeTests: true,
		})
		if err != nil {
			b.Fatalf("ScanAndBuild: %v", err)
		}
	}
}
