package lang

import "strings"

// TypeScriptRules implements Rules for the TypeScript programming language.
// TypeScript convention: PascalCase classes/interfaces are noun-like,
// camelCase functions start with verbs.
type TypeScriptRules struct{}

// tsVerbPrefixes lists verb prefixes common in TypeScript/JavaScript functions (camelCase).
var tsVerbPrefixes = []string{
	"get", "set", "create", "delete", "update", "find",
	"fetch", "handle", "process", "render", "compute",
	"build", "make", "check", "validate", "parse",
	"is", "has", "can", "should", "will",
	"on", "do", "run", "use", // React hooks pattern: useEffect, useState
	"add", "remove", "send", "load", "save",
	"open", "close", "read", "write", "start", "stop",
	"init", "register", "apply", "convert", "format",
	"enable", "disable", "reset", "clear",
	"emit", "dispatch", "subscribe", "unsubscribe",
	"mount", "unmount", "connect", "disconnect",
}

// tsStdlibIdioms are abbreviations idiomatic in TypeScript/JavaScript.
var tsStdlibIdioms = map[string]bool{
	"DOM": true, "HTML": true, "CSS": true, "URL": true, "API": true,
	"HTTP": true, "JSON": true, "XML": true, "SQL": true, "ID": true,
	"IO": true, "IP": true, "TCP": true, "UDP": true, "OK": true,
	"UI": true, "UX": true, "DB": true, "JWT": true, "UUID": true,
}

// IsVerblessViolation returns true if the exported symbol name violates
// TypeScript naming conventions.
func (t *TypeScriptRules) IsVerblessViolation(name, kind string) bool {
	// PascalCase types are noun-like — never violations.
	switch kind {
	case "class", "interface", "enum", "type-parameter", "struct",
		"constant", "variable", "field", "property", "enum-member":
		return false
	}

	// Check verb prefixes (camelCase).
	for _, prefix := range tsVerbPrefixes {
		if strings.HasPrefix(name, prefix) {
			return false
		}
	}

	return true
}

// StdlibIdioms returns TypeScript standard abbreviations.
func (t *TypeScriptRules) StdlibIdioms() map[string]bool {
	return tsStdlibIdioms
}

// KnownVerbPrefixes returns TypeScript verb prefixes.
func (t *TypeScriptRules) KnownVerbPrefixes() []string {
	return tsVerbPrefixes
}
