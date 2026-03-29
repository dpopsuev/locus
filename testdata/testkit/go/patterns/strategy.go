package patterns

// Strategy defines a family of interchangeable algorithms.
type Strategy interface {
	Execute(input string) string
}

// AlphaStrategy is the first strategy implementation.
type AlphaStrategy struct{}

// NewAlphaStrategy creates an AlphaStrategy.
func NewAlphaStrategy() *AlphaStrategy { return &AlphaStrategy{} }

// Execute runs the alpha algorithm.
func (s *AlphaStrategy) Execute(input string) string {
	return "alpha:" + input
}

// BetaStrategy is the second strategy implementation.
type BetaStrategy struct{}

// Execute runs the beta algorithm.
func (s *BetaStrategy) Execute(input string) string {
	return "beta:" + input
}
