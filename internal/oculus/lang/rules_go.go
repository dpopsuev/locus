package lang

import (
	"strings"
	"unicode"
)

// GoRules implements Rules for the Go programming language.
// Go convention: noun types (structs, interfaces) are idiomatic.
// Functions/methods should start with verb prefixes or have type-like suffixes.
type GoRules struct{}

// goVerbPrefixes lists verbs that commonly start Go function names.
var goVerbPrefixes = []string{
	"Get", "Set", "Create", "Delete", "Update", "Find", "Run",
	"Start", "Stop", "Open", "Close", "Read", "Write", "Parse",
	"Build", "Make", "Check", "Validate", "Compute", "Render",
	"Handle", "Process", "Init", "Register", "Add", "Remove",
	"Send", "Receive", "Load", "Save", "Fetch", "Put", "Do",
	"Apply", "Enable", "Disable", "Reset", "Clear", "Format",
	"Convert", "Transform", "Resolve", "Execute", "Marshal",
	"Unmarshal", "Encode", "Decode", "Scan", "Sort", "Filter",
	"Map", "Reduce", "Merge", "Split", "Join", "Wrap", "Unwrap",
	"Lock", "Unlock", "Wait", "Signal", "Notify", "Subscribe",
	"Publish", "Listen", "Serve", "Dial", "Connect", "Disconnect",
	"Flush", "Sync", "Log", "Print", "Dump",
	"Detect", "Extract", "Infer", "Suggest", "Classify", "Diff",
	"Generate", "Partition", "Reverse", "Collect", "Cache",
	"Contain", "Change",
}

// goExemptPrefixes are non-verb prefixes common in Go naming that should not be flagged.
var goExemptPrefixes = []string{
	"Is", "Has", "Can", "Must", "No", "With", "New",
	"Default", "Max", "Min", "Err", "To", "From",
}

// goTypeSuffixes identify symbols that are likely type declarations rather than functions.
var goTypeSuffixes = []string{
	// Common suffixes
	"Error", "Handler", "Server", "Client", "Config", "Options",
	"Result", "Response", "Request", "Store", "Cache", "Pool",
	"Queue", "Stack", "Buffer", "Builder", "Factory", "Provider",
	"Service", "Controller", "Middleware", "Router", "Adapter",
	"Wrapper", "Iterator", "Scanner", "Parser", "Encoder",
	"Decoder", "Formatter", "Validator", "Resolver", "Executor",
	"Scheduler", "Listener", "Observer", "Reporter", "Monitor",
	"Tracker", "Counter", "Logger", "Writer", "Reader", "Closer",
	"Flusher", "Interface", "Impl", "Func", "Map", "Set", "List",
	"Slice", "Array", "Chan", "Mutex", "Lock", "WaitGroup",
	"Context", "Signal", "Event", "Message", "Payload", "Packet",
	"Frame", "Record", "Entry", "Item", "Node", "Edge", "Graph",
	"Tree", "Spec", "Rule", "Policy", "Strategy", "Pattern",
	"Template", "Schema", "Model", "Entity", "DTO", "VO",
	// Enum/classification types
	"Kind", "Type", "Mode", "State", "Status", "Level", "Phase",
	"Role", "Scope", "Zone", "Layer", "Tier", "Class", "Category",
	// Data/metric types
	"Info", "Data", "Metric", "Metrics", "Report", "Summary",
	"Depth", "Width", "Height", "Size", "Count", "Index", "Offset",
	// Identifier/reference types
	"Key", "Value", "Name", "Path", "Dir", "File", "Ref",
	"ID", "URI", "URL", "Opt", "Opts", "Param", "Params",
	// AST/compiler types
	"Def", "Decl", "Stmt", "Expr", "Tok", "Op",
	// Domain-specific architectural types
	"Crossing", "Violation", "Constraint", "Threshold",
	"Component", "Package", "Module", "Namespace",
	"Anchor", "Symbol", "Token", "Annotation",
	"Cycle", "HotSpot", "Surface", "Drift",
	"Fingerprint", "Detection", "Catalog", "Evidence",
	// Additional common noun-type suffixes
	"Hash", "Group", "Ownership", "Commits",
	"Fallback", "Cohesion", "Radius",
}

