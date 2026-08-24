#!/usr/bin/env bats

setup() {
  REPO_ROOT=$(cd -- "$BATS_TEST_DIRNAME/.." && pwd)
  WORK=$(mktemp -d)
  MOCK_BIN="$WORK/bin"
  mkdir -p "$MOCK_BIN"
}

teardown() {
  rm -rf "$WORK"
}

make_uname() {
  cat > "$MOCK_BIN/uname" <<EOF
#!/usr/bin/env bash
if [ "\$1" = "-s" ]; then printf '%s\n' '${1}'; else printf '%s\n' '${2}'; fi
EOF
  chmod +x "$MOCK_BIN/uname"
}

make_curl() {
  cat > "$MOCK_BIN/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
url=
out=
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o) out=$2; shift 2 ;;
    http*) url=$1; shift ;;
    *) shift ;;
  esac
done
printf '%s\n' "$url" >> "$CURL_LOG"
if [[ $url == */checksums.txt ]]; then
  archive=${out%/*}/aiproxy-linux-amd64.tar.gz
  checksum="$(sha256sum "$archive")"
  checksum="${checksum%% *}  aiproxy-linux-amd64.tar.gz"
  if [ "${BAD_CHECKSUM:-0}" = 1 ]; then checksum="0000000000000000000000000000000000000000000000000000000000000000  aiproxy-linux-amd64.tar.gz"; fi
  printf '%s\n' "$checksum" > "$out"
else
  printf 'archive bytes' > "$out"
fi
EOF
  chmod +x "$MOCK_BIN/curl"
}

@test "download maps platform, constructs the release URLs, and verifies checksum" {
  make_uname Linux x86_64
  make_curl
  run env PATH="$MOCK_BIN:$PATH" CURL_LOG="$WORK/urls" ASDF_INSTALL_VERSION=1.2.3 ASDF_DOWNLOAD_PATH="$WORK/download" "$REPO_ROOT/bin/download"
  [ "$status" -eq 0 ]
  grep -Fx 'https://github.com/egose/aiproxy/releases/download/v1.2.3/aiproxy-linux-amd64.tar.gz' "$WORK/urls"
  grep -Fx 'https://github.com/egose/aiproxy/releases/download/v1.2.3/checksums.txt' "$WORK/urls"
}

@test "download rejects unsupported platforms" {
  make_uname SunOS sparc
  run env PATH="$MOCK_BIN:$PATH" ASDF_INSTALL_VERSION=1.2.3 ASDF_DOWNLOAD_PATH="$WORK/download" "$REPO_ROOT/bin/download"
  [ "$status" -ne 0 ]
  [[ $output == *"Unsupported OS"* ]]
}

@test "download rejects a checksum mismatch" {
  make_uname Linux x86_64
  make_curl
  run env PATH="$MOCK_BIN:$PATH" CURL_LOG="$WORK/urls" BAD_CHECKSUM=1 ASDF_INSTALL_VERSION=1.2.3 ASDF_DOWNLOAD_PATH="$WORK/download" "$REPO_ROOT/bin/download"
  [ "$status" -ne 0 ]
  [[ $output == *"Checksum verification failed"* ]]
}

@test "install rejects unsafe archive members without modifying the install path" {
  mkdir -p "$WORK/download" "$WORK/source"
  printf 'payload' > "$WORK/source/payload"
  tar -czf "$WORK/download/aiproxy-linux-amd64.tar.gz" --transform='s#payload#../escape#' -C "$WORK/source" payload
  run env ASDF_INSTALL_TYPE=version ASDF_INSTALL_VERSION=1.2.3 ASDF_INSTALL_PATH="$WORK/install" ASDF_DOWNLOAD_PATH="$WORK/download" "$REPO_ROOT/bin/install"
  [ "$status" -ne 0 ]
  [ ! -e "$WORK/escape" ]
  [ ! -e "$WORK/install" ]
}

@test "install atomically installs exactly one regular expected executable" {
  mkdir -p "$WORK/download" "$WORK/source" "$WORK/installs"
  printf '#!/bin/sh\nexit 0\n' > "$WORK/source/aiproxy"
  tar -czf "$WORK/download/aiproxy-linux-amd64.tar.gz" -C "$WORK/source" aiproxy
  run env ASDF_INSTALL_TYPE=version ASDF_INSTALL_VERSION=1.2.3 ASDF_INSTALL_PATH="$WORK/installs/1.2.3" ASDF_DOWNLOAD_PATH="$WORK/download" "$REPO_ROOT/bin/install"
  [ "$status" -eq 0 ]
  [ -x "$WORK/installs/1.2.3/bin/aiproxy" ]
  [ "$(find "$WORK/installs/1.2.3" -type f | wc -l)" -eq 1 ]
  [ -z "$(find "$WORK/installs" -maxdepth 1 -name '.aiproxy.install.*' -print)" ]
}

@test "list-all fails closed on an API error" {
  cat > "$MOCK_BIN/curl" <<'EOF'
#!/usr/bin/env bash
exit 22
EOF
  chmod +x "$MOCK_BIN/curl"
  run env PATH="$MOCK_BIN:$PATH" "$REPO_ROOT/bin/list-all"
  [ "$status" -eq 22 ]
}

@test "release validation rejects non-SemVer tags before reading git state" {
  run "$REPO_ROOT/scripts/validate-release-tag.sh" v01.2.3
  [ "$status" -ne 0 ]
  [[ $output == *"anchored SemVer"* ]]
}
