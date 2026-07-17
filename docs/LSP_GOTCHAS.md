# LSP GOTCHAS (outside core graph)

Quirks that affect Locus/Oculus agent LSP tools (`resolve`, `definition`,
`references`, `show`, `rename`). Keep this out of `arch/` / scan pipelines —
it is operational knowledge for agents and humans, not graph structure.

## Product invariants

- **show ≠ definition.** `show` is `documentSymbol` body + outline. `definition`
  is a type-accurate jump (`textDocument/definition`). Do not substitute one
  for the other.
- **rename is dry-run by default.** Always inspect the plan + `coverage_ok`
  before `apply:true` / `--apply`. Apply rebinds the graph (`MarkDirty` + SG flush).

## Locators

- Prefer precise forms when names collide: `path:Symbol` or `path:line:Symbol`.
- Ambiguous bare `Symbol` returns escalations / candidates — do not guess; pick
  a pivot and retry.

## WarmLSP / pool

- CLI and MCP analysis without a pool degrade to “unavailable” summaries (not
  hard errors). `locus serve` and `locus definition|references|show|rename`
  start a real pool.
- Parent workspace + CWD under a sibling project: default project binding uses
  process CWD when workspace roots miss — still pass `path=` / `cache_key=`
  when you know the target.
- **Capacity:** default `MaxActive=3` (`LOCUS_LSP_MAX_ACTIVE`). When full, the
  pool **evicts the LRU** idle server so a new workspace can be admitted
  (admit-time eviction). Do not restart the container just to clear
  `lsp pool: at capacity` after multi-repo dogfood — retry once; if it still
  fails, check for stuck dead servers or raise `LOCUS_LSP_MAX_ACTIVE`.
- Idle TTL reaping still runs (default 30m) for long-idle cleanup; eviction on
  admit is what makes serial multi-repo sessions work.

## Rename coverage gate

- Refs spanning multiple files require a multi-file `WorkspaceEdit`.
- Edit site count must not under-cover reference site count.
- Gate failure returns the plan with `coverage_ok:false` and does **not** apply.
- Some servers (notably intermittent single-file mode) under-cover until
  re-indexed; retry after warm / reopen is expected — Locus surfaces the gate
  rather than silently applying a partial rename.

## Language servers

### gopls

- Needs at least one `didOpen` (and usually a module root) before useful
  definition/references/rename.
- Build tags / generated code can empty or skew refs; verify with
  `references` before trusting rename coverage.

### rust-analyzer

- Cold start / indexing delay: early requests may return empty. Prefer warm
  pool (`locus serve`) over one-shot CLI for large crates.
- `file_granularity` scans are per-`.rs` components — locators should use
  concrete paths, not crate package names alone.
- **Container image:** rustup installs a PATH *shim* at `cargo/bin/rust-analyzer`.
  The locus image must keep `RUSTUP_HOME` at the install home (not an empty
  `/tmp/rustup`) and/or PATH-prefer the real toolchain binary
  (`/usr/local/bin/rust-analyzer` symlink). Otherwise initialize fails with
  `Unknown binary 'rust-analyzer'` / EOF.

### Python / pyright

- Project root markers (`pyproject.toml`) select the server workspace.
- **Scanner paths:** `PythonScanner` stores repo-relative `Symbol.File`
  (e.g. `pkg/sub/mod.py`), not basename-only. Basename-only paths break
  resolve/show/rename against nested modules — prefer locators that include
  the nested path.

### TypeScript / pyright

- Project root markers (`tsconfig.json`, `pyproject.toml`) select the server
  workspace. Wrong root → empty symbols.
- Multi-package monorepos: pass the package path, not only the monorepo root,
  when resolving.
- **`file_granularity`:** FileLevel namespaces are file paths. Relative imports
  must resolve to concrete `.ts`/`.tsx`/index files (not parent directories),
  or the architecture graph keeps components with **0 edges**. Dir-level
  scans still use directory→directory import edges.
- **Foreign clones / missing `node_modules`:** `typescript-language-server`
  needs a workspace `typescript` install **or** `tsserver.path`. The container
  sets `LOCUS_TSSERVER_PATH` to the image-global `tsserver.js`. Locally: run
  `npm i` in the clone, or export `LOCUS_TSSERVER_PATH` to a global
  `…/typescript/lib/tsserver.js`. When LSP still fails, `analysis show` returns
  a **source excerpt** (not an empty unavailable body).
- **GJS / GNOME Shell:** `gi://` and `resource://` imports become ambient
  internal edges. Classic `imports.gi` / `imports.ui` forms are still
  unsupported — treat those graphs as inventories until expanded.

### clangd / C++

- Compile commands (`compile_commands.json`) dominate quality. Without them,
  prepareRename/rename are unreliable.
- Header/source pairs often need both open for cross-file rename coverage.

## Diagnostics vs navigation

- Navigation tools do not wait on `publishDiagnostics`. Do not treat missing
  diagnostics as evidence that definition/references failed.
- Progress (`$/progress`) is a hint, not a completion signal for rename.

## After apply

- Disk mutation dirties Merkle freshness and flushes the symbol-graph cache.
- Next analysis should `scan_local` (or rely on watcher-driven rescan) before
  trusting FQNs that were renamed.
