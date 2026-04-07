package config_test

import (
	"testing"

	"github.com/dpopsuev/locus/internal/config"
)

func TestNewStore_External(t *testing.T) {
	s := config.NewStore()
	if s == nil {
		t.Fatal("nil store")
	}
}