// goStdlibIdioms are abbreviations that are idiomatic in Go's standard library.
var goStdlibIdioms = map[string]bool{
	"DB": true, "HTTP": true, "URL": true, "API": true, "ID": true,
	"IO": true, "IP": true, "TCP": true, "UDP": true, "JSON": true,
	"XML": true, "SQL": true, "CSS": true, "HTML": true, "OK": true,
	"EOF": true,
}

// verblessMinLen is the minimum symbol length to consider for verbless check.
// Short names (<=6 chars) are usually types (Config, Signal, etc.).
const goVerblessMinLen = 7

// IsVerblessViolation returns true if the exported symbol name violates
// Go naming conventions.
func (g *GoRules) IsVerblessViolation(name, kind string) bool {
	// Go convention: noun types are idiomatic — never violations.
	if isNonFunctionKind(kind) {
		return false
	}

	// Short names are usually types.
	if len(name) <= goVerblessMinLen {
		return false
	}

	runes := []rune(name)
	if len(runes) == 0 || !unicode.IsUpper(runes[0]) {
		return false
	}

	// Check exempt prefixes.
	for _, prefix := range goExemptPrefixes {
		if strings.HasPrefix(name, prefix) {
			return false
		}
	}

	// Check verb prefixes.
	for _, verb := range goVerbPrefixes {
		if strings.HasPrefix(name, verb) {
			return false
		}
	}

	// Check type suffixes.
	for _, suffix := range goTypeSuffixes {
		if strings.HasSuffix(name, suffix) {
			return false
		}
	}

	// Check camelCase tokens against type suffixes, verb prefixes, and exempt prefixes.
	// This catches compound names like "TopologicalSort" (Sort is a verb),
	// "BoundaryCrossing" (Crossing is a type suffix), and "SymbolsFromNames" (From is exempt).
	tokens := splitCamelCaseGo(name)
	for _, tok := range tokens {
		for _, suffix := range goTypeSuffixes {
			if strings.EqualFold(tok, suffix) {
				return false
			}
		}
		for _, verb := range goVerbPrefixes {
			if strings.EqualFold(tok, verb) {
				return false
			}
		}
		for _, exempt := range goExemptPrefixes {
			if strings.EqualFold(tok, exempt) {
				return false
			}
		}
	}

	return true // no verb, no type suffix -> violation
}

// StdlibIdioms returns Go standard library abbreviations.
func (g *GoRules) StdlibIdioms() map[string]bool {
	return goStdlibIdioms
}

// KnownVerbPrefixes returns Go verb prefixes.
func (g *GoRules) KnownVerbPrefixes() []string {
	return goVerbPrefixes
}

// splitCamelCaseGo splits a PascalCase/camelCase string into tokens.
// Duplicate of splitCamelCase from clinic to avoid import cycle.
func splitCamelCaseGo(s string) []string {
	runes := []rune(s)
	if len(runes) == 0 {
		return nil
	}

	var tokens []string
	start := 0

	for i := 1; i < len(runes); i++ {
		prev := runes[i-1]
		cur := runes[i]

		if unicode.IsLower(prev) && unicode.IsUpper(cur) {
			tokens = append(tokens, string(runes[start:i]))
			start = i
			continue
		}

		if i+1 < len(runes) && unicode.IsUpper(prev) && unicode.IsUpper(cur) && unicode.IsLower(runes[i+1]) {
			if start < i {
				tokens = append(tokens, string(runes[start:i]))
				start = i
			}
		}
	}

	if start < len(runes) {
		tokens = append(tokens, string(runes[start:]))
	}

	return tokens
}
