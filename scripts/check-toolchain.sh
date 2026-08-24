#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(cd -- "$script_dir/.." && pwd)
self_test_tmp=

usage() {
  printf 'usage: %s [--self-test]\n' "${0##*/}" >&2
}

read_go_mod_version() {
  local root=$1 line
  while IFS= read -r line; do
    if [[ $line =~ ^go[[:space:]]+([0-9]+\.[0-9]+)$ ]]; then
      printf '%s\n' "${BASH_REMATCH[1]}"
      return 0
    fi
  done < "$root/go.mod"
  return 1
}

read_tool_version() {
  local root=$1 tool=$2 line tool_regex
  tool_regex="^${tool}[[:space:]]+([^[:space:]]+)$"
  while IFS= read -r line; do
    if [[ $line =~ $tool_regex ]]; then
      printf '%s\n' "${BASH_REMATCH[1]}"
      return 0
    fi
  done < "$root/.tool-versions"
  return 1
}

read_docker_go_version() {
  local root=$1 line
  while IFS= read -r line; do
    if [[ $line =~ ^FROM[[:space:]]+golang:([0-9]+\.[0-9]+\.[0-9]+)(@sha256:[0-9a-f]{64})?([[:space:]]|$) ]]; then
      printf '%s\n' "${BASH_REMATCH[1]}"
      return 0
    fi
  done < "$root/Dockerfile"
  return 1
}

read_package_node_version() {
  local root=$1 line node_regex='"node"[[:space:]]*:[[:space:]]*"([^"]+)"'
  while IFS= read -r line; do
    if [[ $line =~ $node_regex ]]; then
      printf '%s\n' "${BASH_REMATCH[1]}"
      return 0
    fi
  done < "$root/package.json"
  return 1
}

read_python_version() {
  local root=$1
  tr -d '[:space:]' < "$root/.python-version"
}

read_package_manager_pnpm_version() {
  local root=$1 line package_regex='"packageManager"[[:space:]]*:[[:space:]]*"pnpm@([^"]+)"'
  while IFS= read -r line; do
    if [[ $line =~ $package_regex ]]; then
      printf '%s\n' "${BASH_REMATCH[1]}"
      return 0
    fi
  done < "$root/package.json"
  return 1
}

minor_version() {
  local version=$1
  if [[ $version =~ ^([0-9]+\.[0-9]+)(\.|$) ]]; then
    printf '%s\n' "${BASH_REMATCH[1]}"
    return 0
  fi
  return 1
}

