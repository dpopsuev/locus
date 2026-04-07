package survey_test

import (
	"testing"

	"github.com/dpopsuev/locus/internal/survey"
)

func TestDetectLanguage_External(t *testing.T) {
	lang := survey.DetectLanguage(t.TempDir())
	_ = lang
}
