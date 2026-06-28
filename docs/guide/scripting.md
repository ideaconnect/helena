# Scripting & assertions

Each request can carry a **Pre-request** and a **Post-response** script (the
**Scripts** tab). They run JavaScript via the [goja](https://github.com/dop251/goja)
engine — pure Go, no Node. The pre script runs before the request is built (and
can mutate it); the post script runs after the response is read.

The canonical use is logging in once and forwarding a token:

```js
// Post-response hook on a Login request
helena.env.set("TOKEN", response.json.token);
```

`{{TOKEN}}` is then available in any later request for the lifetime of the
process. The session overlay never touches disk.

## The `helena.*` API

| Call | What it does |
| --- | --- |
| `helena.env.get(name)` / `set(name, value)` | Read / write the session env overlay (writes never persist). `helena.vars.get` is an alias for `get`. |
| `helena.interpolate(template)` | Resolve `{{vars}}` in a string with the same scope chain a send uses; reflects `helena.env.set` made earlier in the same script. |
| `helena.sendRequest({url, method, headers, body})` | Fire an ad-hoc HTTP request through the host client (same cookie jar) and get a response object back (`status`, `json`, `headers`, …). |
| `helena.cookies.get(url, name)` / `getAll(url)` | Read the cookie jar for a URL. |
| `helena.runner.stop()` / `skip()` | In a headless `helena run`, halt the run or skip the current request's send. |
| `helena.uuid()` | A random RFC 4122 v4 UUID. |
| `helena.hash.md5 / sha1 / sha256 / sha512(text)` and `hmacSha1 / hmacSha256(key, text)` | Hex digests. |
| `helena.base64.encode / decode(s)` | Standard base64. |
| `helena.date.now()` / `timestamp()` | ISO-8601 string / Unix seconds. |
| `helena.sleep(ms)` | Delay the script (bounded by the script timeout, cancellable). |
| `console.log / info / warn / error(...)` | Append to the Scripts console. |

The pre-request script also gets a mutable `request` (method, url, body,
headers, params); the post-response script gets a read-only `request` plus
`response` (`status`, `headers`, `body`, `json`, `xml`).

## Tests & assertions

Two complementary ways to check a response:

**Scripted** — `test()` / `expect()` inside a script:

```js
test("status is 200", function () {
  expect(response.status).toBe(200);
  expect(response.json.id).toBeDefined();
});
```

Matchers include `toBe`, `toEqual`, `toBeTruthy`/`toBeFalsy`, `toBeNull`,
`toBeDefined`/`toBeUndefined`, `toContain`, `toHaveLength`,
`toBeGreaterThan`/`toBeLessThan`, each negatable via `.not`.

**Declarative** — the no-code **Assertions** tab: rows of
`source` · `operator` · `expected` (e.g. `res.status` `equals` `200`) evaluated
after each send.

Both surface their pass/fail results in the Scripts console and in `helena run`
reports.

## Chaining

A request's **Chain** tab lists other requests to run **first**, each bound to an
alias. Their results are available to this request's scripts and templates as
`chain.<alias>.response.…` — so you can authenticate, then use the token, in one
Send. See the [User guide](../USER_GUIDE.md#request-chaining) for a worked
example.
