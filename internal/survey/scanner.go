package survey

import "github.com/dpopsuev/locus/internal/model"

// Scanner extracts structural metadata from source code.
type Scanner interface {
	Scan(root string) (*model.Project, error)
}
