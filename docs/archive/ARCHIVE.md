# RESERVE Documentation Archive

This directory contains historical or superseded documentation that remains
useful for release context, design history, or auditability but is not the
current source of truth for active development.

Active documentation belongs in the repository root, `docs/`, or the relevant
subsystem directory. When an active document supersedes an archived document,
the active document should link here rather than duplicating the historical
material.

| Archived file | Reason archived | Current source of truth |
|---|---|---|
| [`DEVPLAN.md`](DEVPLAN.md) | Original phased implementation plan; the actual release sequence has moved beyond this plan. | [`CHANGELOG.md`](../../CHANGELOG.md), current code, and the v1.2.3 API decision register |
| [`v1.2.1-status.md`](v1.2.1-status.md) | Historical checkpoint for the v1.2.1 workflow-repository foundation. | [`workflow-repositories.md`](../workflow-repositories.md) |
| [`v1.2.2-preview.md`](v1.2.2-preview.md) | Historical description of the v1.2.2 read-only workflow API preview. | [`workflow-repositories.md`](../workflow-repositories.md) and [`v1.2.3-api-decisions.md`](../v1.2.3-api-decisions.md) |
| [`release-notes-template.md`](release-notes-template.md) | Release-writing instructions retained for reference rather than treated as project status. | `release-manifest.json` and the release process |

## Archive policy

- Do not use archived files as implementation authority without checking the
  active documents and current code.
- Preserve archived files when they explain a release boundary or design
  decision; remove only true duplicates or obsolete scratch material.
- When a document is archived, update active links and add the replacement or
  current status to this index.
