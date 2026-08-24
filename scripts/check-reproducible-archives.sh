#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

for output in one two; do
  make -C "$repo_root" --no-print-directory \
    DIST_DIR="$work/$output" \
    OS_ARCH_PAIRS="${OS_ARCH_PAIRS:-linux:amd64}" \
    VERSION="${VERSION:-reproducible}" \
    SOURCE_DATE_EPOCH="${SOURCE_DATE_EPOCH:-0}" \
    build-all build-archive validate-archives
done

(cd "$work/one" && sha256sum ./*.tar.gz) > "$work/one.sha256"
(cd "$work/two" && sha256sum ./*.tar.gz) > "$work/two.sha256"
diff -u "$work/one.sha256" "$work/two.sha256"
