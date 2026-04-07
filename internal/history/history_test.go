package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/cache"
)

func testSetup(t *testing.T) (sc *cache.ScanCache, histDir string) {
	t.Helper()
	dir := t.TempDir()
	sc = cache.New(filepath.Join(dir, "cache"))
	histDir = filepath.Join(dir, "history")
	return sc, histDir
}

func testReport(components, edges int) *arch.ContextReport {
	svcs := make([]arch.ArchService, 0, components)
	for i := range components {
		svcs = append(svcs, arch.ArchService{Name: "svc" + string(rune('A'+i))})
	}
	edgeList := make([]arch.ArchEdge, 0, edges)
	for i := range edges {
		edgeList = append(edgeList, arch.ArchEdge{From: "a", To: "b" + string(rune('0'+i))})
	}
	return &arch.ContextReport{ScanCore: arch.ScanCore{Architecture: arch.ArchModel{
		Services: svcs,
		Edges:    edgeList,
	}}}
}

func TestRecordAndList(t *testing.T) {
	sc, histDir := testSetup(t)

	if err := Record(sc, histDir, Local, "/repo", "sha1", testReport(3, 2)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)
	if err := Record(sc, histDir, Local, "/repo", "sha2", testReport(5, 4)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)
	if err := Record(sc, histDir, Remote, "/other", "sha3", testReport(1, 0)); err != nil {
		t.Fatal(err)
	}

	entries, err := List(histDir, "/repo", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries for /repo, got %d", len(entries))
	}
	if entries[0].HeadSHA != "sha1" || entries[1].HeadSHA != "sha2" {
		t.Errorf("unexpected order: %v", entries)
	}
	if entries[0].Components != 3 || entries[1].Components != 5 {
		t.Errorf("unexpected components: %d, %d", entries[0].Components, entries[1].Components)
	}
}

func TestListWithLimit(t *testing.T) {
	sc, histDir := testSetup(t)

	for i := range 5 {
		_ = Record(sc, histDir, Local, "/repo", "sha"+string(rune('0'+i)), testReport(i+1, 0))
		time.Sleep(time.Millisecond)
	}

	entries, err := List(histDir, "/repo", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Components != 4 || entries[1].Components != 5 {
		t.Errorf("expected last 2 entries (4,5 components), got %d,%d", entries[0].Components, entries[1].Components)
	}
}

func TestGetReport(t *testing.T) {
	sc, histDir := testSetup(t)

	_ = Record(sc, histDir, Local, "/repo", "sha1", testReport(3, 2))
	time.Sleep(time.Millisecond)
	_ = Record(sc, histDir, Local, "/repo", "sha2", testReport(7, 5))

	report, err := GetReport(sc, histDir, "/repo", -1)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Architecture.Services) != 7 {
		t.Errorf("expected 7 components, got %d", len(report.Architecture.Services))
	}

	report, err = GetReport(sc, histDir, "/repo", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Architecture.Services) != 3 {
		t.Errorf("expected 3 components, got %d", len(report.Architecture.Services))
	}
}

func TestGetReportEmptyHistory(t *testing.T) {
	sc, histDir := testSetup(t)
	_, err := GetReport(sc, histDir, "/nope", -1)
	if err == nil {
		t.Fatal("expected error for empty history")
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
