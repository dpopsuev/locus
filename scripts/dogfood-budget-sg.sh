#!/usr/bin/env bash
# Dogfood: unified budgeted symbol-graph path against alef.
# Asserts interactive scenario <10s, no Set.add→engine.add false edge,
# and quality_tier provenance on the response.
set -euo pipefail

REPO="${ALEF_ROOT:-${HOME}/Workspace/alef}"
BASE="${LOCUS_MCP_URL:-http://127.0.0.1:8081/mcp}"
SYMBOL="${ALEF_SCENARIO_SYMBOL:-applySessionMetadataRefresh}"

INIT=$(curl -sS -D /tmp/budget-sg-hdrs.txt -X POST "$BASE" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"budget-sg-dogfood","version":"1"}}}')
SID=$(grep -i 'Mcp-Session-Id:' /tmp/budget-sg-hdrs.txt | awk '{print $2}' | tr -d '\r')
H=(-H "Content-Type: application/json" -H "Accept: application/json, text/event-stream" -H "Mcp-Session-Id: $SID")
curl -sS -X POST "$BASE" "${H[@]}" -d '{"jsonrpc":"2.0","method":"notifications/initialized"}' -o /dev/null

mcp() {
  local id=$1 name=$2 args=$3
  curl -sS --max-time 180 -X POST "$BASE" "${H[@]}" \
    -d "{\"jsonrpc\":\"2.0\",\"id\":$id,\"method\":\"tools/call\",\"params\":{\"name\":\"$name\",\"arguments\":$args}}"
}

echo "=== scan_local $REPO ==="
SCAN=$(mcp 2 codograph "{\"action\":\"scan_local\",\"path\":\"$REPO\",\"intent\":\"full\",\"scanner\":\"typescript\",\"format\":\"summary\"}")
CK=$(echo "$SCAN" | tr '\n' ' ' | grep -oE "${REPO}@[a-f0-9]+(-[a-z]+)?" | head -1)
echo "CK=$CK"
[[ -n "$CK" ]] || { echo "FAIL: no cache_key"; echo "$SCAN" | head -c 800; exit 1; }

check_scenario() {
  local label=$1 args=$2 max_ms=$3
  echo "=== scenario $label ==="
  local start elapsed scen
  start=$(date +%s%3N)
  scen=$(mcp 3 analysis "$args")
  elapsed=$(( $(date +%s%3N) - start ))
  python3 - "$scen" "$elapsed" "$label" "$max_ms" <<'PY'
import json, sys
raw, elapsed, label, max_ms = sys.argv[1], int(sys.argv[2]), sys.argv[3], int(sys.argv[4])
if "data:" in raw:
    parts = [line[5:].strip() for line in raw.splitlines() if line.startswith("data:")]
    raw = "\n".join(parts) if parts else raw
msg = json.loads(raw)
result = msg.get("result") or msg
if result.get("isError"):
    raise SystemExit(f"FAIL {label}: isError {json.dumps(result)[:600]}")
text = "".join(c.get("text", "") for c in (result.get("content") or []) if c.get("type") == "text")
data = json.loads(text)
blob = json.dumps(data)
qt = data.get("quality_tier")
print(f"[{label}] elapsed_ms={elapsed} quality_tier={qt!r} entry_scoped={data.get('entry_scoped')} "
      f"focus_entry={data.get('focus_entry')!r} cg_winner={data.get('cg_winner')!r} "
      f"ast_ms={data.get('ast_ms')} lsp_ms={data.get('lsp_ms')}")
if "engine.add" in blob:
    raise SystemExit(f"FAIL {label}: engine.add false edge present")
if qt not in ("ast", "ast+lsp", "ast+partial_lsp"):
    raise SystemExit(f"FAIL {label}: missing quality_tier, keys={sorted(data)}")
if not data.get("entry_scoped") and not data.get("focus_entry"):
    raise SystemExit(f"FAIL {label}: expected entry focus provenance")
if elapsed >= max_ms:
    raise SystemExit(f"FAIL {label}: {elapsed}ms >= {max_ms}ms")
print(f"[{label}] PASS")
PY
}

check_scenario default \
  "{\"action\":\"scenario\",\"symbol\":\"$SYMBOL\",\"cache_key\":\"$CK\",\"hops\":3}" \
  10000

check_scenario deep \
  "{\"action\":\"scenario\",\"symbol\":\"$SYMBOL\",\"cache_key\":\"$CK\",\"hops\":3,\"quality\":\"deep\"}" \
  15000

echo "budget-sg dogfood OK (cache_key=$CK)"
