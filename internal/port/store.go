// Package port defines domain interfaces and types for Locus storage.
// Both protocol (domain) and store (infra) import from here.
// This breaks the DIP violation where protocol imported store directly.
//
// TSK-236 / GOL-27 / LCS-CMP-14
package port

import (
	"context"
	"time"

	"github.com/dpopsuev/locus/internal/arch"
)

// --- Domain-specific port interfaces ---

// ReportStore handles cached scan results keyed by (project path, git SHA).
type ReportStore interface {
	GetReport(ctx context.Context, project, sha string) (*arch.ContextReport, bool, error)
	PutReport(ctx context.Context, project, sha string, report *arch.ContextReport) error
}

// HistoryStore handles the append-only log of scan events per project.
type HistoryStore interface {
	RecordScan(ctx context.Context, source, repoPath, sha string, report *arch.ContextReport) error
	ListHistory(ctx context.Context, repoPath string, limit int) ([]HistoryEntry, error)
	GetHistoryReport(ctx context.Context, repoPath string, index int) (*arch.ContextReport, error)
}

// GitResolver resolves git refs to SHAs.
type GitResolver interface {
	ResolveHEAD(repoPath string) string
	ResolveBranch(repoPath, ref string) (string, error)
}

// ProjectStore handles the global project registry.
type ProjectStore interface {
	ListProjects(ctx context.Context) ([]ProjectInfo, error)
	UpsertProject(ctx context.Context, info ProjectInfo) error
}

// ComponentStore handles per-component metadata and search.
type ComponentStore interface {
	PutComponentMeta(ctx context.Context, project, sha string, meta []ComponentMeta) error
	ListComponentMeta(ctx context.Context, project, sha string) ([]ComponentMeta, error)
	SearchComponents(ctx context.Context, project, sha, query string) ([]ComponentMeta, error)
}

// DesiredStateStore handles per-project architecture rules.
type DesiredStateStore interface {
	GetDesiredState(ctx context.Context, project string) (*DesiredState, error)
	PutDesiredState(ctx context.Context, project string, state *DesiredState) error
}

// --- Composed interface ---

// Store is the composed interface of all domain ports.
type Store interface {
	ReportStore
	HistoryStore
	GitResolver
	ProjectStore
	ComponentStore
	DesiredStateStore
	Close() error
}

// --- Domain types ---

// ProjectInfo tracks a scanned project in the registry.
type ProjectInfo struct {
	Path       string    `json:"path"`
	Name       string    `json:"name"`
	Language   string    `json:"language"`
	LastSHA    string    `json:"last_sha"`
	LastScan   time.Time `json:"last_scan"`
	Components int       `json:"components"`
}

// ComponentMeta holds auto-generated metadata for a single component.
type ComponentMeta struct {
	Name        string   `json:"name"`
	Role        string   `json:"role"`
	Keywords    []string `json:"keywords"`
	Description string   `json:"description"`
	Layer       int      `json:"layer"`
	Health      string   `json:"health"`
	LOC         int      `json:"loc"`
	FanIn       int      `json:"fan_in"`
}

// DesiredState defines architecture rules for a project.
type DesiredState struct {
	Layers      []string            `json:"layers" yaml:"layers"`
	Boundaries  []BoundaryRule      `json:"boundaries,omitempty" yaml:"boundaries"`
	Constraints []HealthConstraint  `json:"constraints,omitempty" yaml:"constraints"`
	Roles       map[string]string   `json:"roles,omitempty" yaml:"roles"`
	Accepted    []AcceptedViolation `json:"accepted,omitempty" yaml:"accepted"`
}

// AcceptedViolation records a known violation that should be suppressed.
type AcceptedViolation struct {
	Component string `json:"component" yaml:"component"`
	Principle string `json:"principle" yaml:"principle"`
	Reason    string `json:"reason" yaml:"reason"`
}

// BoundaryRule defines an allowed or denied dependency path.
type BoundaryRule struct {
	FromPattern string `json:"from_pattern" yaml:"from_pattern"`
	ToPattern   string `json:"to_pattern" yaml:"to_pattern"`
	Allow       bool   `json:"allow" yaml:"allow"`
}

// HealthConstraint sets limits on a component's metrics.
type HealthConstraint struct {
	Component  string `json:"component" yaml:"component"`
	MaxFanIn   int    `json:"max_fan_in,omitempty" yaml:"max_fan_in"`
	MaxChurn   int    `json:"max_churn,omitempty" yaml:"max_churn"`
	MaxNesting int    `json:"max_nesting,omitempty" yaml:"max_nesting"`
}

// HistoryEntry summarizes a single scan event in the history log.
type HistoryEntry struct {
	Timestamp  time.Time `json:"timestamp"`
	SHA        string    `json:"sha"`
	Source     string    `json:"source"`
	RepoPath   string    `json:"repo_path"`
	Components int       `json:"components"`
	Edges      int       `json:"edges"`
}
