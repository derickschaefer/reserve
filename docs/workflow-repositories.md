# Workflow Repository Architecture

RESERVE keeps workflow education, authorship, curation, and distribution
distinct. This prevents source-tree examples from becoming an accidental
catalog and leaves room for private repositories later.

## Repository boundaries

| Source | Responsibility | v1.2.1 status |
| --- | --- | --- |
| `examples/workflows/` | Small, readable examples shipped with the CLI source | In this repository; not a catalog |
| Local `personal` workflows | User-created and locally installed workflows | Supported by the local workflow store |
| `reserve-workflows` | Canonical, curated public workflow collection | Defined as the built-in official repository |
| Private repositories | User- or organization-controlled workflow collections | Future; no auth or publishing yet |

The official repository is identified internally as `official`, with the
display name `reserve-workflows`. Its permanent API base is
`https://api.reservecli.dev/v1`. The CLI contract is the RESERVE repository
abstraction and that API boundary, not a GitHub URL, branch layout, or object
storage bucket.

As of the v1.2.1 stopping point, the API is live as a Cloudflare Worker. The
public health endpoint is `GET https://api.reservecli.dev/v1/health`, while the
official repository descriptor is protected by the initial Bearer-key
bootstrap mechanism.

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
reserve workflow browse / install / update
```

The API may use static objects, R2, indexes, or another implementation behind
that boundary. Those details are deliberately not part of the workflow file
format or the CLI contract.

The smallest API boundary reserved for a later release is:

```text
GET /workflows
GET /workflows/{id}
GET /workflows/{id}/{version}
```

The health and official-repository descriptor endpoints are deployed as the
initial service checkpoint. Remote workflow browsing, installation, publishing,
ratings, search infrastructure, caching, and remote execution remain outside
v1.2.1.
