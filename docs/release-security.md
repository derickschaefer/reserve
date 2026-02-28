# Release Security

`reserve` release artifacts are signed with **Sigstore cosign keyless signatures**.
No long-lived private signing key is stored in this repository.

## What is published

Each tagged release (`v*`) publishes:

- platform archives (`.tar.gz` / `.zip`)
- `SHA256SUMS` (checksums for archives)
- `SHA256SUMS.sig` (cosign signature)
- `SHA256SUMS.pem` (Fulcio-issued signing certificate)

## Verify a release

From a directory containing release assets:

```bash
sha256sum -c SHA256SUMS
```

Then verify the keyless signature:

```bash
cosign verify-blob \
  --certificate SHA256SUMS.pem \
  --signature SHA256SUMS.sig \
  --certificate-identity-regexp '^https://github.com/derickschaefer/reserve/\.github/workflows/release-keyless\.yml@refs/tags/v[0-9].*$' \
  --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' \
  SHA256SUMS
```

If verification succeeds, your checksums came from the tagged GitHub Actions
release workflow in this repository.
