package model_test

import (
	"testing"

	"github.com/dpopsuev/locus/internal/model"
)

func TestSymbolsFromNames_External(t *testing.T) {
	syms := model.SymbolsFromNames("Foo", "Bar")
	if len(syms) != 2 {
		t.Errorf("got %d symbols, want 2", len(syms))
	}
}
