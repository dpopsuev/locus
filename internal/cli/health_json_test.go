package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestHealthJSON_IncludesTypeScriptToolchain(t *testing.T) {
	t.Parallel()
	raw := healthJSON()
	var payload map[string]string
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("healthJSON not JSON: %v (%q)", err, raw)
	}
	if payload["status"] != "ok" || payload["service"] != "locus" {
		t.Fatalf("unexpected liveness fields: %#v", payload)
	}
	if _, ok := payload["typescript_language_server"]; !ok {
		t.Fatalf("missing typescript_language_server: %#v", payload)
	}
	if _, ok := payload["tsserver_path"]; !ok {
		t.Fatalf("missing tsserver_path: %#v", payload)
	}
	if _, ok := payload["tsserver_status"]; !ok {
		t.Fatalf("missing tsserver_status: %#v", payload)
	}
	tls := payload["typescript_language_server"]
	if tls != "ok" && tls != "missing" {
		t.Fatalf("typescript_language_server=%q, want ok|missing", tls)
	}
	st := payload["tsserver_status"]
	if st != "ok" && st != "missing" && st != "broken_env" {
		t.Fatalf("tsserver_status=%q, want ok|missing|broken_env", st)
	}
}

func TestHealthJSON_BrokenEnvWhenConfiguredPathMissing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such-tsserver.js")
	t.Setenv("LOCUS_TSSERVER_PATH", missing)
	raw := healthJSON()
	var payload map[string]string
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("healthJSON: %v", err)
	}
	if payload["tsserver_status"] != "broken_env" {
		t.Fatalf("tsserver_status=%q, want broken_env (payload=%#v)", payload["tsserver_status"], payload)
	}
	if payload["tsserver_path"] != "" {
		t.Fatalf("tsserver_path should be empty when broken, got %q", payload["tsserver_path"])
	}
	if !strings.Contains(payload["tsserver_hint"], missing) {
		t.Fatalf("hint should mention configured path: %q", payload["tsserver_hint"])
	}
}
