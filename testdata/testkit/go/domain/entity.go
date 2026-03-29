package domain

// Entity is the core domain type.
type Entity struct {
	ID   string
	Name string
	Data map[string]string
}

// Repository is the port interface for data access.
type Repository interface {
	FindByID(id string) (*Entity, error)
	Save(entity *Entity) error
}

// Service orchestrates domain logic.
type Service struct {
	repo Repository
}

// NewService creates a new Service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// GetEntity retrieves an entity by ID.
func (s *Service) GetEntity(id string) (*Entity, error) {
	return s.repo.FindByID(id)
}
