package oculus

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ErrRegexCallChainNotSupported is returned when regex call chain analysis is attempted.
var ErrRegexCallChainNotSupported = errors.New("regex call chain: not supported")

// RegexAnalyzer is the last-resort analyzer. It uses simple regex patterns
// to extract type declarations from source files. Accuracy is ~40% but it
// never fails — it always returns partial results without errors.
type RegexAnalyzer struct{}

var (
	reGoStruct    = regexp.MustCompile(`(?m)^type\s+(\w+)\s+struct\s*\{`)
	reGoIface     = regexp.MustCompile(`(?m)^type\s+(\w+)\s+interface\s*\{`)
	reClass       = regexp.MustCompile(`(?m)^\s*(?:public\s+|private\s+|protected\s+)?(?:abstract\s+)?class\s+(\w+)(?:\s+extends\s+(\w+))?(?:\s+implements\s+([\w,\s]+))?`)
	reInterface   = regexp.MustCompile(`(?m)^\s*(?:public\s+)?interface\s+(\w+)(?:\s+extends\s+(\w+))?`)
	rePyClass     = regexp.MustCompile(`(?m)^class\s+(\w+)(?:\(([\w,\s]+)\))?:`)
	reRustStruct  = regexp.MustCompile(`(?m)^(?:pub\s+)?struct\s+(\w+)`)
	reRustTrait   = regexp.MustCompile(`(?m)^(?:pub\s+)?trait\s+(\w+)`)
	reRustImpl    = regexp.MustCompile(`(?m)^impl\s+(\w+)\s+for\s+(\w+)`)
	reTSClass     = regexp.MustCompile(`(?m)^\s*(?:export\s+)?class\s+(\w+)(?:\s+extends\s+(\w+))?(?:\s+implements\s+([\w,\s]+))?`)
	reTSInterface = regexp.MustCompile(`(?m)^\s*(?:export\s+)?interface\s+(\w+)`)
	reFunc        = regexp.MustCompile(`(?m)^func\s+(\w+)\s*\(`)
	reGoMain      = regexp.MustCompile(`(?m)^func\s+main\s*\(\s*\)`)
	reNestBlock   = regexp.MustCompile(`\b(if|for|switch|select|while|match|try)\b`)
)

func (a *RegexAnalyzer) Classes(root string) ([]ClassInfo, error) {
	var classes []ClassInfo
	walkSrcFiles(root, func(path, pkg string, content []byte) {
		text := string(content)
		ext := filepath.Ext(path)
		switch ext {
		case extGo:
			for _, m := range reGoStruct.FindAllStringSubmatch(text, -1) {
				classes = append(classes, ClassInfo{
					Name: m[1], Package: pkg, Kind: kindStruct,
					Exported: isExported(m[1]),
				})
			}
			for _, m := range reGoIface.FindAllStringSubmatch(text, -1) {
				classes = append(classes, ClassInfo{
					Name: m[1], Package: pkg, Kind: kindInterface,
					Exported: isExported(m[1]),
				})
			}
		case extJava:
			for _, m := range reClass.FindAllStringSubmatch(text, -1) {
				classes = append(classes, ClassInfo{
					Name: m[1], Package: pkg, Kind: "class",
					Exported: isExported(m[1]),
				})
			}
			for _, m := range reInterface.FindAllStringSubmatch(text, -1) {
				classes = append(classes, ClassInfo{
					Name: m[1], Package: pkg, Kind: kindInterface,
					Exported: true,
				})
			}
		case extPy:
			for _, m := range rePyClass.FindAllStringSubmatch(text, -1) {
				classes = append(classes, ClassInfo{
					Name: m[1], Package: pkg, Kind: "class",
					Exported: !strings.HasPrefix(m[1], "_"),
				})
			}
		case extRust:
			for _, m := range reRustStruct.FindAllStringSubmatch(text, -1) {
				classes = append(classes, ClassInfo{
					Name: m[1], Package: pkg, Kind: kindStruct,
					Exported: true,
				})
			}
			for _, m := range reRustTrait.FindAllStringSubmatch(text, -1) {
				classes = append(classes, ClassInfo{
					Name: m[1], Package: pkg, Kind: "trait",
					Exported: true,
				})
			}
		case extTS, extJS:
			for _, m := range reTSClass.FindAllStringSubmatch(text, -1) {
				classes = append(classes, ClassInfo{
					Name: m[1], Package: pkg, Kind: "class",
					Exported: true,
				})
			}
			for _, m := range reTSInterface.FindAllStringSubmatch(text, -1) {
				classes = append(classes, ClassInfo{
					Name: m[1], Package: pkg, Kind: kindInterface,
					Exported: true,
				})
			}
		}
	})
	return classes, nil
}

