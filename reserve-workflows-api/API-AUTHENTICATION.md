# RESERVE API Authentication Design

**Target release:** v1.2.3  
**Status:** Design specification  
**Scope:** RESERVE Workflows API authentication and enrollment

## 1. Purpose

RESERVE needs a lightweight authentication model for future API-backed workflow services. The design remains consistent with the RESERVE CLI: CLI-first, low-friction, minimal web dependency, minimal account-management overhead, no unnecessary SaaS-style portal, and credentials isolated by service.

The core identity model is:

> **Verified email = RESERVE identity**  
> **FRED API key = enrollment qualification and proof of ecosystem participation**  
> **RESERVE-issued token = ongoing RESERVE API authentication**

The FRED API key must never become the RESERVE authentication credential.

## 2. Design principles

### 2.1 CLI-native enrollment

Users should be able to enroll entirely from the CLI. The initial implementation does not require a RESERVE account website, passwords, usernames, OAuth providers, dashboards, or browser login sessions. The only required browser interaction is following an email-verification link.

The CLI may read an optional email address from `config.json`:

```json
{
  "fred_api_key": "...",
  "email": "user@example.com"
}
```

### 2.2 Separate credentials

The FRED key is a credential issued by FRED, not an identifier that RESERVE may safely reuse. RESERVE must not derive tokens from it, expose it in API requests, or accept it as a bearer token.

During enrollment, the service validates the submitted FRED key with an innocuous FRED API request. After validation, it stores only a one-way fingerprint such as `SHA-256(FRED API key)` for linkage and duplicate-account checks. The raw FRED key is not stored by the RESERVE service.

RESERVE issues an independent, cryptographically random token, for example:

```text
rsv_<long-random-value>
```

Future API requests use:

```http
Authorization: Bearer rsv_<token>
```

### 2.3 Minimal identity record

The initial service-side record should contain only what is needed:

```text
user_id
email
email_verified
fred_key_fingerprint
created_at
updated_at
last_seen_at
status
token_hash
token_created_at
token_revoked_at
```

Token hashes, not usable plaintext tokens, should be retained server-side. The CLI stores its issued token using the platform's secure credential storage where available, with restrictive file permissions as a fallback.

## 3. Enrollment flow

The primary command is conceptually:

```text
reserve auth register
```

The flow is:

1. Read the FRED API key and email address from configuration, prompting for missing values.
2. Validate the email syntax and normalize it for comparison without changing the user's display form.
3. Validate the FRED key with a harmless FRED request. Do not log the key or include it in error messages.
4. Generate a short-lived, single-use email-verification challenge. Store only a hash of the challenge, its expiry, purpose, and rate-limit metadata.
5. Send the verification link through the planned Cloudflare email service.
6. Have the user open the link in a browser.
7. On successful verification, create or activate the minimal RESERVE identity record.
8. Generate an independent random RESERVE token, store its hash, and return the token to the CLI over the enrollment response.
9. Install the token in the CLI's RESERVE credential store and report success.

Illustrative interaction:

```text
$ reserve auth register

RESERVE registration
Email: user@example.com
FRED API: validating...
FRED API: valid
Verification email sent. Check your inbox and follow the link.

After verification, run:
    reserve auth login
```

The verification endpoint must not reveal whether an email or FRED fingerprint already exists. Responses should be intentionally uniform to reduce account enumeration.

## 4. Login, status, and lifecycle

The CLI should eventually provide a small lifecycle surface:

```text
reserve auth register
reserve auth login
reserve auth status
reserve auth logout
reserve auth revoke
reserve auth rotate
```

`login` completes or resumes a verified enrollment and installs a token. `status` reports local authentication state without revealing secrets. `logout` removes the local token only. `revoke` invalidates the server-side token or identity as authorized. `rotate` replaces a token and invalidates the prior one.

Tokens should have an explicit lifetime and revocation model. A future refresh-token design may be added if long-lived access is required, but it should not be introduced until the threat model and storage behavior are documented.

## 5. API behavior

Unauthenticated endpoints should be limited to the minimum enrollment and verification operations. Workflow discovery and other protected operations require a valid RESERVE bearer token.

The API should:

- accept tokens only over HTTPS;
- validate token hashes using constant-time-safe handling;
- return consistent `401` responses for missing, malformed, expired, or revoked credentials;
- use `403` only when an authenticated identity lacks authorization for a resource;
- avoid returning whether an email, FRED fingerprint, or token exists;
- apply per-IP, per-email, per-fingerprint, and per-token rate limits as appropriate;
- include request IDs for support without logging secrets.

The authentication service authenticates the caller; authorization for public, private, and repository-scoped workflows is a separate concern. Possession of a valid token must not automatically grant access to private workflow repositories.

## 6. Cloudflare responsibilities

The Cloudflare edge/service layer may provide the email delivery integration, verification endpoint, API routing, rate limiting, and storage bindings. It must not persist raw FRED API keys or plaintext RESERVE tokens.

Email verification challenges must be short-lived, single-use, scoped to enrollment, and resistant to replay. Email delivery failures should not disclose account state. Operational secrets such as email-service credentials must be managed as deployment secrets, not repository configuration.

## 7. Security and privacy requirements

- Never log FRED keys, RESERVE tokens, verification challenges, authorization headers, or full verification URLs.
- Redact secrets in CLI diagnostics and server errors.
- Use a cryptographically secure random generator for tokens and challenges.
- Hash verification challenges and token values before persistence.
- Expire challenges promptly and invalidate them after use.
- Rate-limit repeated registration, verification, login, and rotation attempts.
- Provide a safe path for token revocation and recovery if a local token is lost or exposed.
- Minimize retained personal data and document retention/deletion behavior.
- Treat email verification as proof of control of an address, not proof of a person's legal identity.
- Keep FRED-key validation and RESERVE authentication logically separate so a FRED credential compromise does not become a RESERVE-token compromise.

## 8. Compatibility and implementation boundary

This is a v1.2.3 design artifact only. It does not authorize changes to the v1.2.2 RESERVE CLI, existing workflow execution, configuration schema, release behavior, or repository layout beyond adding this document.

When implementation begins, it should introduce authentication behind explicit, reviewable seams and preserve the existing local-first workflow semantics. Remote workflows must ultimately use the same workflow definition and semantics as local CLI workflows rather than a second API-specific language or runtime.

## 9. Open decisions for implementation

Before coding, decide and document:

- the Cloudflare storage product and data-region/retention policy;
- the exact FRED validation endpoint and timeout behavior;
- token format, length, lifetime, scopes, and rotation policy;
- whether one verified email or FRED fingerprint may map to multiple identities;
- the CLI credential-store abstraction for macOS, Linux, and Windows;
- email-template, resend, and support/recovery behavior;
- audit events and retention boundaries;
- the protected API routes and authorization model for public versus private workflows.

