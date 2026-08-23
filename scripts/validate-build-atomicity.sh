#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

fake_go="$work/fake-go"
cat >"$fake_go" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

fail_target=${BUILD_ATOMICITY_FAIL_TARGET:-}
target=${GOOS:-}:${GOARCH:-}
if [ "$target" = "$fail_target" ]; then
  exit 37
fi

out=
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then
    shift
    out=$1
  fi
  shift || true
done

if [ -z "$out" ]; then
  exit 2
fi

mkdir -p "${out%/*}"
printf 'fake executable for %s\n' "$target" >"$out"
EOF
chmod +x "$fake_go"

dist="$work/dist"
mkdir -p "$dist/linux-arm64"
printf 'stale executable\n' >"$dist/linux-arm64/aiproxy"

if BUILD_ATOMICITY_FAIL_TARGET=linux:arm64 make -C "$repo_root" BUILD_GO="$fake_go" DIST_DIR="$dist" OS_ARCH_PAIRS="linux:amd64 linux:arm64 linux:386" build-all; then
  echo "build-all masked an intermediate failure" >&2
  exit 1
fi

if [ -e "$dist/linux-arm64" ]; then
  echo "failed target left an archiveable directory or stale executable" >&2
  exit 1
fi

if [ -e "$dist/linux-386" ]; then
  echo "build-all continued after the first failed target" >&2
  exit 1
fi

ok_dist="$work/ok-dist"
make -C "$repo_root" BUILD_GO="$fake_go" DIST_DIR="$ok_dist" OS_ARCH_PAIRS="linux:amd64 windows:386" build-all
make -C "$repo_root" DIST_DIR="$ok_dist" OS_ARCH_PAIRS="linux:amd64 windows:386" build-archive validate-archives

bad_dist="$work/bad-dist"
mkdir -p "$bad_dist/windows-386"
printf 'wrong executable\n' >"$bad_dist/windows-386/aiproxy"
if make -C "$repo_root" DIST_DIR="$bad_dist" OS_ARCH_PAIRS="windows:386" build-archive; then
  echo "build-archive accepted a wrong executable name" >&2
  exit 1
fi

rm -rf "$bad_dist"
mkdir -p "$bad_dist/linux-amd64"
: >"$bad_dist/linux-amd64/aiproxy"
if make -C "$repo_root" DIST_DIR="$bad_dist" OS_ARCH_PAIRS="linux:amd64" build-archive; then
  echo "build-archive accepted an empty executable" >&2
  exit 1
fi

echo "build atomicity validation passed"
