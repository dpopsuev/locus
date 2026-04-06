package lang

import "strings"

// RustRules implements Rules for the Rust programming language.
// Rust convention: PascalCase types are noun-like, snake_case functions start with verbs.
type RustRules struct{}

// rustVerbPrefixes lists verb prefixes common in Rust functions (snake_case).
var rustVerbPrefixes = []string{
	"get_", "set_", "new_", "create_", "build_", "from_",
	"into_", "as_", "to_", "is_", "has_", "try_",
	"parse_", "read_", "write_", "open_", "close_",
	"run_", "start_", "stop_", "handle_", "process_",
	"compute_", "check_", "validate_", "find_",
	"add_", "remove_", "insert_", "delete_", "update_",
	"send_", "recv_", "load_", "save_", "fetch_",
	"push_", "pop_", "peek_", "drain_", "flush_",
	"lock_", "unlock_", "spawn_", "join_", "await_",
	"encode_", "decode_", "serialize_", "deserialize_",
	"map_", "filter_", "fold_", "collect_", "iter_",
	"init_", "drop_", "clone_", "apply_", "execute_",
	"with_", "make_", "do_", "emit_",
}

// rustStdlibIdioms are abbreviations idiomatic in Rust's standard library.
var rustStdlibIdioms = map[string]bool{
	"io": true, "fs": true, "os": true, "fmt": true, "vec": true,
	"str": true, "ptr": true, "rc": true, "arc": true, "tx": true,
	"rx": true, "tcp": true, "udp": true, "ip": true, "http": true,
	"url": true, "api": true, "id": true, "ok": true, "err": true,
}

// IsVerblessViolation returns true if the exported symbol name violates
// Rust naming conventions.
func (r *RustRules) IsVerblessViolation(name, kind string) bool {
	// PascalCase types are noun-like — never violations.
	if isNonFunctionKind(kind) {
		return false
	}

	// Check verb prefixes (snake_case).
	for _, prefix := range rustVerbPrefixes {
		if strings.HasPrefix(name, prefix) {
			return false
		}
	}

	// Trait implementations (impl blocks) and macros are exempt.
	if strings.HasSuffix(name, "!") {
		return false
	}

	return true
}

// StdlibIdioms returns Rust standard library abbreviations.
func (r *RustRules) StdlibIdioms() map[string]bool {
	return rustStdlibIdioms
}

// KnownVerbPrefixes returns Rust verb prefixes.
func (r *RustRules) KnownVerbPrefixes() []string {
	return rustVerbPrefixes
}
