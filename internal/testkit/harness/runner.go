// Package harness provides the test runner that wires Locus store/cache with
// the analysis engine. Separated from testkit so the fixture/manifest helpers
// can drain to Oculus without pulling in Locus-only dependencies.
package harness

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dpopsuev/locus/internal/arch"
	"github.com/dpopsuev/locus/internal/cache"
	"github.com/dpopsuev/locus/internal/clinic"
	clinichexa "github.com/dpopsuev/locus/internal/clinic/hexa"
	clinicnaming "github.com/dpopsuev/locus/internal/clinic/naming"
	clinicsolid "github.com/dpopsuev/locus/internal/clinic/solid"
	"github.com/dpopsuev/locus/internal/diagram"
	diagramcore "github.com/dpopsuev/locus/internal/diagram/core"
	"github.com/dpopsuev/locus/internal/history"
	"github.com/dpopsuev/locus/internal/protocol"
	"github.com/dpopsuev/locus/internal/store"
	"github.com/dpopsuev/locus/internal/testkit"
)

// RunFixture loads a manifest and validates the fixture against Locus analysis.
func RunFixture(t *testing.T, fixtureDir string) {
	t.Helper()

	manifestPath := filepath.Join(fixtureDir, "manifest.json")
	manifest, err := testkit.LoadManifest(manifestPath)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	intent := manifest.ScanIntent
	if intent == "" {
		intent = "health"
	}

	report, err := arch.ScanAndBuild(fixtureDir, arch.ScanOpts{
		Intent: arch.ScanIntent(intent),
	})
	if err != nil {
		t.Fatalf("scan fixture: %v", err)
	}

	checkStructure(t, report, manifest)
	checkPatternScan(t, report, manifest)
	checkHexa(t, report, manifest)
	checkSOLID(t, report, manifest, fixtureDir)
	checkSymbols(t, report, manifest)
	checkDiagrams(t, report, manifest, fixtureDir)
	checkPresets(t, manifest, fixtureDir, intent)
}

func checkStructure(t *testing.T, report *arch.ContextReport, m *testkit.Manifest) {
	t.Helper()
	t.Run("components", func(t *testing.T) {
		if len(report.Architecture.Services) < m.ExpectedComponentsMin {
			t.Errorf("expected >= %d components, got %d",
				m.ExpectedComponentsMin, len(report.Architecture.Services))
		}
	})
	t.Run("edges", func(t *testing.T) {
		if len(report.Architecture.Edges) < m.ExpectedEdgesMin {
			t.Errorf("expected >= %d edges, got %d",
				m.ExpectedEdgesMin, len(report.Architecture.Edges))
		}
	})
}

func checkPatternScan(t *testing.T, report *arch.ContextReport, m *testkit.Manifest) {
	t.Helper()
	if len(m.ExpectedSmells) == 0 && len(m.ExpectedPatterns) == 0 {
		return
	}
	t.Run("pattern_scan", func(t *testing.T) {
		pr := clinic.ComputePatternScan(
			report.Architecture.Services, report.Architecture.Edges,
			report.Cycles, nil, nil, nil, nil,
		)
		for _, exp := range m.ExpectedSmells {
			if !hasDetection(pr.Detections, exp.ID) {
				t.Errorf("expected smell %s not detected", exp.ID)
			}
		}
		for _, exp := range m.ExpectedPatterns {
			if !hasDetection(pr.Detections, exp.ID) {
				t.Errorf("expected pattern %s not detected", exp.ID)
			}
		}
	})
}

func checkHexa(t *testing.T, report *arch.ContextReport, m *testkit.Manifest) {
	t.Helper()
	if m.ExpectedHexa == nil {
		return
	}
	t.Run("hexa", func(t *testing.T) {
		hr := clinichexa.ComputeHexaViolations(
			report.Architecture.Services, report.Architecture.Edges, nil,
		)
		if len(hr.Violations) > m.ExpectedHexa.MaxViolations {
			t.Errorf("expected <= %d hexa violations, got %d",
				m.ExpectedHexa.MaxViolations, len(hr.Violations))
		}
		roleMap := make(map[string]string, len(hr.Classification))
		for _, c := range hr.Classification {
			roleMap[c.Name] = string(c.Role)
		}
		assertRoles(t, roleMap, m.ExpectedHexa.Entrypoint, "entrypoint")
		assertRoles(t, roleMap, m.ExpectedHexa.Adapter, "adapter")
		assertRoles(t, roleMap, m.ExpectedHexa.Infra, "infra")
		assertRoles(t, roleMap, m.ExpectedHexa.Port, "port")
		assertRoles(t, roleMap, m.ExpectedHexa.Domain, "domain")
	})
}

