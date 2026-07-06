# Security Policy

Helena handles credentials (HTTP auth secrets, OAuth2 tokens) and runs a
JavaScript sandbox for pre/post-request scripts, so we take security reports
seriously.

## Reporting a vulnerability

**Please do not open a public issue for security problems.**

Use one of these private channels instead:

- **GitHub private vulnerability reporting** (preferred): on the repository's
  **Security** tab, choose **Report a vulnerability**. This opens a private
  advisory visible only to you and the maintainers.
- **Email**: `bartosz@idct.tech` with a subject starting `[helena-security]`.

Please include:

- a description of the issue and its impact,
- steps to reproduce (a minimal collection / request / script if relevant),
- the Helena version or commit, and your OS.

## Response expectations

This is a small open-source project maintained on a best-effort basis:

- We aim to **acknowledge** a report within **7 days**.
- We aim to provide an initial **assessment** within **30 days**.
- Once a fix is ready we will coordinate a disclosure timeline with you and
  credit you in the release notes unless you prefer to remain anonymous.

## Supported versions

Helena is pre-1.0 and ships from `main`. Security fixes land on `main` and in
the next tagged release; there are no separate long-term support branches yet.

| Version | Supported |
| ------- | --------- |
| latest `main` / latest release | ✅ |
| older releases | ❌ (please upgrade) |

## Scope notes

- Helena makes **no background network requests** — see the *Privacy* section
  of the [README](README.md). Outbound traffic happens only for actions you
  explicitly trigger (sending a request, importing from a URL, fetching an
  OAuth2 token, or clicking *Check for updates* in the status bar).
- Credentials (auth secrets + Secret env vars) are kept out of the git-tracked
  collection YAML — externalized to a per-collection store under your OS config
  dir (or `$HELENA_SECRETS_DIR`) — so a committed collection carries no
  cleartext secret. That store is still plaintext on local disk today; treat
  your config dir like any secrets location. At-rest *encryption* (OS keychain)
  is a tracked follow-up.