check_root() {
  local root=$1 failed=0
  local go_mod tool_go docker_go tool_pnpm package_pnpm tool_node package_node tool_python python_version tool_go_minor docker_go_minor

  go_mod=$(read_go_mod_version "$root") || { printf 'missing go directive in go.mod\n' >&2; return 1; }
  tool_go=$(read_tool_version "$root" golang) || { printf 'missing golang entry in .tool-versions\n' >&2; return 1; }
  docker_go=$(read_docker_go_version "$root") || { printf 'missing exact golang builder tag in Dockerfile\n' >&2; return 1; }
  tool_pnpm=$(read_tool_version "$root" pnpm) || { printf 'missing pnpm entry in .tool-versions\n' >&2; return 1; }
  package_pnpm=$(read_package_manager_pnpm_version "$root") || { printf 'missing pnpm packageManager in package.json\n' >&2; return 1; }
  tool_node=$(read_tool_version "$root" nodejs) || { printf 'missing nodejs entry in .tool-versions\n' >&2; return 1; }
  package_node=$(read_package_node_version "$root") || { printf 'missing exact Node engine in package.json\n' >&2; return 1; }
  tool_python=$(read_tool_version "$root" python) || { printf 'missing python entry in .tool-versions\n' >&2; return 1; }
  python_version=$(read_python_version "$root") || { printf 'missing .python-version\n' >&2; return 1; }

  tool_go_minor=$(minor_version "$tool_go") || { printf 'invalid golang version in .tool-versions: %s\n' "$tool_go" >&2; return 1; }
  docker_go_minor=$(minor_version "$docker_go") || { printf 'invalid golang version in Dockerfile: %s\n' "$docker_go" >&2; return 1; }

  if [ "$tool_go_minor" != "$go_mod" ]; then
    printf '.tool-versions golang %s is not on go.mod release line %s\n' "$tool_go" "$go_mod" >&2
    failed=1
  fi
  if [ "$docker_go_minor" != "$go_mod" ]; then
    printf 'Dockerfile golang %s is not on go.mod release line %s\n' "$docker_go" "$go_mod" >&2
    failed=1
  fi
  if [ "$tool_go" != "$docker_go" ]; then
    printf '.tool-versions golang %s does not match Dockerfile golang %s\n' "$tool_go" "$docker_go" >&2
    failed=1
  fi
  if [ "$tool_pnpm" != "$package_pnpm" ]; then
    printf '.tool-versions pnpm %s does not match packageManager pnpm %s\n' "$tool_pnpm" "$package_pnpm" >&2
    failed=1
  fi
  if [ "$tool_node" != "$package_node" ]; then
    printf '.tool-versions nodejs %s does not match package.json engine %s\n' "$tool_node" "$package_node" >&2
    failed=1
  fi
  if [ "$tool_python" != "$python_version" ]; then
    printf '.tool-versions python %s does not match .python-version %s\n' "$tool_python" "$python_version" >&2
    failed=1
  fi

  if [ "$failed" -ne 0 ]; then
    return 1
  fi

  printf 'toolchain ok: Go minimum %s, pinned Go %s, Node %s, Python %s, pnpm %s\n' "$go_mod" "$tool_go" "$tool_node" "$tool_python" "$tool_pnpm"
}

self_test() {
  local tmp
  self_test_tmp=$(mktemp -d)
  tmp=$self_test_tmp
  trap 'rm -rf "$self_test_tmp"' EXIT

  printf 'module example.test/toolchain\n\ngo 1.26\n' > "$tmp/go.mod"
  printf 'nodejs 26.7.0\npython 3.14.6\ngolang 1.26.6\npnpm 10.14.0\n' > "$tmp/.tool-versions"
  printf '3.14.6\n' > "$tmp/.python-version"
  printf 'FROM golang:1.26.6@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa AS builder\n' > "$tmp/Dockerfile"
  printf '{\n  "packageManager": "pnpm@10.14.0",\n  "engines": {"node": "26.7.0"}\n}\n' > "$tmp/package.json"

  check_root "$tmp" >/dev/null

  printf 'nodejs 26.7.0\npython 3.14.6\ngolang 1.27.0\npnpm 10.14.0\n' > "$tmp/.tool-versions"
  if check_root "$tmp" >/dev/null 2>&1; then
    printf 'self-test failed: mismatched Go fixture unexpectedly passed\n' >&2
    return 1
  fi

  printf 'nodejs 26.7.0\npython 3.14.6\ngolang 1.26.6\npnpm 11.22.0\n' > "$tmp/.tool-versions"
  if check_root "$tmp" >/dev/null 2>&1; then
    printf 'self-test failed: mismatched pnpm fixture unexpectedly passed\n' >&2
    return 1
  fi

  printf 'nodejs 27.0.0\npython 3.14.6\ngolang 1.26.6\npnpm 10.14.0\n' > "$tmp/.tool-versions"
  if check_root "$tmp" >/dev/null 2>&1; then
    printf 'self-test failed: mismatched Node fixture unexpectedly passed\n' >&2
    return 1
  fi

  printf 'nodejs 26.7.0\npython 3.15.0\ngolang 1.26.6\npnpm 10.14.0\n' > "$tmp/.tool-versions"
  if check_root "$tmp" >/dev/null 2>&1; then
    printf 'self-test failed: mismatched Python fixture unexpectedly passed\n' >&2
    return 1
  fi
}

case "${1:-}" in
  '')
    check_root "$repo_root"
    ;;
  --self-test)
    self_test
    check_root "$repo_root"
    ;;
  *)
    usage
    exit 2
    ;;
esac
