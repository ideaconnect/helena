---
layout: page
hero_image: /assets/img/shot-auth-fancy.png
hero_alt: Helena's Auth tab with a Bearer token and a JSON response
title: Roadmap
eyebrow: Where Helena is going
lead: Most of the client is already shipped. Here's the picture by track, and what's still planned.
description: Helena's roadmap - shipped tracks, in-progress work, and what's planned next.
---

<div style="margin:8px 0 30px">
  <div class="meter"><span style="width:67%"></span></div>
  <div class="meter-label"><span>8 of 12 tracks shipped</span><span>67%</span></div>
</div>

<ul class="timeline">

  <li class="tl-item done"><span class="tl-dot"></span>
    <div class="tl-card">
      <div class="tl-head"><h2>Core request &amp; response</h2><span class="badge shipped">Shipped</span></div>
      <p>Method, URL, query, headers, and bodies - JSON, XML, text, GraphQL, form, multipart, file. Pretty/raw response views, Validate/Format, and the native CORS advisory.</p>
      <div class="chips"><span class="chip">GraphQL mode</span><span class="chip">file bodies</span><span class="chip">CORS advisory</span></div>
    </div>
  </li>

  <li class="tl-item done"><span class="tl-dot"></span>
    <div class="tl-card">
      <div class="tl-head"><h2>Variables &amp; environments</h2><span class="badge shipped">Shipped</span></div>
      <p>A layered scope chain (global → collection .env → collection → environment → folder → request → script overlay), dynamic <code>{% raw %}{{$guid}}{% endraw %}</code>/<code>{% raw %}{{$timestamp}}{% endraw %}</code> values, and ask-at-send <code>{% raw %}{{?Name}}{% endraw %}</code> prompts, with a live resolved-URL preview.</p>
    </div>
  </li>

  <li class="tl-item done"><span class="tl-dot"></span>
    <div class="tl-card">
      <div class="tl-head"><h2>Authentication - nine schemes</h2><span class="badge shipped">Shipped</span></div>
      <p>Basic, Bearer, API Key, OAuth 2.0 (+ PKCE), OAuth 1.0a, WSSE, AWS SigV4, Digest, and NTLM - with secrets externalized out of the committed YAML.</p>
      <div class="chips"><span class="chip">OAuth 2.0 + PKCE</span><span class="chip">AWS SigV4</span><span class="chip">Digest</span><span class="chip">NTLM</span></div>
    </div>
  </li>

  <li class="tl-item done"><span class="tl-dot"></span>
    <div class="tl-card">
      <div class="tl-head"><h2>Scripting, tests &amp; chaining</h2><span class="badge shipped">Shipped</span></div>
      <p>Pre/post JavaScript with the curated <code>helena.*</code> API (env, interpolate, sendRequest, cookies, hash, sleep, runner control), a <code>test()</code>/<code>expect()</code> framework, no-code assertions, and request chaining.</p>
    </div>
  </li>

  <li class="tl-item done"><span class="tl-dot"></span>
    <div class="tl-card">
      <div class="tl-head"><h2>Collections &amp; headless runner</h2><span class="badge shipped">Shipped</span></div>
      <p>Workspaces, collections, folders; an in-app runner and a <code>helena run</code> CLI for CI with the same resolution, scripts, and assertions as a GUI Send. Run a whole collection or a single folder, and emit JSON / JUnit reports for CI dashboards.</p>
      <div class="chips"><span class="chip">folder-scoped runs</span><span class="chip">JSON / JUnit reports</span></div>
    </div>
  </li>

  <li class="tl-item done"><span class="tl-dot"></span>
    <div class="tl-card">
      <div class="tl-head"><h2>Import / export / codegen</h2><span class="badge shipped">Shipped</span></div>
      <p>Import OpenAPI / Swagger / WSDL / Postman; export to cURL, wget, fetch, Python, or Go; paste-cURL to build a request.</p>
    </div>
  </li>

  <li class="tl-item done"><span class="tl-dot"></span>
    <div class="tl-card">
      <div class="tl-head"><h2>Real-time - SSE &amp; WebSocket</h2><span class="badge shipped">Shipped</span></div>
      <p>Server-Sent Events streamed into the response view, and a bidirectional WebSocket session UI - both hand-rolled on the standard library, the WebSocket frame codec pinned to RFC 6455's own test vectors.</p>
    </div>
  </li>

  <li class="tl-item done"><span class="tl-dot"></span>
    <div class="tl-card">
      <div class="tl-head"><h2>Documentation</h2><span class="badge shipped">Shipped</span></div>
      <p>This site plus reference docs and a per-module design-notes set living alongside the code.</p>
    </div>
  </li>

  <li class="tl-item active"><span class="tl-dot"></span>
    <div class="tl-card">
      <div class="tl-head"><h2>Packaging &amp; app stores</h2><span class="badge progress">In progress</span></div>
      <p>Today there's a native binary for every release (with checksums, an SBOM, and build provenance) and Linux <code>.deb</code>/<code>.rpm</code> packages. Windows winget/Scoop manifests and an Inno Setup installer script are committed but not yet wired into the release pipeline. macOS is built in CI but not yet signed, and CI already builds the (unsigned) Microsoft Store MSIX bundle - only the Store submission remains. Next, so each OS can install and update Helena the way it expects:</p>
      <ul style="margin:8px 0 0;padding-left:18px;color:var(--muted);font-size:14px">
        <li><b>Microsoft Store</b> (MSIX) on Windows</li>
        <li><b>Mac App Store</b> and a <b>Homebrew</b> cask on macOS, signed and notarized</li>
        <li><b>Flatpak / Flathub</b> (the AppStream metadata already lives in the repo), plus AppImage, on Linux</li>
      </ul>
      <div class="chips"><span class="chip">.deb / .rpm</span><span class="chip">winget</span><span class="chip">Scoop</span><span class="chip">Microsoft Store</span><span class="chip">Mac App Store</span><span class="chip">Flatpak</span><span class="chip">Homebrew</span><span class="chip">AppImage</span></div>
    </div>
  </li>

  <li class="tl-item active"><span class="tl-dot"></span>
    <div class="tl-card">
      <div class="tl-head"><h2>UI beautification</h2><span class="badge progress">In progress</span></div>
      <p>An ongoing visual refresh: the green accent, the Inter / JetBrains Mono type, the icon sidebar toolbar, and the reworked top bar are in; refined tabs and general polish are next - so Helena looks as good as it works.</p>
      <div class="chips"><span class="chip">green accent</span><span class="chip">custom fonts</span><span class="chip">icon toolbar</span><span class="chip">tab polish</span></div>
    </div>
  </li>

  <li class="tl-item"><span class="tl-dot"></span>
    <div class="tl-card">
      <div class="tl-head"><h2>gRPC support</h2><span class="badge planned">Planned</span></div>
      <p>Unary + streaming calls from <code>.proto</code> definitions. Unlike SSE/WebSocket this can't be hand-rolled on the standard library, so it's gated on a deliberate dependency decision (grpc-go + protobuf).</p>
    </div>
  </li>

  <li class="tl-item"><span class="tl-dot"></span>
    <div class="tl-card">
      <div class="tl-head"><h2>Internationalization</h2><span class="badge planned">Planned</span></div>
      <p>An i18n seam and accessible request/response text, so the UI can be localized.</p>
    </div>
  </li>

</ul>

<p style="color:var(--muted)">Have a request? <a href="{{ site.repo }}/issues">Open an issue →</a></p>
