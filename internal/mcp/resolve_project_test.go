package mcp

import (
	"context"
	"path/filepath"
	"testing"

	oculuscache "github.com/dpopsuev/oculus/v3/cache"
	"github.com/dpopsuev/oculus/v3/engine"
	"github.com/dpopsuev/oculus/v3/port"
	"github.com/dpopsuev/locus/internal/store"
)

func newHandlerWithWorkspaces(t *testing.T, workspaces []string) (h *handler, db store.Store) {
	t.Helper()
	sc := oculuscache.New(t.TempDir())
	db = store.NewFilesystem(sc, t.TempDir())
	return &handler{proto: engine.New(db, workspaces)}, db
}

// fakeProjectPath builds a deterministic non-temp path for store tests.
// isTempPath filters /tmp paths from the project registry, so tests must
// use paths outside os.TempDir().
func fakeProjectPath(parts ...string) string {
	return filepath.Join(append([]string{"/fake", "workspace"}, parts...)...)
}

func TestResolveDefaultProject_WorkspaceRootIsScannedProject(t *testing.T) {
	alef := fakeProjectPath("alef")
	utiqa := fakeProjectPath("utiqa")

	h, db := newHandlerWithWorkspaces(t, []string{alef})

	ctx := context.Background()
	_ = db.UpsertProject(ctx, port.ProjectInfo{Path: alef, Name: "alef"})
	_ = db.UpsertProject(ctx, port.ProjectInfo{Path: utiqa, Name: "utiqa"})

	got := h.resolveDefaultProject(ctx)
	if got != alef {
		t.Errorf("resolveDefaultProject() = %q, want %q (workspace root)", got, alef)
	}
}

func TestResolveDefaultProject_WorkspaceInsideProject(t *testing.T) {
	project := fakeProjectPath("project")
	subdir := fakeProjectPath("project", "packages", "core")

	h, db := newHandlerWithWorkspaces(t, []string{subdir})

	ctx := context.Background()
	_ = db.UpsertProject(ctx, port.ProjectInfo{Path: project, Name: "project"})

	got := h.resolveDefaultProject(ctx)
	if got != project {
		t.Errorf("resolveDefaultProject() = %q, want %q (enclosing project)", got, project)
	}
}

func TestResolveDefaultProject_NoRegisteredProjects(t *testing.T) {
	h, _ := newHandlerWithWorkspaces(t, []string{fakeProjectPath("empty")})

	got := h.resolveDefaultProject(context.Background())
	if got != "" {
		t.Errorf("resolveDefaultProject() = %q, want empty string", got)
	}
}

func TestResolveDefaultProject_WorkspaceRootNotAProject(t *testing.T) {
	parentDir := fakeProjectPath("projects")
	alef := fakeProjectPath("projects", "alef")

	h, db := newHandlerWithWorkspaces(t, []string{parentDir})

	ctx := context.Background()
	_ = db.UpsertProject(ctx, port.ProjectInfo{Path: alef, Name: "alef"})

	got := h.resolveDefaultProject(ctx)
	if got != "" {
		t.Errorf("resolveDefaultProject() = %q, want empty (workspace root is parent, not inside project)", got)
	}
}

func TestResolveDefaultProject_PrefersWorkspaceOverLastScanned(t *testing.T) {
	alef := fakeProjectPath("alef")
	utiqa := fakeProjectPath("utiqa")

	h, db := newHandlerWithWorkspaces(t, []string{alef})

	ctx := context.Background()
	_ = db.UpsertProject(ctx, port.ProjectInfo{Path: alef, Name: "alef"})
	_ = db.UpsertProject(ctx, port.ProjectInfo{Path: utiqa, Name: "utiqa"})

	h.lastScannedPath.Store(utiqa)

	got := h.resolveDefaultProject(ctx)
	if got != alef {
		t.Errorf("resolveDefaultProject() = %q, want %q (should prefer workspace over lastScannedPath)", got, alef)
	}
}
