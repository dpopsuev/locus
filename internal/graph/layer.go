package graph

// LayerViolation flags an edge where a lower-layer node imports from a higher layer.
type LayerViolation struct {
	From      string `json:"from"`
	To        string `json:"to"`
	FromLayer string `json:"from_layer"`
	ToLayer   string `json:"to_layer"`
}

// CheckLayerPurity detects edges where a node in a lower layer imports from a
// higher layer. layers is ordered from bottom (index 0) to top (index N-1).
func CheckLayerPurity[E Edge](edges []E, layers []string) []LayerViolation {
	if len(layers) == 0 {
		return nil
	}
	rank := make(map[string]int, len(layers))
	for i, l := range layers {
		rank[l] = i
	}

	var violations []LayerViolation
	for _, e := range edges {
		fromRank, fromOK := rank[e.Source()]
		toRank, toOK := rank[e.Target()]
		if !fromOK || !toOK {
			continue
		}
		if fromRank < toRank {
			violations = append(violations, LayerViolation{
				From:      e.Source(),
				To:        e.Target(),
				FromLayer: e.Source(),
				ToLayer:   e.Target(),
			})
		}
	}
	return violations
}
