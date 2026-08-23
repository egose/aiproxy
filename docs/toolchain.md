# Toolchain Policy

`go.mod` is the authoritative minimum Go release line. It stays at the lowest
supported `major.minor` version and is not raised only to match a local tool
installation.

`.tool-versions` pins the exact Go patch used by local development and normal
CI. The Docker builder image pins the same exact Go patch, and both must remain
on the `go.mod` release line.

`package.json` is authoritative for the pnpm package-manager version, and
`.tool-versions` must select the same exact pnpm version.

Run `make check-toolchain` after changing `go.mod`, `.tool-versions`,
`Dockerfile`, `package.json`, or CI setup.
