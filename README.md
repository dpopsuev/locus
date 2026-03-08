# Locus

Spatial context bus for AI agents. Point it at any repository and get architecture, dependency graph, churn, hot spots, and symbols — via CLI or MCP server.

## Quickstart (container)

```bash
docker run -d --name locus \
  -p 8081:8081 \
  -v locus-data:/data \
  quay.io/dpopsuev/locus
```

## Quickstart (binary)

```bash
go install github.com/dpopsuev/locus/cmd/locus@latest
locus serve                           # stdio (Cursor/Claude)
locus serve --transport http          # HTTP on :8081
```

## Cursor MCP configuration

### stdio (local binary)

```json
{
  "mcpServers": {
    "locus": {
      "command": "locus",
      "args": ["serve"]
    }
  }
}
```

### HTTP (container)

```json
{
  "mcpServers": {
    "locus": {
      "url": "http://localhost:8081/"
    }
  }
}
```

## Environment variables

| Variable | Default | Description |
|---|---|---|
| `LOCUS_CACHE_DIR` | `~/.locus/cache` | Scan cache directory |
| `LOCUS_HISTORY_DIR` | `~/.locus/history` | Codograph history directory |
| `LOCUS_TRANSPORT` | `stdio` | Transport: `stdio`, `http` |
| `LOCUS_ADDR` | `:8081` | Listen address (http only) |

## MCP tools

| Tool | Description |
|---|---|
| `scan_project` | Full codebase context: architecture, deps, churn, symbols |
| `suggest_depth` | Optimal `--depth` grouping level for a repo |
| `get_hot_spots` | High fan-in + high churn components |
| `get_dependencies` | Fan-in/fan-out edges for a component |
| `get_coupling_table` | Package coupling: fan-in, fan-out, churn, symbols |
| `get_edge_list` | Dependency edge list, optional component filter |
| `get_rules` | `.cursor/rules` for a workspace |
| `get_skills` | `.cursor/skills` for a workspace |
| `codograph_remote` | Scan a remote GitHub repo via shallow clone |
| `get_codograph_history` | Past codographs for a repo |
| `diff_codographs` | Diff between two most recent codographs |
| `diff_branches` | Architecture diff between two git branches |
