package resource

import "testing"

func TestParseKBLine(t *testing.T) {
	tests := []struct {
		line string
		want float64
	}{
		{"VmRSS:    21780 kB", 21780.0 / 1024},
		{"VmRSS: 0 kB", 0},
		{"VmRSS:", 0},
		{"", 0},
	}
	for _, tt := range tests {
		got := parseKBLine(tt.line)
		if got != tt.want {
			t.Errorf("parseKBLine(%q) = %f, want %f", tt.line, got, tt.want)
		}
	}
}

func TestSelfRSSMB(t *testing.T) {
	rss := selfRSSMB()
	if rss <= 0 {
		t.Skipf("selfRSSMB() = %f; /proc/self/status may not be available", rss)
	}
}
