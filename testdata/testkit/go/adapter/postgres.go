package adapter

import "testkit/go/domain"

// PostgresRepo implements domain.Repository with a fake postgres backend.
type PostgresRepo struct {
	data map[string]*domain.Entity
}

// NewPostgresRepo creates a new PostgresRepo.
func NewPostgresRepo() *PostgresRepo {
	return &PostgresRepo{data: make(map[string]*domain.Entity)}
}

// FindByID retrieves an entity from the fake store.
func (r *PostgresRepo) FindByID(id string) (*domain.Entity, error) {
	e, ok := r.data[id]
	if !ok {
		return nil, nil
	}
	return e, nil
}

// Save stores an entity in the fake store.
func (r *PostgresRepo) Save(entity *domain.Entity) error {
	r.data[entity.ID] = entity
	return nil
}
