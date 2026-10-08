# Workflow Repository Architecture

RESERVE keeps workflow education, authorship, curation, and distribution
distinct. This prevents source-tree examples from becoming an accidental
catalog and leaves room for private repositories later.

## Repository boundaries

| Source | Responsibility | Current status |
| --- | --- | --- |
| `examples/workflows/` | Small, readable examples shipped with the CLI source | In this repository; not a catalog |
| Local `personal` workflows | User-created and locally installed workflows | Supported by the local workflow store |
| `reserve-workflows` | Canonical, curated public workflow collection | v1.2.2 read-only preview published; v1.2.3 authentication design in progress |
| Private repositories | User- or organization-controlled workflow collections | Future; authorization and publishing decisions are tracked for v1.2.3 |

The official repository is identified internally as `official`, with the
display name `reserve-workflows`. Its permanent API base is
`https://api.reservecli.dev/v1`. The CLI contract is the RESERVE repository
abstraction and that API boundary, not a GitHub URL, branch layout, or object
storage bucket.

The API is live as a Cloudflare Worker at `https://api.reservecli.dev/v1`.
Health, catalog, collection, and versioned workflow reads are available for the
v1.2.2 preview. Public browse behavior and the authenticated route boundary are
being finalized for v1.2.3; the official repository descriptor currently uses
the initial Bearer-key bootstrap mechanism.

## Distribution boundary

The intended future flow is:

```text
reserve-workflows (curated source)
        |
        | publish / build
        v
RESERVE workflow API
        |
        v
reserve workflow browse / get / install / update
```

The API may use static objects, R2, indexes, or another implementation behind
that boundary. Those details are deliberately not part of the workflow file
format or the CLI contract.

The current read-only API boundary is:

```text
GET /v1/browse
GET /v1/collections/{collection}
GET /v1/workflows/{collection}/{workflow}
GET /v1/workflows/{collection}/{workflow}/{version}
```

Remote installation, publishing, ratings, search infrastructure, and remote
execution remain outside the v1.2.2 preview. Authentication, FRED-key
validation, token lifecycle, and authorization are v1.2.3 design work tracked
in [`docs/v1.2.3-api-decisions.md`](v1.2.3-api-decisions.md).
