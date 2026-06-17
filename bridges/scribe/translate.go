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
				"loc":   svc.LOC,
				"churn": svc.Churn,
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

func componentID(project, name string) string {
	slug := strings.ReplaceAll(strings.ToLower(name), " ", "-")
	return fmt.Sprintf("%s/%s", project, slug)
}
