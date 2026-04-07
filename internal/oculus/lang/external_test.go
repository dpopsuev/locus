package lang_test

import (
	"testing"

	"github.com/dpopsuev/locus/internal/oculus/lang"
)

func TestDetectLanguage_External(t *testing.T) {
	detected := lang.DetectLanguage(t.TempDir())
	_ = detected
}
