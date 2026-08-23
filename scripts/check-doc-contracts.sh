#!/usr/bin/env bash
set -euo pipefail

root=${1:-$(pwd)}

public_expected=$(cat <<'EOF'
| Surface | `openai` | `openai-compatible` | `anthropic` | `gemini` |
| --- | --- | --- | --- | --- |
| `GET /v1/models` | Proxy-owned | Proxy-owned | Proxy-owned | Proxy-owned |
| `GET /v1/billing/usage` | Proxy-owned local usage accounting | Proxy-owned local usage accounting | Proxy-owned local usage accounting | Proxy-owned local usage accounting |
| `GET /metrics` | Proxy-owned Prometheus metrics | Proxy-owned Prometheus metrics | Proxy-owned Prometheus metrics | Proxy-owned Prometheus metrics |
| `POST /v1/chat/completions` | JSON and SSE | JSON and SSE | JSON and SSE translated | JSON and SSE translated |
| `POST /v1/embeddings` | Yes | Yes | No | Yes |
| `POST /v1/responses` | JSON and SSE | JSON and SSE | JSON and SSE translated subset | JSON and SSE translated subset |
| `POST /v1/images/generations` | Yes | Yes | No | No |
| `POST /v1/audio/transcriptions` | Yes | Yes | No | No |
| `POST /v1/audio/speech` | Yes | Yes | No | No |
EOF
)

capability_expected=$(cat <<'EOF'
| Provider type | Default capabilities when omitted | Additional supported capabilities |
| --- | --- | --- |
| `openai` | `chat`, `responses`, `embeddings` | `images`, `audio_transcriptions`, `audio_speech` |
| `openai-compatible` | `chat`, `responses`, `embeddings` | `images`, `audio_transcriptions`, `audio_speech` |
| `anthropic` | `chat`, `responses` | None |
| `gemini` | `chat`, `responses` | `embeddings` |
EOF
)

public_matrix_files=(
  AGENTS.md
  README.md
  docs/design.md
  website/docs/api-reference.md
)

capability_matrix_files=(
  AGENTS.md
  README.md
  docs/design.md
  website/docs/api-reference.md
  website/docs/providers-and-routing.md
)

extract_block() {
  local file=$1
  local name=$2
  awk "/^<!--[[:space:]]*docs-contract:${name}:start[[:space:]]*-->$/ { inside=1; next } /^<!--[[:space:]]*docs-contract:${name}:end[[:space:]]*-->$/ { inside=0; next } inside { print }" "$file"
}

for relative in "${public_matrix_files[@]}"; do
  file="$root/$relative"
  public_actual=$(extract_block "$file" public-matrix)
  if [[ "$public_actual" != "$public_expected" ]]; then
    printf 'public matrix drift in %s\n' "$relative" >&2
    exit 1
  fi
done

for relative in "${capability_matrix_files[@]}"; do
  file="$root/$relative"
  capability_actual=$(extract_block "$file" capability-matrix)
  if [[ "$capability_actual" != "$capability_expected" ]]; then
    printf 'capability matrix drift in %s\n' "$relative" >&2
    exit 1
  fi
done

printf 'documentation contract matrices match\n'
