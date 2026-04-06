package lang

import "strings"

// PythonRules implements Rules for the Python programming language.
// Python convention: PascalCase classes are noun-like, snake_case functions start with verbs.
type PythonRules struct{}

// pythonVerbPrefixes lists verb prefixes common in Python functions (snake_case).
var pythonVerbPrefixes = []string{
	"get_", "set_", "create_", "delete_", "update_", "find_",
	"run_", "start_", "stop_", "open_", "close_", "read_", "write_",
	"parse_", "build_", "make_", "check_", "validate_", "compute_",
	"handle_", "process_", "init_", "register_", "add_", "remove_",
	"send_", "load_", "save_", "fetch_", "is_", "has_", "can_",
	"do_", "apply_", "convert_", "format_", "render_", "execute_",
	"to_", "from_", "as_",
}

// pythonStdlibIdioms are abbreviations idiomatic in Python's standard library.
var pythonStdlibIdioms = map[string]bool{
	"os": true, "io": true, "re": true, "db": true, "id": true,
	"ok": true, "ip": true, "url": true, "api": true, "http": true,
	"json": true, "xml": true, "sql": true, "csv": true, "html": true,
}

// IsVerblessViolation returns true if the exported symbol name violates
// Python naming conventions.
func (p *PythonRules) IsVerblessViolation(name, kind string) bool {
	// PascalCase classes are noun-like — never violations.
	if isNonFunctionKind(kind) {
		return false
	}

	// Constants in UPPER_SNAKE are exempt.
	if name == strings.ToUpper(name) && len(name) > 1 {
		return false
	}

	// Dunder methods (__init__, __str__) are exempt.
	if strings.HasPrefix(name, "__") && strings.HasSuffix(name, "__") {
		return false
	}

	// Private names starting with underscore are exempt from public naming rules.
	if strings.HasPrefix(name, "_") {
		return false
	}

	// Check verb prefixes.
	for _, prefix := range pythonVerbPrefixes {
		if strings.HasPrefix(name, prefix) {
			return false
		}
	}

	return true
}

// StdlibIdioms returns Python standard library abbreviations.
func (p *PythonRules) StdlibIdioms() map[string]bool {
	return pythonStdlibIdioms
}

// KnownVerbPrefixes returns Python verb prefixes.
func (p *PythonRules) KnownVerbPrefixes() []string {
	return pythonVerbPrefixes
}
