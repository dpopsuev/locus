package lang

// DefaultLSPServers maps languages to their conventional LSP server commands.
var DefaultLSPServers = map[Language]string{
	Go:         "gopls serve",
	Rust:       "rust-analyzer",
	Python:     "pyright-langserver --stdio",
	TypeScript: "typescript-language-server --stdio",
	C:          "clangd",
	Cpp:        "clangd",
	Java:       "jdtls",
	Kotlin:     "kotlin-language-server",
	CSharp:     "omnisharp",
	Swift:      "sourcekit-lsp",
	Zig:        "zls",
}

// DefaultLSPServer returns the conventional LSP server command for a language.
func DefaultLSPServer(l Language) string {
	return DefaultLSPServers[l]
}
