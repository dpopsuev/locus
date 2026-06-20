// Package scribe translates Locus architecture data into Battery canonical Records.
package scribe

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/dpopsuev/battery/translate"
	oculus "github.com/dpopsuev/oculus/v3"
	"github.com/dpopsuev/oculus/v3/model"
)

// TranslateScan converts a Locus scan result into canonical Records + Edges.
func TranslateScan(report *oculus.ContextReport, project string) translate.Result {
	var result translate.Result

	sourceLabel := "source:locus"
	projectLabel := "project:" + project

	for _, svc := range report.Architecture.Services {
		id := componentID(project, svc.Name)
		r := translate.Record{
			ID:     id,
			Kind:   "knowledge.source",
			Title:  svc.Name,
			Labels: []string{sourceLabel, projectLabel},
			Sections: []translate.Section{
				{Name: "package", Text: svc.Package},
				{Name: "language", Text: svc.Language.String()},
			},
			Extra: map[string]any{
				"ref_backend": "locus",
				"ref_id":      id,
				"loc":         svc.LOC,
				"churn":       svc.Churn,
			},
		}
		if depth, ok := report.ImportDepth[svc.Name]; ok {
			r.Extra["layer_depth"] = depth
		}
		if svc.TrustZone != "" {
			r.Labels = append(r.Labels, "zone:"+svc.TrustZone)
		}
		result.Records = append(result.Records, r)
	}

	for _, edge := range report.Architecture.Edges {
		result.Edges = append(result.Edges, translate.Edge{
			From:     componentID(project, edge.From),
			Relation: "depends_on",
			To:       componentID(project, edge.To),
		})
	}

	return result
}

// TranslateScanWithSymbols extends TranslateScan with symbol-level records
// from the SymbolGraph. When sg is non-nil, symbols and edges come from the
// full SymbolGraph (including private symbols and inter-symbol edges).
// When sg is nil, falls back to the exported-only symbols from ArchService.
func TranslateScanWithSymbols(report *oculus.ContextReport, sg *oculus.SymbolGraph, project string) translate.Result {
	result := TranslateScan(report, project)

	if sg != nil {
		translateSymbolGraph(&result, report, sg, project)
		return result
	}

	translateArchSymbols(&result, report, project)
	return result
}

// translateSymbolGraph emits records and edges from a full SymbolGraph.
func translateSymbolGraph(result *translate.Result, report *oculus.ContextReport, sg *oculus.SymbolGraph, project string) {
	sourceLabel := "source:locus"
	projectLabel := "project:" + project
	pkgToComponent := buildPkgToComponent(report, project)

	fileNodes := translateFiles(result, report, project, sourceLabel, projectLabel, pkgToComponent)

	for _, sym := range sg.Nodes {
		symID := symbolIDFromFQN(project, symbolFQN(sym.Package, sym.Name))
		r := buildSymbolRecord(sym, symID, sourceLabel, projectLabel)
		result.Records = append(result.Records, r)

		if sym.File != "" {
			if fileID, ok := fileNodes[sym.File]; ok {
				result.Edges = append(result.Edges, translate.Edge{
					From: fileID, Relation: "contains", To: symID,
				})
				continue
			}
		}
		if compID, ok := pkgToComponent[sym.Package]; ok {
			result.Edges = append(result.Edges, translate.Edge{
				From: compID, Relation: "contains", To: symID,
			})
		}
	}

	for _, edge := range sg.Edges {
		if rel := mapEdgeKind(edge.Kind); rel != "" {
			result.Edges = append(result.Edges, translate.Edge{
				From:     symbolIDFromFQN(project, edge.SourceFQN),
				Relation: rel,
				To:       symbolIDFromFQN(project, edge.TargetFQN),
			})
		}
	}
}

