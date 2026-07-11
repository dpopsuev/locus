package mcp

import "testing"

func TestPathFromCacheKey(t *testing.T) {
	tests := []struct {
		ck   string
		want string
	}{
		{"/home/u/Workspace/oculus@abc123def456", "/home/u/Workspace/oculus"},
		{"/home/u/Workspace/oculus@abc123def456789012345678901234567890abcd-full", "/home/u/Workspace/oculus"},
		{"/repo@deadbeef-health", "/repo"},
		{"/repo@deadbeef-full-file", "/repo"},
		{"nosplit", ""},
		{"@onlysha", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := pathFromCacheKey(tt.ck)
		if got != tt.want {
			t.Errorf("pathFromCacheKey(%q) = %q, want %q", tt.ck, got, tt.want)
		}
	}
}
