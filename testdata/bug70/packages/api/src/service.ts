// packages/api/src namespace — top of the cycle.
// TypeScript allows circular imports; this one is intentional for LCS-BUG-70.
// At grouping depth=2 both packages/api/src and packages/api/src/core collapse
// into the single component "packages/api", so this edge is dropped by
// buildGroupEdges (fromGroup == toGroup) and DetectCycles sees 0 cycles.
import { Handler } from './core/handler';

export class Service {
  private handler = new Handler();
}
