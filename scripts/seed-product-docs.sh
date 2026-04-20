#!/usr/bin/env bash
set -euo pipefail

# Seed product PDFs into the RAG ingestion service.
# Idempotent: skips if the product-docs collection already exists.
#
# Usage:
#   ./scripts/seed-product-docs.sh <ingestion-base-url> [jwt-token]
#
# Examples:
#   ./scripts/seed-product-docs.sh http://localhost:8001
#   ./scripts/seed-product-docs.sh http://localhost:8001 eyJhbGci...

INGESTION_URL="${1:?Usage: $0 <ingestion-base-url> [jwt-token]}"
JWT_TOKEN="${2:-}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PDF_DIR="$SCRIPT_DIR/../docs/product-catalog"
COLLECTION="product-docs"

# Build auth header if token provided
AUTH_HEADER=""
if [ -n "$JWT_TOKEN" ]; then
  AUTH_HEADER="Authorization: Bearer $JWT_TOKEN"
fi

curl_opts=(-s -f)
if [ -n "$AUTH_HEADER" ]; then
  curl_opts+=(-H "$AUTH_HEADER")
fi

# Check if collection already exists
echo "==> Checking for existing '$COLLECTION' collection..."
COLLECTIONS=$(curl "${curl_opts[@]}" "$INGESTION_URL/collections" 2>/dev/null || echo '{"collections":[]}')

if echo "$COLLECTIONS" | grep -q "\"name\":\"$COLLECTION\""; then
  echo "==> Collection '$COLLECTION' already exists, skipping seed."
  exit 0
fi

# Upload each PDF
PDF_COUNT=0
for pdf in "$PDF_DIR"/*.pdf; do
  [ -f "$pdf" ] || continue
  FILENAME=$(basename "$pdf")
  echo "==> Uploading $FILENAME to collection '$COLLECTION'..."

  RESPONSE=$(curl "${curl_opts[@]}" \
    -X POST \
    -F "file=@$pdf" \
    "$INGESTION_URL/ingest?collection=$COLLECTION" 2>&1) || {
    echo "    WARN: Failed to upload $FILENAME (may be rate-limited), retrying in 15s..."
    sleep 15
    RESPONSE=$(curl "${curl_opts[@]}" \
      -X POST \
      -F "file=@$pdf" \
      "$INGESTION_URL/ingest?collection=$COLLECTION")
  }

  CHUNKS=$(echo "$RESPONSE" | grep -o '"chunks_created":[0-9]*' | cut -d: -f2 || echo "?")
  echo "    OK: $CHUNKS chunks created"
  PDF_COUNT=$((PDF_COUNT + 1))

  # Rate limit: ingestion API allows 5 requests/minute
  if [ "$PDF_COUNT" -lt "$(ls "$PDF_DIR"/*.pdf 2>/dev/null | wc -l)" ]; then
    echo "    (waiting 13s for rate limit...)"
    sleep 13
  fi
done

echo "==> Seeded $PDF_COUNT PDFs into collection '$COLLECTION'."
