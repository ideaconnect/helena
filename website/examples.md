---
layout: page
title: Examples
eyebrow: See it in action
lead: A few representative workflows, captured from the real app running against a local demo API.
description: Example Helena workflows - composing requests, authentication, variables, chaining, real-time WebSocket, and headless CI runs.
---

## Compose and send a request

Pick a method, type a URL (with {% raw %}`{{variables}}`{% endraw %}), fill in a
body, hit **Send**. The response panel pretty-prints JSON with structured
folding, search, and a raw view - and shows the status, size, and timing.

<figure class="shot">
  <img src="{{ '/assets/img/app-hero.png' | relative_url }}" alt="The Helena desktop app: a collection sidebar, a POST request with a JSON body, and the 201 Created response">
  <figcaption>A <code>POST</code> with a JSON body and the <code>201&nbsp;Created</code> response, alongside the collection sidebar.</figcaption>
</figure>

## Authenticate, nine schemes

Open the **Auth** tab and choose a scheme - Basic, Bearer, API Key, OAuth&nbsp;2.0
(incl. auth-code + PKCE), OAuth&nbsp;1.0a, WSSE, AWS&nbsp;SigV4, Digest, or NTLM.
Credentials can come from {% raw %}`{{variables}}`{% endraw %}, so secrets stay
out of the collection YAML.

<figure class="shot">
  <img src="{{ '/assets/img/shot-auth.png' | relative_url }}" alt="The Helena Auth tab set to Bearer Token, with the token sourced from a variable, and a 200 OK response">
  <figcaption>A Bearer token sourced from <code>{% raw %}{{TOKEN}}{% endraw %}</code> - resolved on the wire, never written to disk.</figcaption>
</figure>

```js
// Post-response script
test("profile looks right", function () {
  expect(response.status).toBe(200);
  expect(response.json.role).toBe("admin");
  expect(response.json.tags).toContain("founder");
});
```

## Headers, variables & a structured response

Add request headers (with {% raw %}`{{$guid}}`{% endraw %} and other dynamic
values), then read the response back as a folding JSON tree with search.

<figure class="shot">
  <img src="{{ '/assets/img/shot-request.png' | relative_url }}" alt="The Helena Headers tab with an Accept and an X-Request-Id header, and a nested JSON response">
  <figcaption>Request headers including a dynamic <code>{% raw %}{{$guid}}{% endraw %}</code>, and a nested JSON response.</figcaption>
</figure>

## Chain requests together

A request can run other requests **first**. On the **Chain** tab, name a prior
request by its path and give it an alias; Helena runs it before the leaf and
binds the result as `chain.<alias>` for your headers, body, and scripts. So you
can log in and place an order in a single Send - no copy-paste, no juggling
tokens by hand.

<figure class="shot">
  <img src="{{ '/assets/img/shot-chain.png' | relative_url }}" alt="Helena's Chain tab: a Place order request that runs Auth/Login first (aliased auth), with the 201 Created order response">
  <figcaption>"Place order" runs "Auth/Login" first (aliased <code>auth</code>); the order then reuses the login token and comes back 201 Created.</figcaption>
</figure>

Reference the chained response anywhere a {% raw %}`{{variable}}`{% endraw %} works - here, the
token straight into an Authorization header:

{% raw %}
```
Authorization: Bearer {{chain.auth.response.json.token}}
```
{% endraw %}

...or reach it from a script:

{% raw %}
```js
// Pull a value out of an earlier request's response
const token = chain.auth.response.json.token;

// Or stash it from the login's own post-response script, then use it by name
helena.env.set("TOKEN", response.json.token);   // -> {{TOKEN}} anywhere
```
{% endraw %}

## Stream a WebSocket feed

Enter a `wss://` URL and press **Send** - Helena opens a live session. Type
messages to send; received messages stream into the transcript. Pings are
answered automatically and fragmented messages are reassembled. Server-Sent
Events work the same way: a `text/event-stream` response appends events live.

## Run a collection in CI

Run the whole collection headlessly - same resolution, scripts, and assertions
as the GUI - and gate your pipeline on the exit code:

```bash
helena run ./collections/acme --env Staging
# exits non-zero if any request errors or any check fails
```

```text
ok    Users/Get user               GET    200  3ms
ok    Users/Create user            POST   201  5ms
        PASS  profile looks right
2 requests, 1 checks passed, 0 failed
```

> The screenshots above are real captures of the app rendered against a local
> demo API - regenerate them any time with `make screenshots` (see
> `website/README.md`).
