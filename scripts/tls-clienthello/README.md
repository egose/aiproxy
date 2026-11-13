# Local Bun ClientHello capture

This optional developer tool measures an **already installed** Bun runtime. It
does not install Bun, contact providers, or become a serving dependency. The
initial supported API is `fetch`, using fresh processes and first-flight TLS.

Run from the repository root with the repository's Go toolchain:

```sh
mkdir -p tls
go run ./scripts/tls-clienthello capture --bun bun --api fetch --samples 3 --out tls/bun-fetch.json --metadata tls/bun-fetch.metadata.json
go run ./scripts/tls-clienthello compare --bun bun --api fetch --samples 3 --spec tls/bun-fetch.json
```

`capture` requires two distinct paths in existing directories. It publishes only
after the exported recipe passes `tlsprofile.ParseCustom` and all normalized
comparisons match. Existing regular files require an explicit `--force` flag;
symlink, directory and special-file destinations are rejected even with that flag.
Files are staged with mode `0600` and published without following output symlinks.
The two files are not a filesystem-wide transaction: a forced publication can
partially succeed if the second rename fails. Non-forced failures roll back only
links created by this invocation.

`compare` reads an existing regular custom-spec file (at most 64 KiB), prints a
separate JSON comparison report, and exits nonzero on a mismatch. `--samples`
accepts 2–16; the default is 3. No arbitrary Bun arguments, preloads, networking
APIs, target URLs or experimental HTTP/2/HTTP/3 flags are accepted.

## Selecting Bun safely

`--bun bun` looks up the executable without launching a shell. A recognized asdf
Bun shim is resolved with `asdf which bun`, using only `PATH`, `HOME`, locale and
the optional `ASDF_DATA_DIR`; project tool-version selection is retained. The
resolved **direct binary**, not the shim, runs the capture. Other wrappers fail
with an actionable error rather than inheriting the full parent environment.

An operator can select the direct installed executable explicitly, for example:

```sh
go run ./scripts/tls-clienthello capture --bun "$(asdf which bun)" --api fetch --samples 3 --out tls/bun-fetch.json --metadata tls/bun-fetch.metadata.json
```

The selected executable is trusted operator input, not sandboxed hostile code.
Runtime paths and child stdout/stderr are never copied into published metadata or
failure reports.

## Capture conditions and bounds

Each Bun sample gets its own empty HOME, config directory, temporary directory
and cwd. The child receives a minimal explicit environment, not inherited
provider credentials, proxy variables, `NODE_OPTIONS`, `BUN_*`, preload settings
or TLS-verification overrides. `NO_PROXY` and `no_proxy` are set to `*`. The parent
environment is unchanged. Windows also retains the OS-required `SYSTEMROOT`.

The exact invocation order is:

```text
bun --config=<empty-config> --no-env-file --no-install run <fetch-script> <loopback-url>
```

The embedded script uses a literal `https://127.0.0.1:<ephemeral-port>/`,
`proxy: false`, `tls.serverName: "localhost"`, GET without a body,
`redirect: "error"` and `AbortSignal.timeout(4000)`. The loopback listener closes
immediately after the first ClientHello. No certificate, verification bypass,
privileged packet capture, external application traffic or HTTP body is needed.

Accept/read deadlines are 3 seconds and each child is bounded to 5 seconds.
Canceled/timed-out children are killed and reaped; listeners and sockets close.
Each stdout/stderr stream is capped at 64 KiB. TCP/TLS fragmentation is
reassembled with at most 16 records, at most 18,432 payload bytes per record and
at most 32,768 total handshake bytes. Unexpected/truncated/oversized data fails
without a partial export.

## Export and comparison contract

The profile is explicitly exported into the accepted native-shaped JSON schema,
**not** `json.Marshal(ClientHelloSpec)`. The pinned uTLS v1.8.2 Fingerprinter uses
`AllowBluntMimicry: false`, `AlwaysAddPadding: false` and
`RealPSKResumption: false`. The private `X25519MLKEM768` symbol adapter is used
without mutating uTLS dictionaries. Real key bytes, captured SNI, client random
and session contents are not recipe fields; materialization generates fresh
cryptographic values and uses the actual connection's SNI.

Unsupported, opaque or resumption-dependent extensions/payloads are rejected.
Empty session-ticket and PSK-mode advertisements are first-flight hints, not
imported sessions. `capture` rejects padding rather than infer fixed or
conditional padding policy from a capture. An explicitly supplied valid padding
recipe can still be checked by `compare`.

Comparison takes multiple fresh Bun captures and multiple fresh materialized
uTLS captures against the same synthetic loopback conditions. Only client-random
bytes, session-ID **bytes** and supported real key-exchange **bytes** are zeroed.
Record/handshake versions and lengths, record boundaries, session-ID length,
cipher/extension order, groups, signatures, ALPN, SNI, padding, GREASE values and
key-share groups/lengths remain significant. TCP segmentation is not TLS record
framing. No JA3/JA4-only or GREASE-normalized match is claimed.

Metadata is separate: runtime version/revision/platform/architecture, conditions,
sample counts, dependency/schema/policy pins, canonical-spec digest, structural
summary, normalized digest and residual limitations. It contains no executable
or machine paths, host identifiers, provider credentials, private keys or actual
session IDs.

## Measured fixture and verification

[`examples/tls/`](../../examples/tls/) contains a measured Bun **1.4.4 fetch**
recipe and metadata for Linux x64. The default capture was 1500 bytes, with 17
suites, HTTP/1.1-only ALPN, groups `4588/29/23/24` and shares `4588/29`; no GREASE,
ALPS or padding. This is not a Bun-to-Chrome mapping.

Normal CI uses sanitized fixtures and hermetic Go child-process tests, requiring
no installed Bun:

```sh
go test ./internal/tlsprofile ./scripts/tls-clienthello
go test -race ./internal/tlsprofile ./scripts/tls-clienthello
```

The optional real-runtime check also verifies the **captured** materialized
recipe against local TLS 1.2 and TLS 1.3 servers with synthetic trust and normal
hostname/certificate validation:

```sh
TLS_CLIENTHELLO_TEST_BUN=bun go test -count=1 -run '^TestInstalledBunCaptureCompareAndVerifiedTLS$' -v ./scripts/tls-clienthello
```

The capture/compare commands themselves do not perform a complete TLS handshake.
Production transport/serving activation is verified separately by TLS-03/TLS-08;
this tool's direct materialization test is not that integration evidence. Other
Bun versions/APIs/options, resumed handshakes, full HTTP/TCP-stack equivalence,
provider compatibility, WAF acceptance and bot-detector bypass remain unverified.
