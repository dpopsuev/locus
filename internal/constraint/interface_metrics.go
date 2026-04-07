package constraint

import (
	"fmt"
	"sort"

	"github.com/dpopsuev/oculus"
)

// InterfaceMetric holds metrics for a single interface type.
type InterfaceMetric struct {
	Name         string   `json:"name"`
	Package      string   `json:"package"`
	MethodCount  int      `json:"method_count"`
	Implementors []string `json:"implementors"`
	IsOrphan     bool     `json:"is_orphan"`
}

// InterfaceMetricsReport holds aggregate interface metrics for a codebase.
type InterfaceMetricsReport struct {
	Interfaces   []InterfaceMetric `json:"interfaces"`
	TotalOrphans int               `json:"total_orphans"`
	AvgSize      float64           `json:"avg_method_count"`
	LargestIface string            `json:"largest_interface"`
	Summary      string            `json:"summary"`
}

// ComputeInterfaceMetrics analyzes interfaces and their implementors.
func ComputeInterfaceMetrics(classes []oculus.ClassInfo, impls []oculus.ImplEdge) *InterfaceMetricsReport {
	// Step 1: Filter interfaces from ClassInfo.
	ifaceMap := make(map[string]*InterfaceMetric)
	for _, c := range classes {
		if c.Kind != "interface" {
			continue
		}
		ifaceMap[c.Name] = &InterfaceMetric{
			Name:        c.Name,
			Package:     c.Package,
			MethodCount: len(c.Methods),
		}
	}

	// Step 2: Find implementors from ImplEdge list.
	for _, edge := range impls {
		if edge.Kind != "implements" {
			continue
		}
		iface, ok := ifaceMap[edge.To]
		if !ok {
			continue
		}
		iface.Implementors = append(iface.Implementors, edge.From)
	}

	// Step 3: Build sorted slice, mark orphans, compute aggregates.
	metrics := make([]InterfaceMetric, 0, len(ifaceMap))
	for _, m := range ifaceMap {
		sort.Strings(m.Implementors)
		m.IsOrphan = len(m.Implementors) == 0
		metrics = append(metrics, *m)
	}
	sort.Slice(metrics, func(i, j int) bool { return metrics[i].Name < metrics[j].Name })

	totalOrphans := 0
	totalMethods := 0
	largestName := ""
	largestCount := 0

	for _, m := range metrics {
		if m.IsOrphan {
			totalOrphans++
		}
		totalMethods += m.MethodCount
		if m.MethodCount > largestCount {
			largestCount = m.MethodCount
			largestName = m.Name
		}
	}

	var avgSize float64
	if len(metrics) > 0 {
		avgSize = float64(totalMethods) / float64(len(metrics))
	}

	summary := fmt.Sprintf("%d interface(s), %d orphan(s), avg %.1f methods, largest: %s (%d methods)",
		len(metrics), totalOrphans, avgSize, largestName, largestCount)

	return &InterfaceMetricsReport{
		Interfaces:   metrics,
		TotalOrphans: totalOrphans,
		AvgSize:      avgSize,
		LargestIface: largestName,
		Summary:      summary,
	}
}
