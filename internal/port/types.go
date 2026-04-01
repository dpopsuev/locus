package port

// CallerSite represents a single call site in the codebase.
type CallerSite struct {
	Caller       string `json:"caller"`
	CallerPkg    string `json:"caller_pkg"`
	Line         int    `json:"line,omitempty"`
	File         string `json:"file,omitempty"`
	ReceiverType string `json:"receiver_type,omitempty"`
}