// translateFiles emits code.file records from the Project's namespace file
// lists. Returns a map from file path to Scribe file ID for symbol→file
// edge wiring.
func translateFiles(result *translate.Result, report *oculus.ContextReport, project, sourceLabel, projectLabel string, pkgToComponent map[string]string) map[string]string {
	if report.Project == nil {
		return nil
	}
	fileNodes := make(map[string]string)
	seen := make(map[string]bool)

	for _, ns := range report.Project.Namespaces {
		compID := pkgToComponent[ns.ImportPath]
		if compID == "" {
			compID = pkgToComponent[ns.Name]
		}
		for _, f := range ns.Files {
			if seen[f.Path] {
				continue
			}
			seen[f.Path] = true

			fileID := fileIDFromPath(project, f.Path)
			fileNodes[f.Path] = fileID

			r := translate.Record{
				ID:     fileID,
				Kind:   "code.file",
				Title:  filepath.Base(f.Path),
				Labels: []string{sourceLabel, projectLabel},
				Extra: map[string]any{
					"ref_backend": "locus",
					"ref_id":      fileID,
					"path":        f.Path,
					"package":     f.Package,
					"lines":       f.Lines,
				},
			}
			result.Records = append(result.Records, r)

			if compID != "" {
				result.Edges = append(result.Edges, translate.Edge{
					From: compID, Relation: "contains", To: fileID,
				})
			}
		}
	}
	return fileNodes
}

func buildSymbolRecord(sym oculus.Symbol, symID, sourceLabel, projectLabel string) translate.Record {
	visibility := "private"
	if sym.Exported {
		visibility = "public"
	}
	r := translate.Record{
		ID:    symID,
		Kind:  mapSymbolKind(sym.Kind),
		Title: sym.Name,
		Labels: []string{
			sourceLabel, projectLabel,
			"symbol:" + sym.Kind,
			"visibility:" + visibility,
		},
		Extra: map[string]any{
			"ref_backend": "locus",
			"ref_id":      symID,
			"symbol_kind": sym.Kind,
			"package":     sym.Package,
			"exported":    sym.Exported,
		},
	}
	if sym.File != "" {
		r.Extra["file"] = sym.File
		r.Extra["line"] = sym.Line
	}
	if sym.EndLine > 0 {
		r.Extra["end_line"] = sym.EndLine
	}
	if sym.Signature != "" {
		r.Sections = append(r.Sections, translate.Section{Name: "signature", Text: sym.Signature})
	}
	if len(sym.ParamTypes) > 0 {
		r.Sections = append(r.Sections, translate.Section{Name: "param_types", Text: strings.Join(sym.ParamTypes, ", ")})
	}
	if len(sym.ReturnTypes) > 0 {
		r.Sections = append(r.Sections, translate.Section{Name: "return_types", Text: strings.Join(sym.ReturnTypes, ", ")})
	}
	if sym.ReceiverType != "" {
		r.Extra["receiver_type"] = sym.ReceiverType
	}
	return r
}

// translateArchSymbols is the legacy path: emit exported-only symbols from
// ArchService when no SymbolGraph is available.
func translateArchSymbols(result *translate.Result, report *oculus.ContextReport, project string) {
	sourceLabel := "source:locus"
	projectLabel := "project:" + project

	for _, svc := range report.Architecture.Services {
		compID := componentID(project, svc.Name)
		for _, sym := range svc.Symbols {
			symID := symbolID(project, svc.Name, sym.Name)
			visibility := "private"
			if sym.Exported {
				visibility = "public"
			}
			r := translate.Record{
				ID:     symID,
				Kind:   mapModelSymbolKind(sym.Kind),
				Title:  sym.Name,
				Labels: []string{sourceLabel, projectLabel, "symbol:" + sym.Kind.String(), "visibility:" + visibility},
				Extra: map[string]any{
					"ref_backend": "locus",
					"ref_id":      symID,
					"symbol_kind": sym.Kind.String(),
					"component":   svc.Name,
					"exported":    sym.Exported,
				},
			}
			if sym.File != "" {
				r.Extra["file"] = sym.File
				r.Extra["line"] = sym.Line
			}
			if sym.Signature != "" {
				r.Sections = append(r.Sections, translate.Section{
					Name: "signature", Text: sym.Signature,
				})
			}
			result.Records = append(result.Records, r)
			result.Edges = append(result.Edges, translate.Edge{
				From:     compID,
				Relation: "contains",
				To:       symID,
			})
		}
	}
}

