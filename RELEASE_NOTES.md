# RESERVE v1.2.1

Establishes the workflow repository foundations for the RESERVE ecosystem.
This is a plumbing and documentation release: it clarifies where workflows
belong, adds source-shipped examples, and introduces the internal boundary for
the future official `reserve-workflows` catalog.

- Published: 2026-09-08
- Severity: `info`
- Release: [GitHub v1.2.1](https://github.com/derickschaefer/reserve/releases/tag/v1.2.1)
- Installer: [download.reservecli.dev/install.sh](https://download.reservecli.dev/install.sh)

## Added

- Four educational examples under `examples/workflows/`.
- An examples README that explicitly distinguishes source-tree examples from
  the official workflow catalog.
- Repository-boundary architecture documentation for examples, local
  workflows, `reserve-workflows`, and future private repositories.
- A minimal internal descriptor for the built-in official repository:
  `official` / `reserve-workflows`.

## Changed

- The release version is now `v1.2.1`.
- The supported release toolchain is documented as Go 1.27.1.
- The RESERVE workflow API boundary is documented for future distribution work
  without adding remote workflow commands in this release.

## Security

- Workflow pipeline content is never executed by RESERVE v1.2.1.
- `workflow render` does not invoke Bash or another shell.
- Repository-style references cannot traverse or escape the configured workflow root, including through symlinks.
- Pipeline execution remains deferred until secure, deterministic, and cross-platform semantics are defined.

## Known Limitations

- No workflow browse, install, update, or publish commands are included.
- No private-repository authentication, repository management, ratings, search,
  remote execution, or storage-provider implementation is included.
- The examples are not installed into `~/.reserve/workflows/` and are not the
  official catalog.
- Rendered pipeline stages are descriptive output for operator or agent review, not a promise of portable or safe shell execution.

## Verification

- `go test ./...` passes.
- All four example workflows validate successfully.
- `go vet ./...` passes.
- The release distribution build produces platform archives and SHA256SUMS.

## Update

Download the latest release from the GitHub releases page or rerun the install
script from `download.reservecli.dev`.

## Additional Release Context

For user-focused release narratives, design rationale, and implementation insights,
visit the [RESERVE Releases](https://reservecli.dev/documentation/releases/)
section of the RESERVE website.
