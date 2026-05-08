package organs

import (
	"testing"

	"github.com/dpopsuev/locus/internal/config"
)

func TestCapabilities_AllRegistered(t *testing.T) {
	s := config.NewStore()
	l := New(s, []string{"."})

	caps := l.Capabilities()

	expected := map[string]bool{
		"locus_scan":       false,
		"locus_probe":      false,
		"locus_callers":    false,
		"locus_deps":       false,
		"locus_violations": false,
		"locus_search":     false,
	}

	for _, c := range caps {
		if _, ok := expected[c.Name]; ok {
			expected[c.Name] = true
		} else {
			t.Errorf("unexpected organ: %s", c.Name)
		}
		if c.Execute == nil {
			t.Errorf("organ %s has nil Execute", c.Name)
		}
		if len(c.Schema) == 0 {
			t.Errorf("organ %s has empty Schema", c.Name)
		}
	}

	for name, found := range expected {
		if !found {
			t.Errorf("missing organ: %s", name)
		}
	}

	if len(caps) != 6 {
		t.Errorf("expected 6 organs, got %d", len(caps))
	}
}
