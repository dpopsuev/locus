package scribe_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bridge "github.com/dpopsuev/locus/bridges/scribe"
	"github.com/dpopsuev/locus/testdata"
)

func TestIngestScan_PostsNDJSON(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s; want POST", r.Method)
		}
		if !strings.Contains(r.URL.String(), "source=locus") {
			t.Errorf("URL = %s; want source=locus param", r.URL.String())
		}
		data, _ := io.ReadAll(r.Body)
		body = string(data)
		w.WriteHeader(http.StatusMultiStatus)
	}))
	defer srv.Close()

	report := testdata.SmallProject()
	err := bridge.IngestScan(context.Background(), report, "test-proj", srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, `"type":"node"`) {
		t.Error("body missing node records")
	}
	if !strings.Contains(body, `"type":"edge"`) {
		t.Error("body missing edge records")
	}
	if !strings.Contains(body, `"type":"meta"`) {
		t.Error("body missing meta record")
	}
	if !strings.Contains(body, "test-proj/api") {
		t.Error("body missing component ID test-proj/api")
	}
}

func TestIngestScan_MonorepoProject(t *testing.T) {
	var nodeCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		nodeCount = strings.Count(string(data), `"type":"node"`)
		w.WriteHeader(http.StatusMultiStatus)
	}))
	defer srv.Close()

	report := testdata.MonorepoProject()
	err := bridge.IngestScan(context.Background(), report, "mono", srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if nodeCount != 6 {
		t.Errorf("nodes = %d; want 6", nodeCount)
	}
}
