# Workflow YAML Format

The v1.2 workflow ecosystem uses YAML documents wrapped in top-level keys.

Workflow documents use `workflow:` and collection manifests use `collection:`.

The filesystem layout is collection-oriented:

```text
~/.reserve/workflows/
  personal/
    my-analysis/
      collection.yaml
      README.md
      workflows/
        recession-study.yaml
  official/
    inflation/
      collection.yaml
      README.md
      workflows/
        cpi-dashboard.yaml
```

Collections are the installation unit. Workflows are the reusable analysis unit.
Within this layout, `documentation` paths are relative to the collection
directory. Collection creation scaffolds `README.md` without replacing an
existing file.

A workflow document looks like this:

```yaml
workflow:
  title: Inflation Dashboard
  summary: Repeatable CPI analysis

  difficulty: Beginner
  estimated_runtime: 5 seconds

  categories:
    - inflation

  concepts:
    - CPI
    - FRED
    - Time Series

  outputs:
    - chart
    - json

  requires_network: true
  reserve_version: ">=1.2"

  author: Derick Schaefer
  documentation: README.md
  contract:
    - label: start date
      format: YYYY-MM-DD
      sample: 2020-01-01
      description: First date in the requested range
    - label: end date
      format: YYYY-MM-DD
      sample: 2024-12-31
      description: Final date in the requested range
  pipeline:
    - printf 'hello\n'
```

Dynamic workflows use positional placeholders in the pipeline:

- `@1` is the first runtime value
- `@2` is the second runtime value
- `@X` is the `X`th runtime value

Hard-coded workflows simply omit `contract` and use no placeholders.

In v1.2.0, `workflow render` resolves positional values, joins the declared
stages with ` | `, and prints one copy-ready pipeline command. RESERVE does not
execute that command.

The runtime contract is meant to read like a small spec block when printed:

- label names the slot
- format explains the expected shape
- sample gives a concrete example value
- description explains how the slot is used

Normalization rules for the v1.2 foundation:

- `categories` and `outputs` are treated as lowercase slugs.
- `concepts` preserve display casing but are deduplicated case-insensitively.
- surrounding whitespace is trimmed from all scalar fields.
- `pipeline` entries are trimmed and empty steps are dropped.
- Each `pipeline` entry is one pipe stage; `workflow render` joins the stages with ` | `.
- `contract` entries are trimmed and empty slots are dropped.
- empty workflow documents are rejected.
- collection manifests are trimmed and empty collection documents are rejected.

Validation and compatibility rules in v1.2.0:

- Unknown YAML fields and multiple YAML documents in one file are rejected.
- `reserve_version` supports one comparison: `>=`, `>`, `<=`, `<`, `=`, `==`, or a bare exact version.
- Versions may use `X.Y` or `X.Y.Z`, with an optional leading `v`.
- `workflow validate`, `workflow render`, and `workflow contract` reject workflows that are incompatible with the running RESERVE version.
- Repository, collection, and workflow names may contain letters, numbers, dots, underscores, and hyphens, and must begin with a letter or number.
- Repository-style references are confined to the configured workflow root and cannot escape it through traversal or symlinks.
- Explicit `.yaml` and `.yml` file paths remain supported for local authoring and inspection outside the workflow store.

This format is intentionally small for v1.2.0 and leaves room for the later
repository, collection, and pipeline layers described in `WORKFLOWS.md`.
