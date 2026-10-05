# RESERVE v1.2.2

Adds a read-only CLI preview of the official `reserve-workflows` catalog through
the permanent RESERVE workflow API at `https://api.reservecli.dev/v1`.

- Published: 2026-09-15
- Severity: `info`
- Release: [GitHub v1.2.2](https://github.com/derickschaefer/reserve/releases/tag/v1.2.2)
- Installer: [download.reservecli.dev/install.sh](https://download.reservecli.dev/install.sh)

## Added

- `reserve workflow browse` for the complete official catalog or one collection.
- `reserve workflow get <collection> <workflow> [version]` for versioned
  official workflow YAML retrieval.
- Typed internal registry client coverage for catalog, collection, and artifact
  reads.
- v1.2.2 preview documentation for the public read-only API boundary.

## Changed

- The release version is now `v1.2.2`.
- Official workflow reads use `https://api.reservecli.dev/v1`; the CLI does not
  expose GitHub, R2, or another storage provider as its repository contract.
- Catalog reads support the existing table and JSON output modes, while
  workflow retrieval defaults to raw YAML and supports JSON envelopes.

## Security

- Workflow pipeline content is never executed by RESERVE v1.2.2.
- `workflow render` does not invoke Bash or another shell.
- Repository-style references cannot traverse or escape the configured workflow root, including through symlinks.
- Pipeline execution remains deferred until secure, deterministic, and cross-platform semantics are defined.
- The v1.2.2 preview does not use the FRED API key as RESERVE authentication.
- Verified-email enrollment, one-time FRED-key validation, RESERVE token
  issuance, and authenticated API enforcement remain v1.2.3 work.

## Known Limitations

- The preview is read-only: it does not install, update, publish, or execute
  remote workflows.
- No private-repository authentication, repository management, ratings, search,
  remote execution, or user enrollment is included.
- Local workflow authoring and execution semantics remain separate from the
  official API catalog.
- Rendered pipeline stages are descriptive output for operator or agent review, not a promise of portable or safe shell execution.

## Verification

- `make test` passes.
- `go test ./cmd ./internal/... ./tests` passes.
- Registry client tests cover catalog, collection, versioned artifact, and
  bearer-header behavior.
- `go vet ./...` passes.
- The release distribution build produces platform archives and SHA256SUMS.
- Release docs and manifests are updated for `v1.2.2`.

## Update

Download the latest release from the GitHub releases page or rerun the install
script from `download.reservecli.dev`.

## Additional Release Context

For user-focused release narratives, design rationale, and implementation insights,
visit the [RESERVE Releases](https://reservecli.dev/documentation/releases/)
section of the RESERVE website.
