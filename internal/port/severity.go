package port

// Severity classifies the urgency of a violation or finding.
type Severity string

// Severity levels for violations and findings.
const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityError    Severity = "error"
	SeverityCritical Severity = "critical"
)

// Confidence represents a detection confidence between 0.0 and 1.0.
type Confidence float64

// Score represents a quality or compliance score between 0 and 100.
type Score float64

// RiskLevel classifies the impact risk of a change.
type RiskLevel string

// Risk level constants.
const (
	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)
