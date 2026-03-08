package remote

import (
	"testing"
)

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://github.com/org/repo", "https://github.com/org/repo"},
		{"https://github.com/org/repo.git", "https://github.com/org/repo"},
		{"github.com/org/repo", "https://github.com/org/repo"},
		{"git@github.com:org/repo.git", "https://github.com/org/repo"},
		{"git@github.com:org/repo", "https://github.com/org/repo"},
		{"http://gitlab.com/org/repo.git", "http://gitlab.com/org/repo"},
	}
	for _, tt := range tests {
		got := NormalizeURL(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeURL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestCloneDirDeterministic(t *testing.T) {
	d1 := CloneDir("https://github.com/org/repo", "main")
	d2 := CloneDir("https://github.com/org/repo", "main")
	if d1 != d2 {
		t.Errorf("CloneDir not deterministic: %q != %q", d1, d2)
	}

	d3 := CloneDir("https://github.com/org/repo", "dev")
	if d1 == d3 {
		t.Errorf("CloneDir should differ for different refs: both %q", d1)
	}
}

func TestCacheKey(t *testing.T) {
	key := CacheKey("https://github.com/org/repo", "abc123")
	want := "remote:https://github.com/org/repo@abc123"
	if key != want {
		t.Errorf("CacheKey = %q, want %q", key, want)
	}
}
