package oculus

// Internal constants used by analyzer implementations.
const (
	nodeFuncDecl      = "function_declaration"
	nodeMethodDecl    = "method_declaration"
	nodeTypeID        = "type_identifier"
	nodeStructType    = "struct_type"
	nodeInterfaceType = "interface_type"
	nodePointerType   = "pointer_type"
	nodeQualifiedType = "qualified_type"
)

const (
	kindStruct    = "struct"
	kindInterface = "interface"
)

const (
	dirVendor   = "vendor"
	dirTestdata = "testdata"
)

const (
	extGo   = ".go"
	extRust = ".rs"
	extPy   = ".py"
	extTS   = ".ts"
	extJS   = ".js"
	extJava = ".java"
	extTSX  = ".tsx"
	extJSX  = ".jsx"
)

const pkgRoot = "(root)"
