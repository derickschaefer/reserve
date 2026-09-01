# RESERVE v1.2.0

Introduces the foundational workflow ecosystem for creating, validating,
inspecting, and safely rendering reusable economic analyses.

- Published: 2026-08-27
- Severity: `info`
- Release: [GitHub v1.2.0](https://github.com/derickschaefer/reserve/releases/tag/v1.2.0)
- Installer: [download.reservecli.dev/install.sh](https://download.reservecli.dev/install.sh)

## Added

- New `workflow` commands manage repository- and collection-aware YAML definitions.
- Static workflows support repeatable analyses with no runtime inputs.
- Dynamic workflows declare ordered positional inputs using `@1`, `@2`, through `@X`.
- `workflow contract` prints input labels, formats, samples, and descriptions.
- `workflow render` resolves runtime values and prints one copy-ready pipeline command.
- Collection creation scaffolds a repository-owned `README.md` without overwriting existing documentation.

## Changed

- The experimental `snippet` command and configuration surface have been removed in favor of workflows.
- Long workflow titles and summaries are truncated to preserve readable terminal tables.
- Workflow documentation and AI onboarding now describe the repository, collection, workflow, and runtime-contract model.
- Workflow validation rejects unknown YAML fields and incompatible `reserve_version` requirements.
- Empty observation ranges now report the affected series and requested dates before a pipeline receives no rows.

## Security

- Workflow pipeline content is never executed by RESERVE v1.2.0.
- `workflow render` does not invoke Bash or another shell.
- Repository-style references cannot traverse or escape the configured workflow root, including through symlinks.
- Pipeline execution remains deferred until secure, deterministic, and cross-platform semantics are defined.

## Known Limitations

- Repository search, installation, updates, and publishing are planned for later releases.
- Rendered pipeline stages are descriptive output for operator or agent review, not a promise of portable or safe shell execution.

## Verification

- `make test` passes.
- `go test ./cmd ./internal/... ./tests` passes.
- Release documentation and manifests are updated for `v1.2.0`.
- `release-manifest.json` points to `v1.2.0`.

## Update

Download the latest release from the GitHub releases page or rerun the install
script from `download.reservecli.dev`.

## Additional Release Context

For user-focused release narratives, design rationale, and implementation insights,
visit the [RESERVE Releases](https://reservecli.dev/documentation/releases/)
section of the RESERVE website.
