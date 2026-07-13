#!/usr/bin/env bash
# Live dogfood for AX campaign: scan → hotspots → architecture_review → probe/scenario
# with cache_key only (empty path). Expects locus HTTP MCP on :8081.
set -euo pipefail

REPO="${OCULUS_FIXTURE:-${HOME}/Workspace/oculus}"
BASE="${LOCUS_MCP_URL:-http://127.0.0.1:8081/mcp}"
SYMBOL="${AX_PROBE_SYMBOL:-engine.Engine}"

INIT=$(curl -sS -D /tmp/ax-dogfood-hdrs.txt -X POST "$BASE" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"ax-dogfood","version":"1"}}}')
SID=$(grep -i 'Mcp-Session-Id:' /tmp/ax-dogfood-hdrs.txt | awk '{print $2}' | tr -d '\r')
H=(-H "Content-Type: application/json" -H "Accept: application/json, text/event-stream" -H "Mcp-Session-Id: $SID")
curl -sS -X POST "$BASE" "${H[@]}" -d '{"jsonrpc":"2.0","method":"notifications/initialized"}' -o /dev/null

mcp() {
  local id=$1 name=$2 args=$3
  curl -sS --max-time 120 -X POST "$BASE" "${H[@]}" \
    -d "{\"jsonrpc\":\"2.0\",\"id\":$id,\"method\":\"tools/call\",\"params\":{\"name\":\"$name\",\"arguments\":$args}}"
}

echo "=== status ==="
mcp 1 codograph '{"action":"status"}' | head -c 400 || true; echo

echo "=== scan_local $REPO ==="
SCAN=$(mcp 2 codograph "{\"action\":\"scan_local\",\"path\":\"$REPO\",\"intent\":\"full\"}")
CK=$(echo "$SCAN" | tr '\n' ' ' | grep -oE "${REPO}@[a-f0-9]+(-[a-z]+)?" | head -1)
echo "CK=$CK"
if [[ -z "$CK" ]]; then
  echo "FAIL: no cache_key"; echo "$SCAN" | head -c 800; exit 1
fi

echo "=== hot_spots (cache_key only) ==="
HS=$(mcp 3 analysis "{\"action\":\"coupling\",\"view\":\"hot_spots\",\"top_n\":15,\"cache_key\":\"$CK\"}")
echo "$HS" | head -c 600; echo
# SSE JSON escapes quotes as \", so match component without requiring raw ".
echo "$HS" | grep -q 'component' || { echo "FAIL: no hotspots"; exit 1; }

echo "=== architecture_review ==="
REV=$(mcp 4 analysis "{\"action\":\"preset\",\"preset\":\"architecture_review\",\"cache_key\":\"$CK\"}")
echo "$REV" | head -c 800; echo
echo "$REV" | grep -q 'Coupling' || { echo "FAIL: no Coupling"; exit 1; }
echo "$REV" | grep -q 'Hot Spots' || { echo "FAIL: no Hot Spots"; exit 1; }

echo "=== probe $SYMBOL ==="
START=$(date +%s)
PROBE=$(mcp 5 analysis "{\"action\":\"probe\",\"symbol\":\"$SYMBOL\",\"cache_key\":\"$CK\"}")
ELAPSED=$(( $(date +%s) - START ))
echo "elapsed=${ELAPSED}s"
echo "$PROBE" | head -c 1200; echo
echo "$PROBE" | grep -q '"isError":true' && { echo "FAIL: probe error"; exit 1; }
[[ "$ELAPSED" -lt 10 ]] || { echo "FAIL: probe ${ELAPSED}s >= 10s"; exit 1; }

echo "=== scenario ==="
START=$(date +%s)
SCEN=$(mcp 6 analysis "{\"action\":\"scenario\",\"symbol\":\"$SYMBOL\",\"cache_key\":\"$CK\"}")
ELAPSED=$(( $(date +%s) - START ))
echo "elapsed=${ELAPSED}s"
echo "$SCEN" | head -c 800; echo
echo "$SCEN" | grep -q '"isError":true' && { echo "FAIL: scenario error"; exit 1; }
[[ "$ELAPSED" -lt 10 ]] || { echo "FAIL: scenario ${ELAPSED}s >= 10s"; exit 1; }

echo "AX dogfood OK (cache_key=$CK)"
