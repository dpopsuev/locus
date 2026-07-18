package cli

import (
	"encoding/json"
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
	tls := payload["typescript_language_server"]
	if tls != "ok" && tls != "missing" {
		t.Fatalf("typescript_language_server=%q, want ok|missing", tls)
	}
}
