package diagram

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dpopsuev/locus/internal/graph"
)

func renderLayers(in Input, _ Options) string {
	report := in.Report
	rt := in.ResolvedTheme

	depths := report.ImportDepth
	if depths == nil {
		depths = graph.ImportDepth(report.Architecture.Edges)
	}

	type layer struct {
		depth      int
		components []string
	}

	layerMap := make(map[int][]string)
	for i := range report.Architecture.Services {
		svc := &report.Architecture.Services[i]
		d := depths[svc.Name]
		layerMap[d] = append(layerMap[d], svc.Name)
	}

	layers := make([]layer, 0, len(layerMap))
	for d, comps := range layerMap {
		sort.Strings(comps)
		layers = append(layers, layer{depth: d, components: comps})
	}
	sort.Slice(layers, func(i, j int) bool { return layers[i].depth < layers[j].depth })

	violationSet := make(map[[2]string]bool)
	for _, v := range report.LayerViolations {
		violationSet[[2]string{v.From, v.To}] = true
	}

	var b strings.Builder
	b.WriteString(rt.InitDirective() + "\n")
	b.WriteString("block-beta\n")

	for _, l := range layers {
		label := fmt.Sprintf("Layer %d", l.depth)
		if l.depth == -1 {
			label = "Cycle"
		}
		id := fmt.Sprintf("layer_%d", l.depth)
		if l.depth == -1 {
			id = "layer_cycle"
		}

		cols := len(l.components)
		if cols < 1 {
			cols = 1
		}
		fmt.Fprintf(&b, "    columns %d\n", cols)
		fmt.Fprintf(&b, "    block:%s[\"%s\"]\n", id, label)
		for _, c := range l.components {
			cid := mermaidID(c)
			fmt.Fprintf(&b, "        %s[\"%s\"]\n", cid, c)
		}
		b.WriteString("    end\n")
	}

	if len(report.LayerViolations) > 0 {
		violationColor := rt.ShapeHex["violation_edge"]
		if violationColor.Stroke != "" {
			fmt.Fprintf(&b, "\n    classDef violation stroke:%s,stroke-width:2px\n", violationColor.Stroke)
		}
		b.WriteString("\n")
		for _, v := range report.LayerViolations {
			fromID := mermaidID(v.From)
			toID := mermaidID(v.To)
			label := fmt.Sprintf("%s->%s", v.FromLayer, v.ToLayer)
			fmt.Fprintf(&b, "    %s -- \"%s\" --> %s\n", fromID, label, toID)
		}
	}

	return b.String()
}
