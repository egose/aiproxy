# Measured Bun fetch profile

`bun-1.4.4-fetch.json` is a measured, secret-free custom ClientHello recipe, not
an illustrative Chrome preset. `bun-1.4.4-fetch.metadata.json` records the
installed Bun revision, Linux x64 platform, isolated fetch conditions and three
fresh Bun/uTLS normalized comparisons. The metadata is **not** part of the
loadable custom-spec JSON.

## Reproduce Without Replacing The Fixture

Compare from the repository root with an already installed Bun 1.4.4:

```sh
go run ./scripts/tls-clienthello compare --bun bun --api fetch --samples 3 --spec examples/tls/bun-1.4.4-fetch.json
```

To produce separate local evidence, create output directories first:

```sh
mkdir -p tls
go run ./scripts/tls-clienthello capture --bun bun --api fetch --samples 3 --out tls/bun-1.4.4-fetch.json --metadata tls/bun-1.4.4-fetch.metadata.json
go run ./scripts/tls-clienthello compare --bun bun --api fetch --samples 3 --spec tls/bun-1.4.4-fetch.json
```

Existing regular outputs require explicit `--force`; the default does not
overwrite. Different Bun versions/APIs or platforms should use different output
filenames instead of changing the identity of the committed example. Recipe and
metadata are separate files; `--samples` accepts 2–16 (default 3).

Measured structure: 1500 record bytes, 17 suites, HTTP/1.1-only ALPN, groups
`4588/29/23/24` (MLKEM/X25519/P-256/P-384), and shares `4588/29`. No GREASE, ALPS,
padding, provider credentials, captured hostnames or fixed real keys are included.
Preserved legacy advertisements are not permission to negotiate weak crypto.

## Use With A Static Config

The Docker image includes the `*-fetch.json` recipes under
`/usr/local/share/aiproxy/tls/`. For the bundled Bun recipe, use:

```hcl
tls_client_hello           = "custom"
tls_client_hello_spec_file = "/usr/local/share/aiproxy/tls/bun-1.4.4-fetch.json"
```

The image includes recipes only, not their metadata. The bundled recipe reflects
the measured Bun version and environment described above; it does not detect the
installed runtime.

The complete [`examples/tls-client-hello.hcl`](../tls-client-hello.hcl) resolves its
reference from `examples/`, the directory containing that config file. For an
existing `config/config.hcl` elsewhere, copy the recipe into a new config-relative
destination, from the repository root:

```sh
mkdir -p config/tls
cp -- examples/tls/bun-1.4.4-fetch.json config/tls/bun-1.4.4-fetch.json
```

Then add the provider below to `config/config.hcl`. The endpoint and model are
illustrative; choose a real endpoint/model and set `UPSTREAM_API_KEY` before use.

```hcl
provider "openai-compatible" "measured-bun" {
  base_url                  = "https://api.example.test/v1"
  api_key                   = env("UPSTREAM_API_KEY")
  tls_client_hello           = "custom"
  tls_client_hello_spec_file = "tls/bun-1.4.4-fetch.json"

  model "example-model" {}
}
```

Relative paths use the config file's lexical directory, not the invocation cwd.
Inline `AIPROXY_CONFIG` requires an absolute reference; conversion emits a resolved
absolute host-local reference without copying the recipe. Do not load the metadata
file or copy a repository-relative reference into an unrelated config directory.
Validation caches the recipe; successful reload applies replacement bytes to new
work, while failed reloads keep the old cached profile. Static derivatives inherit
the profile and cannot declare TLS settings locally. Managed providers are built-in-only.

See the [tool usage and limits](../../scripts/tls-clienthello/README.md).
Direct `Profile.Materialize` use has local verified TLS 1.2/1.3 test coverage;
production serving/transport integration is checked separately. This example
does not establish other Bun versions/APIs/options or SNI lengths, resumed TLS, an entire Bun/HTTP-stack
fingerprint, live-provider compatibility or WAF acceptance.
