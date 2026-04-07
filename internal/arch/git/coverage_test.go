package git

import (
	"os"
	"testing"
)

func TestParseCoverProfile(t *testing.T) {
	content := `mode: set
github.com/example/proj/pkg/store/store.go:10.30,15.2 3 1
github.com/example/proj/pkg/store/store.go:17.30,20.2 2 0
github.com/example/proj/pkg/api/handler.go:5.20,10.2 4 1
`
	tmp, err := os.CreateTemp("", "cover-test-*.out")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())
	tmp.WriteString(content)
	tmp.Close()

	results, err := parseCoverProfile(tmp.Name(), "github.com/example/proj")
	if err != nil {
		t.Fatal(err)
	}

	byComp := make(map[string]CoverageResult)
	for _, r := range results {
		byComp[r.Component] = r
	}

	store, ok := byComp["pkg/store"]
	if !ok {
		t.Fatal("missing pkg/store")
	}
	// 3 covered out of 5 total = 60%
	if store.CoveragePct != 60 {
		t.Errorf("store coverage: got %.1f, want 60.0", store.CoveragePct)
	}

	api, ok := byComp["pkg/api"]
	if !ok {
		t.Fatal("missing pkg/api")
	}
	if api.CoveragePct != 100 {
		t.Errorf("api coverage: got %.1f, want 100.0", api.CoveragePct)
	}
}