const (
	kindInterface = "code.interface"
	kindStruct    = "code.struct"
	kindFunction  = "code.function"
	kindMethod    = "code.method"

	relCalls      = "calls"
	relImplements = "implements"
	relEmbeds     = "embeds"
	relFieldRef   = "field_ref"
)

var symbolKindMap = map[string]string{
	"interface": kindInterface,
	"struct":    kindStruct,
	"class":     kindStruct,
	"function":  kindFunction,
	"method":    kindMethod,
}

var modelSymbolKindMap = map[model.SymbolKind]string{
	model.SymbolInterface: kindInterface,
	model.SymbolStruct:    kindStruct,
	model.SymbolClass:     kindStruct,
	model.SymbolFunction:  kindFunction,
	model.SymbolMethod:    kindMethod,
}

var edgeKindMap = map[string]string{
	"call":          relCalls,
	"implements":    relImplements,
	"extends":       relImplements,
	"embeds":        relEmbeds,
	"field_ref":     relFieldRef,
	"goroutine":     relCalls,
	"channel_send":  relCalls,
	"channel_recv":  relCalls,
	"await_call":    relCalls,
	"promise_chain": relCalls,
	"task_spawn":    relCalls,
}

func mapSymbolKind(kind string) string {
	if k, ok := symbolKindMap[kind]; ok {
		return k
	}
	return kindFunction
}

func mapModelSymbolKind(kind model.SymbolKind) string {
	if k, ok := modelSymbolKindMap[kind]; ok {
		return k
	}
	return kindFunction
}

func mapEdgeKind(kind string) string {
	return edgeKindMap[kind]
}

// buildPkgToComponent creates a mapping from package import path to component
// Scribe ID, derived from the architecture services.
func buildPkgToComponent(report *oculus.ContextReport, project string) map[string]string {
	m := make(map[string]string)
	for _, svc := range report.Architecture.Services {
		if svc.Package != "" {
			m[svc.Package] = componentID(project, svc.Name)
		}
		m[svc.Name] = componentID(project, svc.Name)
	}
	return m
}

// symbolFQN builds a fully qualified name from package and symbol name.
func symbolFQN(pkg, name string) string {
	name = strings.TrimPrefix(name, "*")
	if pkg == "" {
		return name
	}
	return pkg + "." + name
}

// symbolIDFromFQN normalizes a symbol FQN into a Scribe artifact ID.
// Example: "github.com/dpopsuev/scribe/service.Protocol.Create" → "scribe/service:protocol.create"
func symbolIDFromFQN(project, fqn string) string {
	parts := strings.SplitN(fqn, ".", 2)
	if len(parts) < 2 {
		return fmt.Sprintf("%s/_:%s", project, slug(fqn))
	}
	pkg := parts[0]
	name := parts[1]

	pkgShort := pkg
	if idx := strings.LastIndex(pkg, "/"); idx >= 0 {
		prefix := strings.TrimSuffix(pkg[:idx], "/")
		if slashIdx := strings.LastIndex(prefix, "/"); slashIdx >= 0 {
			pkgShort = prefix[slashIdx+1:] + "/" + pkg[idx+1:]
		} else {
			pkgShort = pkg[idx+1:]
		}
	}

	return fmt.Sprintf("%s/%s:%s", project, slug(pkgShort), slug(name))
}

func symbolID(project, component, name string) string {
	return fmt.Sprintf("%s/%s:%s", project, slug(component), slug(name))
}

func componentID(project, name string) string {
	return fmt.Sprintf("%s/%s", project, slug(name))
}

// fileIDFromPath builds a Scribe ID for a file node.
// Example: "service/handler.go" → "proj/service:handler.go"
func fileIDFromPath(project, path string) string {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	if dir == "." || dir == "" {
		return fmt.Sprintf("%s/_:%s", project, slug(base))
	}
	return fmt.Sprintf("%s/%s:%s", project, slug(dir), slug(base))
}

func slug(s string) string {
	return strings.ReplaceAll(strings.ToLower(s), " ", "-")
}