//nolint:gocyclo // multi-language regex matching requires branching per language
func (a *RegexAnalyzer) Implements(root string) ([]ImplEdge, error) {
	var edges []ImplEdge
	walkSrcFiles(root, func(path, pkg string, content []byte) {
		text := string(content)
		ext := filepath.Ext(path)
		switch ext {
		case extRust:
			for _, m := range reRustImpl.FindAllStringSubmatch(text, -1) {
				edges = append(edges, ImplEdge{From: m[2], To: m[1], Kind: "implements"})
			}
		case extJava:
			for _, m := range reClass.FindAllStringSubmatch(text, -1) {
				if m[2] != "" {
					edges = append(edges, ImplEdge{From: m[1], To: m[2], Kind: "extends"})
				}
				if m[3] != "" {
					for _, iface := range strings.Split(m[3], ",") {
						iface = strings.TrimSpace(iface)
						if iface != "" {
							edges = append(edges, ImplEdge{From: m[1], To: iface, Kind: "implements"})
						}
					}
				}
			}
		case extPy:
			for _, m := range rePyClass.FindAllStringSubmatch(text, -1) {
				if m[2] != "" {
					for _, parent := range strings.Split(m[2], ",") {
						parent = strings.TrimSpace(parent)
						if parent != "" && parent != "object" {
							edges = append(edges, ImplEdge{From: m[1], To: parent, Kind: "extends"})
						}
					}
				}
			}
		case extTS, extJS:
			for _, m := range reTSClass.FindAllStringSubmatch(text, -1) {
				if m[2] != "" {
					edges = append(edges, ImplEdge{From: m[1], To: m[2], Kind: "extends"})
				}
				if m[3] != "" {
					for _, iface := range strings.Split(m[3], ",") {
						iface = strings.TrimSpace(iface)
						if iface != "" {
							edges = append(edges, ImplEdge{From: m[1], To: iface, Kind: "implements"})
						}
					}
				}
			}
		}
	})
	return edges, nil
}

func (a *RegexAnalyzer) FieldRefs(root string) ([]FieldRef, error) {
	return nil, nil
}

func (a *RegexAnalyzer) CallChain(root, entry string, depth int) ([]Call, error) {
	return nil, ErrRegexCallChainNotSupported
}

func (a *RegexAnalyzer) EntryPoints(root string) ([]EntryPoint, error) {
	var entries []EntryPoint
	walkSrcFiles(root, func(path, pkg string, content []byte) {
		text := string(content)
		ext := filepath.Ext(path)
		if ext == extGo {
			if reGoMain.MatchString(text) {
				entries = append(entries, EntryPoint{
					Name: "main", Kind: "main", Package: pkg, File: path,
				})
			}
			for _, m := range reFunc.FindAllStringSubmatch(text, -1) {
				if strings.HasPrefix(m[1], "Test") {
					entries = append(entries, EntryPoint{
						Name: m[1], Kind: "test", Package: pkg, File: path,
					})
				}
			}
		}
	})
	return entries, nil
}

func (a *RegexAnalyzer) NestingDepth(root string) ([]NestingResult, error) {
	var results []NestingResult
	walkSrcFiles(root, func(path, pkg string, content []byte) {
		if filepath.Ext(path) != extGo {
			return
		}
		for _, m := range reFunc.FindAllStringSubmatchIndex(string(content), -1) {
			funcName := string(content[m[2]:m[3]])
			// Scan from function start to next function or EOF
			funcStart := m[0]
			funcEnd := len(content)
			nextFunc := reFunc.FindStringIndex(string(content[funcStart+1:]))
			if nextFunc != nil {
				funcEnd = funcStart + 1 + nextFunc[0]
			}
			body := string(content[funcStart:funcEnd])
			maxDepth := 0
			depth := 0
			for _, line := range strings.Split(body, "\n") {
				trimmed := strings.TrimSpace(line)
				if reNestBlock.MatchString(trimmed) {
					depth++
					if depth > maxDepth {
						maxDepth = depth
					}
				}
				if trimmed == "}" {
					if depth > 0 {
						depth--
					}
				}
			}
			results = append(results, NestingResult{
				Function: funcName, Package: pkg, MaxDepth: maxDepth,
			})
		}
	})
	return results, nil
}

func walkSrcFiles(root string, fn func(path, pkg string, content []byte)) {
	absRoot, _ := filepath.Abs(root)
	_ = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := d.Name()
			if base == dirVendor || base == dirTestdata || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(d.Name())
		switch ext {
		case extGo, extRust, extPy, extTS, extJS, extJava:
		default:
			return nil
		}
		if strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(absRoot, path)
		pkg := filepath.ToSlash(filepath.Dir(rel))
		if pkg == "." {
			pkg = pkgRoot
		}
		fn(path, pkg, content)
		return nil
	})
}
