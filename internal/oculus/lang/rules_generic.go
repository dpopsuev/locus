package lang

// GenericRules is the safe default for unknown languages.
// Returns no violations — conservative baseline that never produces false positives.
type GenericRules struct{}

func (g *GenericRules) IsVerblessViolation(_, _ string) bool { return false }
func (g *GenericRules) StdlibIdioms() map[string]bool        { return nil }
func (g *GenericRules) KnownVerbPrefixes() []string          { return nil }
