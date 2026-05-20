package query_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/dpopsuev/locus/internal/store"
	"github.com/dpopsuev/locus/query"
	"github.com/dpopsuev/oculus/v3/cache"
)

func TestQuery_FixtureScan(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(filepath.Dir(file)), "testdata/testkit/go")
	dir := t.TempDir()
	sc := cache.New(dir)
	fs := store.NewFilesystem(sc, filepath.Join(dir, "history"))
	client := query.New(fs, []string{root})
	ctx := context.Background()

	resp, err := client.Query(ctx, query.Request{
		Path:    root,
		Actions: []query.Action{
			{Name: "cycles"},
			{Name: "coupling"},
			{Name: "pattern_scan"},
		},
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	for i, r := range resp.Results {
		var v json.RawMessage
		_ = json.Unmarshal(r.Data, &v)
		fmt.Printf("result[%d] action=%s ok=%v err=%q\n  data=%s\n", i, r.Action, r.OK, r.Err, string(r.Data)[:min2(200, len(r.Data))])
	}
}

func min2(a, b int) int {
	if a < b { return a }; return b
}
