// Package scribe translates Locus architecture data into Battery canonical Records.
package scribe

import (
	"fmt"
	"strings"

	"github.com/dpopsuev/battery/translate"
	oculus "github.com/dpopsuev/oculus/v3"
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

// TranslateScanWithSymbols extends TranslateScan to also emit symbol-level
// records (code.interface, code.test) linked to their parent component.
func TranslateScanWithSymbols(report *oculus.ContextReport, project string) translate.Result {
	result := TranslateScan(report, project)

	sourceLabel := "source:locus"
	projectLabel := "project:" + project

	for _, svc := range report.Architecture.Services {
		compID := componentID(project, svc.Name)
		for _, sym := range svc.Symbols {
			if !sym.Exported {
				continue
			}
			kind := kindCodeInterface
			symID := symbolID(project, svc.Name, sym.Name)
			r := translate.Record{
				ID:     symID,
				Kind:   kind,
				Title:  sym.Name,
				Labels: []string{sourceLabel, projectLabel, "symbol:" + sym.Kind.String()},
				Extra: map[string]any{
					"ref_backend":  "locus",
					"ref_id":       symID,
					"symbol_kind":  sym.Kind.String(),
					"component":    svc.Name,
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
	return result
}

const kindCodeInterface = "code.interface"

func symbolID(project, component, name string) string {
	comp := strings.ReplaceAll(strings.ToLower(component), " ", "-")
	sym := strings.ReplaceAll(strings.ToLower(name), " ", "-")
	return fmt.Sprintf("%s/%s:%s", project, comp, sym)
}

func componentID(project, name string) string {
	slug := strings.ReplaceAll(strings.ToLower(name), " ", "-")
	return fmt.Sprintf("%s/%s", project, slug)
}
