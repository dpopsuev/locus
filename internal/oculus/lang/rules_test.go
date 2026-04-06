package lang_test

import (
	"testing"

	"github.com/dpopsuev/locus/internal/oculus/lang"
)

// --- GenericRules ---

func TestGenericRules_NeverViolates(t *testing.T) {
	r := &lang.GenericRules{}
	if r.IsVerblessViolation("anything", "function") {
		t.Error("GenericRules should never report violations")
	}
}

func TestGenericRules_NilIdioms(t *testing.T) {
	r := &lang.GenericRules{}
	if r.StdlibIdioms() != nil {
		t.Error("GenericRules should return nil idioms")
	}
}

// --- GoRules ---

func TestGoRules_NounTypeIsNotViolation(t *testing.T) {
	r := &lang.GoRules{}
	if r.IsVerblessViolation("ArchModel", "struct") {
		t.Error("Go struct nouns should not be violations")
	}
}

func TestGoRules_InterfaceIsNotViolation(t *testing.T) {
	r := &lang.GoRules{}
	if r.IsVerblessViolation("Repository", "interface") {
		t.Error("Go interface nouns should not be violations")
	}
}

func TestGoRules_ConstantIsNotViolation(t *testing.T) {
	r := &lang.GoRules{}
	if r.IsVerblessViolation("MaxRetries", "constant") {
		t.Error("Go constant should not be a violation")
	}
}

func TestGoRules_VerblessFunctionIsViolation(t *testing.T) {
	r := &lang.GoRules{}
	if !r.IsVerblessViolation("Frobnicate", "function") {
		t.Error("Go function without verb prefix should be a violation")
	}
}

func TestGoRules_VerbFunctionIsNotViolation(t *testing.T) {
	r := &lang.GoRules{}
	if r.IsVerblessViolation("ComputeScore", "function") {
		t.Error("Go function with verb prefix should not be a violation")
	}
}

func TestGoRules_ExemptPrefixIsNotViolation(t *testing.T) {
	r := &lang.GoRules{}
	prefixes := []string{"IsValid", "HasPermission", "CanExecute", "NewBuilder", "MustParse"}
	for _, name := range prefixes {
		if r.IsVerblessViolation(name, "function") {
			t.Errorf("Go exempt prefix %q should not be a violation", name)
		}
	}
}

func TestGoRules_TypeSuffixIsNotViolation(t *testing.T) {
	r := &lang.GoRules{}
	names := []string{"UserHandler", "TokenValidator", "SessionStore", "BoundaryCrossing"}
	for _, name := range names {
		if r.IsVerblessViolation(name, "function") {
			t.Errorf("Go type suffix %q should not be a violation", name)
		}
	}
}

func TestGoRules_ShortNameIsNotViolation(t *testing.T) {
	r := &lang.GoRules{}
	if r.IsVerblessViolation("Signal", "function") {
		t.Error("Short names (<=7 chars) should not be violations")
	}
}

func TestGoRules_StdlibIdioms(t *testing.T) {
	r := &lang.GoRules{}
	idioms := r.StdlibIdioms()
	required := []string{"DB", "HTTP", "URL", "JSON", "API", "ID"}
	for _, abbr := range required {
		if !idioms[abbr] {
			t.Errorf("Go stdlib idioms should include %s", abbr)
		}
	}
}

func TestGoRules_KnownVerbPrefixes(t *testing.T) {
	r := &lang.GoRules{}
	prefixes := r.KnownVerbPrefixes()
	if len(prefixes) == 0 {
		t.Error("Go should have known verb prefixes")
	}
}

// --- PythonRules ---

func TestPythonRules_ClassIsNotViolation(t *testing.T) {
	r := &lang.PythonRules{}
	if r.IsVerblessViolation("HttpClient", "class") {
		t.Error("Python class should not be a violation")
	}
}

func TestPythonRules_VerbFunctionIsNotViolation(t *testing.T) {
	r := &lang.PythonRules{}
	names := []string{"get_user", "set_config", "create_session", "is_valid", "has_permission"}
	for _, name := range names {
		if r.IsVerblessViolation(name, "function") {
			t.Errorf("Python function %q should not be a violation", name)
		}
	}
}

func TestPythonRules_VerblessFunctionIsViolation(t *testing.T) {
	r := &lang.PythonRules{}
	if !r.IsVerblessViolation("user_data", "function") {
		t.Error("Python function without verb prefix should be a violation")
	}
}

