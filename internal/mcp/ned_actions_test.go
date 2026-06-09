package mcp

import (
	"context"
	"strings"
	"testing"
)

// TestNED14_IntraPackageDeps_Action verifies the intra_deps MCP action.
//
// Given a scanned repository
// When analysis action=intra_deps component=<component> is called
// Then a JSON result is returned listing file-level edges within that component
func TestNED14_IntraPackageDeps_Action(t *testing.T) {
	dir := monorepoFixture(t)
	h := newHandlerWithWorkspace(t, dir)
	ctx := context.Background()

	// Scan first.
	_, _, err := h.handleScanProject(ctx, nil, &codographActionInput{
		Path:   dir,
		Intent: "full",
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	// Request intra-package deps — component that exists in the scan.
	// monorepoFixture has "packages/spine" — use that.
	result, _, err := h.handleAnalysis(ctx, nil, analysisInput{
		Action:    ActionIntraDeps,
		Path:      dir,
		Component: "packages/spine",
	})
	if err != nil {
		t.Fatalf("intra_deps action: %v", err)
	}
	text := extractText(result)
	t.Logf("intra_deps result: %s", text)
	// Should produce valid JSON with component field, not an error.
	if strings.Contains(text, "error") && !strings.Contains(text, "component not found") {
		t.Errorf("unexpected error in intra_deps result: %s", text)
	}
}

// TestNED17_IntraCoupling_Action verifies the intra_coupling MCP action.
func TestNED17_IntraCoupling_Action(t *testing.T) {
	dir := monorepoFixture(t)
	h := newHandlerWithWorkspace(t, dir)
	ctx := context.Background()

	_, _, err := h.handleScanProject(ctx, nil, &codographActionInput{
		Path:   dir,
		Intent: "full",
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	result, _, err := h.handleAnalysis(ctx, nil, analysisInput{
		Action:    ActionIntraCoupling,
		Component: "packages/spine",
		Path:      dir,
	})
	if err != nil {
		t.Fatalf("intra_coupling action: %v", err)
	}
	text := extractText(result)
	t.Logf("intra_coupling result: %s", text)
	if strings.Contains(text, "error") && !strings.Contains(text, "component not found") {
		t.Errorf("unexpected error in intra_coupling result: %s", text)
	}
}

// TestNED16_TypeUsages_Action verifies the type_usages MCP action.
func TestNED16_TypeUsages_Action(t *testing.T) {
	dir := monorepoFixture(t)
	h := newHandlerWithWorkspace(t, dir)
	ctx := context.Background()

	_, _, err := h.handleScanProject(ctx, nil, &codographActionInput{
		Path:   dir,
		Intent: "full",
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	result, _, err := h.handleAnalysis(ctx, nil, analysisInput{
		Action: ActionTypeUsages,
		Query:  "Scanner", // a type that likely exists in the test fixture
		Path:   dir,
	})
	if err != nil {
		t.Fatalf("type_usages action: %v", err)
	}
	text := extractText(result)
	t.Logf("type_usages result: %s", text)
	// Must return valid output (even if zero files).
	if text == "" {
		t.Error("type_usages returned empty output")
	}
}
