package survey

import (
	"regexp"
	"strings"

	"github.com/dpopsuev/locus/internal/model"
)

// nsRoot is the namespace key used for files at the project root directory.
const nsRoot = "(root)"

// Scanner extracts structural metadata from source code.
type Scanner interface {
	Scan(root string) (*model.Project, error)
}

// symbolPattern pairs a compiled regex with the SymbolKind it detects.
type symbolPattern struct {
	re   *regexp.Regexp
	kind model.SymbolKind
}

// matchSymbolPatterns tries each pattern against line and adds the first
// match as a symbol to ns. Returns true if a match was found.
func matchSymbolPatterns(line string, patterns []symbolPattern, ns *model.Namespace, seen map[string]bool, exported bool) {
	for _, p := range patterns {
		m := p.re.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name := m[1]
		if seen[name] {
			return
		}
		seen[name] = true
		exp := exported
		if !exported {
			exp = !strings.HasPrefix(name, "_")
		}
		ns.AddSymbol(&model.Symbol{
			Name:     name,
			Kind:     p.kind,
			Exported: exp,
		})
		return
	}
}
