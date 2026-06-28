---
layout: page
title: Examples
eyebrow: See it in action
lead: A few representative workflows. The visuals below are interface mockups; drop in real captures under assets/img/ to replace them.
description: Example Helena workflows — sending and asserting, real-time WebSocket, and headless CI runs.
---

## Send a request, assert the response

Type a URL (with {% raw %}`{{variables}}`{% endraw %}), pick an auth scheme, hit **Send**. Add a
post-response check inline or on the no-code Assertions tab.

<figure class="shot">
  <img src="{{ '/assets/img/app-hero.svg' | relative_url }}" alt="The Helena request editor with a Bearer token and a JSON response">
  <figcaption>The request editor: a Bearer token sourced from <code>{% raw %}{{TOKEN}}{% endraw %}</code>, and the JSON response with passing checks.</figcaption>
</figure>

```js
// Post-response script
test("profile looks right", function () {
  expect(response.status).toBe(200);
  expect(response.json.plan).toBe("pro");
  expect(response.json.roles).toContain("admin");
});
```

## Log in once, reuse the token

Chain a login request before the leaf and forward its token — no copy-paste:

{% raw %}
```js
// Login → post-response
helena.env.set("TOKEN", response.json.token);

// Any later request → Auth tab → Bearer
//   Token:  {{TOKEN}}
// …or pull a chained result directly in a script:
request.headers["Authorization"] = "Bearer " + chain.login.response.json.token;
```
{% endraw %}

## Stream a WebSocket feed

Enter a `wss://` URL and press **Send** — Helena opens a live session. Type
messages to send; received messages stream into the transcript. Pings are
answered automatically and fragmented messages are reassembled.

<figure class="shot">
  <img src="{{ '/assets/img/shot-websocket.svg' | relative_url }}" alt="A Helena WebSocket session with a streaming message transcript">
  <figcaption>A WebSocket session: <code>→</code> messages you sent, <code>←</code> messages received.</figcaption>
</figure>

## Run a collection in CI

Run the whole collection headlessly — same resolution, scripts, and assertions
as the GUI — and gate your pipeline on the exit code:

<figure class="shot">
  <img src="{{ '/assets/img/shot-runner.svg' | relative_url }}" alt="helena run executing a collection in a terminal with pass/fail/skip results">
  <figcaption><code>helena run</code> in CI: per-request status, checks, and a summary line.</figcaption>
</figure>

```bash
helena run ./collections/acme --env Staging
# exits non-zero if any request errors or any check fails
```

> **Note** — these illustrations are SVG mockups so the site renders without a
> running app. Replace them with real PNG/GIF captures in
> `website/assets/img/` (the repo tracks a capture guide in `docs/media/`) and
> the figures pick them up.
