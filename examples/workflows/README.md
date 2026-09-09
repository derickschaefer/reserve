# Workflow Examples

These files are educational examples shipped with the RESERVE source tree.
They demonstrate workflow metadata, pipelines, transformations, multi-series
input, and runtime contracts.

They are **not** the official RESERVE workflow catalog. The curated public
collection belongs in the separate `reserve-workflows` repository and will be
distributed through the RESERVE workflow API at
`https://api.reservecli.dev/v1`.

## Examples in this directory

- `simple-observation.yaml` — a fixed date range and a single summary stage.
- `transform-pipeline.yaml` — a year-over-year transformation followed by analysis.
- `multi-series-analysis.yaml` — several series summarized independently.
- `parameterized-workflow.yaml` — a two-slot date contract rendered at runtime.

Validate the examples from the repository root with:

```bash
go run . workflow validate examples/workflows/*.yaml
```

Render the parameterized example without executing it:

```bash
go run . workflow render examples/workflows/parameterized-workflow.yaml 2015-01-01 2024-12-31
```

For the repository boundaries and future distribution API, see
[`docs/workflow-repositories.md`](../../docs/workflow-repositories.md).
