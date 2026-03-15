package store

import (
	"context"
	"time"

	"github.com/dpopsuev/locus/internal/arch"
)

// Store is the port interface for all Locus persistence operations.
// Protocol depends on this interface, never on concrete backends.
// Adapters (filesystem, SQLite, LRU decorator) implement it.
type Store interface {
	// Reports — cached scan results keyed by (project path, git SHA).
	GetReport(ctx context.Context, project, sha string) (*arch.ContextReport, bool, error)
	PutReport(ctx context.Context, project, sha string, report *arch.ContextReport) error

	// History — append-only log of scan events per project.
	RecordScan(ctx context.Context, source, repoPath, sha string, report *arch.ContextReport) error
	ListHistory(ctx context.Context, repoPath string, limit int) ([]HistoryEntry, error)
	GetHistoryReport(ctx context.Context, repoPath string, index int) (*arch.ContextReport, error)

	// Git helpers — resolve refs to SHAs. Implementations may cache or delegate to git CLI.
	ResolveHEAD(repoPath string) string
	ResolveBranch(repoPath, ref string) (string, error)

	// Project registry
	ListProjects(ctx context.Context) ([]ProjectInfo, error)
	UpsertProject(ctx context.Context, info ProjectInfo) error

	// Component metadata
	PutComponentMeta(ctx context.Context, project, sha string, meta []ComponentMeta) error
	ListComponentMeta(ctx context.Context, project, sha string) ([]ComponentMeta, error)
	SearchComponents(ctx context.Context, project, sha, query string) ([]ComponentMeta, error)

	// Lifecycle
	Close() error
}

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

// HistoryEntry summarizes a single scan event in the history log.
type HistoryEntry struct {
	Timestamp  time.Time `json:"timestamp"`
	SHA        string    `json:"sha"`
	Source     string    `json:"source"` // "local" or "remote"
	RepoPath   string    `json:"repo_path"`
	Components int       `json:"components"`
	Edges      int       `json:"edges"`
}
