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
echo "$SCAN" | grep -q 'merkle_root' || { echo "FAIL: merkle_root missing from scan summary"; exit 1; }

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

echo "=== hybrid query (auth / chunk) ==="
HQ=$(mcp 7 analysis "{\"action\":\"query\",\"query\":\"where is auth handled\",\"cache_key\":\"$CK\"}")
echo "$HQ" | head -c 1000; echo
echo "$HQ" | grep -qi 'auth\|hybrid\|chunk\|Authenticate\|Token\|Login\|protocol.go\|engine' || { echo "FAIL: hybrid auth query miss"; exit 1; }
# Prefer chunk-level hit signal when present (not whole-package dump).
echo "$HQ" | grep -qi 'chunk\|HybridHit\|path.*\.go' || echo "WARN: no explicit chunk field (engine may still have hit)"

echo "=== complexity_hints ==="
CH=$(mcp 8 analysis "{\"action\":\"complexity_hints\",\"top_n\":10,\"cache_key\":\"$CK\"}")
echo "$CH" | head -c 800; echo
echo "$CH" | grep -qi 'disclaimer\|heuristic\|hot_spots\|complexity' || { echo "FAIL: complexity_hints miss"; exit 1; }

echo "=== taint (heuristic) ==="
TAINT_SRC="${AX_TAINT_SOURCE:-Source}"
TAINT_SINK="${AX_TAINT_SINK:-Sink}"
TAINT_PATH="${AX_TAINT_PATH:-}"
if [[ -z "$TAINT_PATH" ]]; then
  # Prefer fixture module if present; else probe oculus Engine call chain.
  FIX="$REPO/testdata/taintfix"
  if [[ -d "$FIX" ]]; then
    TAINT_PATH="$FIX"
  else
    TAINT_PATH="$REPO"
    TAINT_SRC="${AX_TAINT_SOURCE:-ScanProject}"
    TAINT_SINK="${AX_TAINT_SINK:-getOrScan}"
  fi
fi
TN=$(mcp 9 analysis "{\"action\":\"taint\",\"from\":\"$TAINT_SRC\",\"to\":\"$TAINT_SINK\",\"path\":\"$TAINT_PATH\",\"cache_key\":\"$CK\"}")
echo "$TN" | head -c 800; echo
echo "$TN" | grep -qi 'heuristic\|federated\|disclaimer' || { echo "FAIL: taint miss"; exit 1; }

echo "AX dogfood OK (cache_key=$CK)"