func checkSOLID(t *testing.T, report *arch.ContextReport, m *testkit.Manifest, root string) {
	t.Helper()
	if m.ExpectedSOLID == nil {
		return
	}
	t.Run("solid", func(t *testing.T) {
		sr := clinicsolid.ComputeSOLIDScan(
			report.Architecture.Services, report.Architecture.Edges,
			nil, nil, nil, root, nil, nil,
		)
		if len(sr.Violations) > m.ExpectedSOLID.MaxViolations {
			t.Errorf("expected <= %d SOLID violations, got %d",
				m.ExpectedSOLID.MaxViolations, len(sr.Violations))
		}
	})
}

func checkSymbols(t *testing.T, report *arch.ContextReport, m *testkit.Manifest) {
	t.Helper()
	if m.ExpectedSymbols == nil {
		return
	}
	t.Run("symbols", func(t *testing.T) {
		sq := clinicnaming.ComputeSymbolQuality(
			report.Architecture.Services, report.Architecture.Edges,
		)
		for _, abbr := range m.ExpectedSymbols.Abbreviations {
			if !hasIssue(sq.Issues, "abbreviation", abbr) {
				t.Errorf("expected abbreviation %q not detected", abbr)
			}
		}
		for _, gen := range m.ExpectedSymbols.GenericNames {
			if !hasIssue(sq.Issues, "generic_name", gen) {
				t.Errorf("expected generic name %q not detected", gen)
			}
		}
	})
}

func checkDiagrams(t *testing.T, report *arch.ContextReport, m *testkit.Manifest, root string) {
	t.Helper()
	if len(m.ExpectedDiagrams) == 0 {
		return
	}
	t.Run("diagrams", func(t *testing.T) {
		for _, dtype := range m.ExpectedDiagrams {
			t.Run(dtype, func(t *testing.T) {
				_, err := diagram.Render(
					diagramcore.Input{Report: report, Root: root},
					diagramcore.Options{Type: dtype},
				)
				if err != nil {
					t.Errorf("diagram %s failed: %v", dtype, err)
				}
			})
		}
	})
}

func checkPresets(t *testing.T, m *testkit.Manifest, root, intent string) {
	t.Helper()
	if len(m.ExpectedPresets) == 0 {
		return
	}
	t.Run("presets", func(t *testing.T) {
		sc := cache.New(t.TempDir())
		db := store.NewFilesystem(sc, history.DefaultHistoryDir())
		proto := protocol.New(db, nil)
		ctx := context.Background()
		if _, err := proto.ScanProject(ctx, root, protocol.ScanOpts{Intent: intent}); err != nil {
			t.Fatalf("scan for presets: %v", err)
		}
		for _, preset := range m.ExpectedPresets {
			t.Run(preset, func(t *testing.T) {
				out, err := proto.RunPreset(ctx, root, preset)
				if err != nil {
					t.Errorf("preset %s failed: %v", preset, err)
					return
				}
				if out == "" {
					t.Errorf("preset %s returned empty output", preset)
				}
			})
		}
	})
}

func hasDetection(detections []clinic.PatternDetection, id string) bool {
	for i := range detections {
		if detections[i].PatternID == id {
			return true
		}
	}
	return false
}

func hasIssue(issues []clinicnaming.SymbolIssue, issueType, substr string) bool {
	for _, i := range issues {
		if i.Issue == issueType && strings.Contains(i.Symbol, substr) {
			return true
		}
	}
	return false
}

func assertRoles(t *testing.T, roleMap map[string]string, names []string, expectedRole string) {
	t.Helper()
	for _, name := range names {
		found := false
		for comp, role := range roleMap {
			if strings.HasSuffix(comp, name) || comp == name {
				found = true
				if role != expectedRole {
					t.Errorf("component %s: expected role %s, got %s", comp, expectedRole, role)
				}
				break
			}
		}
		if !found {
			t.Errorf("component %s not found in classification", name)
		}
	}
}
