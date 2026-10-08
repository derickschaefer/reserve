You are generating `RELEASE_NOTES.md` for the RESERVE CLI project.

Use `release-manifest.json` as the single source of truth. The manifest is consumed by `make dist` and copied into `dist/releases/<VERSION>/release.json`.

Requirements:
- Preserve all factual statements from `release-manifest.json`.
- Do not invent features not present in the manifest.
- Use `latest_version` for the title/version.
- Use `summary` as the opening paragraph.
- Expand `highlights` into concise, readable bullets.
- Group bullets under:
  - `## Added`
  - `## Changed`
  - `## Fixed`
  - `## Security`
  - `## Known Limitations`
  - `## Verification`
- Omit empty sections (`Added`/`Changed`/`Fixed`/`Security`/`Known Limitations`) if there are no applicable items.
- Use inline code formatting for commands, flags, files, and JSON keys.
- Output only final Markdown release notes.
- No emojis, no marketing copy.

Output skeleton:

```markdown
# RESERVE <VERSION>

<SUMMARY PARAGRAPH>

## Added

- ...

## Changed

- ...

## Fixed

- ...

## Security

- ...

## Known Limitations

- ...

## Verification

- `make test` passes
- `go test ./cmd ./internal/... ./tests` passes
- Release docs/manifests updated for `<VERSION>`
- `release-manifest.json` now points to `<VERSION>`

## Additional Release Context

For user-focused release narratives, design rationale, and implementation insights, visit the [RESERVE Releases](https://reservecli.dev/documentation/releases/) section of the RESERVE website.
```

Input `release-manifest.json`:

```json
<PASTE release-manifest.json HERE>
```
