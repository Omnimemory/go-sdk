#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <router-openapi.json>" >&2
  exit 2
fi

jq '
  .paths |= with_entries(
    select(
      .key == "/memory/ingest" or
      .key == "/memory/ingest/jobs/{job_id}" or
      .key == "/memory/retrieval" or
      .key == "/memory/retrieval/hybrid"
    )
  )
' "$1" > internal/openapi/openapi.json
