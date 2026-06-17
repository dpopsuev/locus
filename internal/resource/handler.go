package resource

import (
	"encoding/json"
	"net/http"
)

// Handler returns an http.HandlerFunc for GET /debug/resources.
func Handler(m *Monitor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var snap *Snapshot
		if r.URL.Query().Get("refresh") == "true" {
			snap = m.Collect()
		} else {
			snap = m.Latest()
		}
		if snap == nil {
			snap = m.Collect()
		}
		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(snap)
	}
}
