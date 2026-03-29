package smells

import (
	"testkit/go/adapter"
	"testkit/go/domain"
	"testkit/go/patterns"
)

// Cfg is a known abbreviation that should be flagged by symbol quality.
var Cfg = "config_value"

// Manager is a generic name that should be flagged.
var Manager = "default"

// GodStruct has too many fields and responsibilities — triggers god_component smell.
type GodStruct struct {
	Field1, Field2, Field3, Field4, Field5     string
	Field6, Field7, Field8, Field9, Field10    string
	Field11, Field12, Field13, Field14, Field15 string
	Field16, Field17, Field18, Field19, Field20 string
	Field21, Field22, Field23, Field24, Field25 string
	Field26, Field27, Field28, Field29, Field30 string
	Field31, Field32                            string
	Repo    domain.Repository
	Bus     patterns.EventBus
	Handler func()
}

// DoEverything demonstrates feature envy — touches many packages.
func (g *GodStruct) DoEverything() {
	_ = adapter.NewPostgresRepo()
	_ = patterns.NewAlphaStrategy()
	_ = patterns.NewWidget("x")
	_ = g.Repo
	g.Bus.Notify("done")
}