func TestPythonRules_UpperSnakeConstantIsNotViolation(t *testing.T) {
	r := &lang.PythonRules{}
	if r.IsVerblessViolation("MAX_RETRIES", "function") {
		t.Error("Python UPPER_SNAKE constant should not be a violation")
	}
}

func TestPythonRules_DunderIsNotViolation(t *testing.T) {
	r := &lang.PythonRules{}
	if r.IsVerblessViolation("__init__", "function") {
		t.Error("Python dunder method should not be a violation")
	}
}

func TestPythonRules_PrivateIsNotViolation(t *testing.T) {
	r := &lang.PythonRules{}
	if r.IsVerblessViolation("_internal_helper", "function") {
		t.Error("Python private name should not be a violation")
	}
}

func TestPythonRules_StdlibIdioms(t *testing.T) {
	r := &lang.PythonRules{}
	idioms := r.StdlibIdioms()
	required := []string{"os", "io", "re"}
	for _, abbr := range required {
		if !idioms[abbr] {
			t.Errorf("Python stdlib idioms should include %s", abbr)
		}
	}
}

// --- TypeScriptRules ---

func TestTypeScriptRules_ClassIsNotViolation(t *testing.T) {
	r := &lang.TypeScriptRules{}
	if r.IsVerblessViolation("UserService", "class") {
		t.Error("TypeScript class should not be a violation")
	}
}

func TestTypeScriptRules_InterfaceIsNotViolation(t *testing.T) {
	r := &lang.TypeScriptRules{}
	if r.IsVerblessViolation("IRepository", "interface") {
		t.Error("TypeScript interface should not be a violation")
	}
}

func TestTypeScriptRules_VerbFunctionIsNotViolation(t *testing.T) {
	r := &lang.TypeScriptRules{}
	names := []string{"getUser", "setConfig", "createElement", "handleClick", "useEffect", "fetchData"}
	for _, name := range names {
		if r.IsVerblessViolation(name, "function") {
			t.Errorf("TypeScript function %q should not be a violation", name)
		}
	}
}

func TestTypeScriptRules_VerblessFunctionIsViolation(t *testing.T) {
	r := &lang.TypeScriptRules{}
	if !r.IsVerblessViolation("dataProcessor", "function") {
		t.Error("TypeScript function without verb prefix should be a violation")
	}
}

func TestTypeScriptRules_StdlibIdioms(t *testing.T) {
	r := &lang.TypeScriptRules{}
	idioms := r.StdlibIdioms()
	required := []string{"DOM", "HTML", "URL", "JSON", "API"}
	for _, abbr := range required {
		if !idioms[abbr] {
			t.Errorf("TypeScript stdlib idioms should include %s", abbr)
		}
	}
}

// --- RustRules ---

func TestRustRules_StructIsNotViolation(t *testing.T) {
	r := &lang.RustRules{}
	if r.IsVerblessViolation("HttpClient", "struct") {
		t.Error("Rust struct should not be a violation")
	}
}

func TestRustRules_EnumIsNotViolation(t *testing.T) {
	r := &lang.RustRules{}
	if r.IsVerblessViolation("Direction", "enum") {
		t.Error("Rust enum should not be a violation")
	}
}

func TestRustRules_VerbFunctionIsNotViolation(t *testing.T) {
	r := &lang.RustRules{}
	names := []string{"get_user", "set_config", "new_connection", "try_parse", "from_str", "into_inner"}
	for _, name := range names {
		if r.IsVerblessViolation(name, "function") {
			t.Errorf("Rust function %q should not be a violation", name)
		}
	}
}

func TestRustRules_VerblessFunctionIsViolation(t *testing.T) {
	r := &lang.RustRules{}
	if !r.IsVerblessViolation("user_data", "function") {
		t.Error("Rust function without verb prefix should be a violation")
	}
}

func TestRustRules_MacroIsNotViolation(t *testing.T) {
	r := &lang.RustRules{}
	if r.IsVerblessViolation("println!", "function") {
		t.Error("Rust macro should not be a violation")
	}
}

func TestRustRules_StdlibIdioms(t *testing.T) {
	r := &lang.RustRules{}
	idioms := r.StdlibIdioms()
	required := []string{"io", "fs", "os", "fmt", "vec"}
	for _, abbr := range required {
		if !idioms[abbr] {
			t.Errorf("Rust stdlib idioms should include %s", abbr)
		}
	}
}
