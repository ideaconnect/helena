# Authentication

Every request has an **Auth** tab. Pick a scheme from the *Type* dropdown and
fill in the fields. Auth set on a **folder** or the **collection** is inherited
by descendants whose own type is *Inherit from parent*; the Inherit panel shows
what would actually be applied.

All credential fields run through `{{variable}}` resolution at send time, so you
can store a token as `{{TOKEN}}` and keep the value in an environment variable.

!!! tip "Secrets stay out of your YAML"
    Password / token / secret-key fields are **not** written into the
    git-tracked collection YAML. On save they are externalized to a
    per-collection store under your OS config dir (or `$HELENA_SECRETS_DIR`) and
    merged back on load — so you can version-control a collection without
    committing cleartext credentials.

A request header you set yourself always wins over an auth-derived header, so
manual escape hatches keep working.

## None / Inherit

*None* sends no credentials. *Inherit from parent* walks up the
folder → collection chain and applies the nearest configured auth.

## Basic Auth

Username + password, sent as `Authorization: Basic base64(user:pass)`.

## Bearer Token

A token sent as `Authorization: Bearer <token>`. Commonly `{{TOKEN}}` populated
by a login request's post-response script (see [Scripting](scripting.md)).

## API Key

A `name`/`value` pair placed either in a **header** or the **query string**.

## OAuth 2.0

Helena fetches and caches tokens for you. Two grants are supported:

- **Client Credentials** — server-to-server. Fill the token URL, client id, and
  client secret (plus optional scope / audience).
- **Authorization Code** — opens your browser, runs the redirect on an ephemeral
  localhost listener, verifies the CSRF state, and exchanges the code. Tick
  **Use PKCE** for the SHA-256 challenge.

Tokens are cached per collection + config and reused until just before expiry.
*Clear cached tokens* drops them for the active collection.

## OAuth 1.0a

Two- or three-legged OAuth 1.0a with **HMAC-SHA1** request signing (RFC 5849).
Fill the consumer key/secret and optional token/token-secret; each send computes
a signature over the request and emits an `Authorization: OAuth …` header.

## WS-Security (WSSE)

WS-Security `UsernameToken`. Each send generates a fresh nonce + timestamp and
emits an `X-WSSE` header whose `PasswordDigest` is
`Base64(SHA1(nonce + created + password))`.

## AWS Signature v4

Signs the request with **AWS4-HMAC-SHA256**. Fill the access key id, secret
access key, region, and service (a session token is optional, for temporary STS
credentials). Helena builds the canonical request, derives the signing key, and
emits the `Authorization` + `X-Amz-Date` headers (and `X-Amz-Security-Token`
when a session token is set).

## HTTP Digest

Challenge/response digest auth (RFC 7616). Helena sends the request
unauthenticated, reads the server's `WWW-Authenticate: Digest` challenge, and
retries with the computed response (MD5 or SHA-256, `qop=auth`). Just fill the
username + password.

## NTLM

NTLMv2, the multi-round Windows scheme: Helena runs the
NEGOTIATE → CHALLENGE → AUTHENTICATE handshake over a single keep-alive
connection. Fill the username + password and optional domain / workstation.

!!! info "Lean dependencies"
    The crypto these schemes need that Go's standard library omits — notably
    **MD4** for NTLM — is implemented from the spec inside Helena rather than
    pulled in as a third-party dependency, and pinned to the published
    test vectors (RFC 1320, MS-NLMP, RFC 5849, RFC 7616, the AWS SigV4 suite).
