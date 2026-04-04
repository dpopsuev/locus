package analysis

import "github.com/dpopsuev/locus/internal/oculus"

// Type aliases re-exported from oculus. Consumers should migrate to
// importing oculus directly. These aliases exist for gradual migration
// and will be removed when all consumers are updated.

type ClassInfo = oculus.ClassInfo
type FieldInfo = oculus.FieldInfo
type MethodInfo = oculus.MethodInfo
type ImplEdge = oculus.ImplEdge
type FieldRef = oculus.FieldRef
type Call = oculus.Call
type EntryPoint = oculus.EntryPoint
type NestingResult = oculus.NestingResult
