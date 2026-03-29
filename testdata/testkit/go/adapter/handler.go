package adapter

import (
	"fmt"
	"net/http"
)

// HealthHandler is an HTTP handler for health checks.
func HealthHandler(w http.ResponseWriter, _ *http.Request) {
	fmt.Fprintln(w, "ok")
}
