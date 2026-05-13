// packages/api/src/core namespace — closes the cycle back to packages/api/src.
// Together with service.ts this forms:
//   packages/api/src → packages/api/src/core → packages/api/src
// locus_analysis cycles (ungrouped scan) surfaces this cycle.
// locus_codograph scan_local (grouped at depth=2) silently drops it.
import { Service } from '../service';

export class Handler {
  handle(s: Service): void {}
}
