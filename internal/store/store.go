// Package store provides concrete storage adapters (FilesystemStore, LRUStore).
// Domain types and port interfaces live in internal/port (DIP compliance).
// This package re-exports port types as aliases for backward compatibility.
package store

import "github.com/dpopsuev/oculus/v3/port"

// Type aliases — re-export port types for backward compatibility.
// Consumers should migrate to importing port directly over time.
type (
	ReportStore       = port.ReportStore
	HistoryStore      = port.HistoryStore
	GitResolver       = port.GitResolver
	ProjectStore      = port.ProjectStore
	ComponentStore    = port.ComponentStore
	DesiredStateStore = port.DesiredStateStore
	Store             = port.Store

	ProjectInfo       = port.ProjectInfo
	ComponentMeta     = port.ComponentMeta
	DesiredState      = port.DesiredState
	AcceptedViolation = port.AcceptedViolation
	BoundaryRule      = port.BoundaryRule
	HealthConstraint  = port.HealthConstraint
	HistoryEntry      = port.HistoryEntry
)

// Compile-time checks — concrete adapters implement the Store port.
var (
	_ port.Store = (*FilesystemStore)(nil)
	_ port.Store = (*LRUStore)(nil)
)
