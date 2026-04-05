// Package lang provides language-agnostic language detection, LSP server
// mappings, and directory skip rules. Part of the Oculus symbol resolution
// library — zero dependency on Locus domain packages.
package lang

// Language identifies the programming language of a project.
type Language string

const (
	Unknown    Language = "unknown"
	Go         Language = "go"
	Rust       Language = "rust"
	Python     Language = "python"
	TypeScript Language = "typescript"
	C          Language = "c"
	Cpp        Language = "cpp"
	Java       Language = "java"
	JavaScript Language = "javascript"
	Zig        Language = "zig"
	Kotlin     Language = "kotlin"
	Swift      Language = "swift"
	CSharp     Language = "csharp"
	Lua        Language = "lua"
	Proto      Language = "proto"
	Shell      Language = "shell"
)
