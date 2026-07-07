---
layout: page
title: About
eyebrow: Who's behind Helena
lead: Helena is built and maintained by Bartosz Pachołek.
description: About Helena and its author, Bartosz Pachołek.
---

## Named after Helena

<div class="tribute-grid">
  <div class="tribute-copy">
    <p class="tribute-text">Helena is named after our cat, <strong>Helena</strong> — a gentle tabby and our great friend for almost nineteen years. She passed away on the second day of Christmas, 2025. The app carries her name so that a little of her stays with us.</p>
  </div>
  <div class="tribute-box">
    <figure class="cat-win cat-win-back">
      <picture>
        <source srcset="{{ '/assets/img/helena-cat-1.webp' | relative_url }}" type="image/webp">
        <img src="{{ '/assets/img/helena-cat-1.jpg' | relative_url }}" alt="Helena the cat, stretched out and relaxed on a wooden shelf" width="900" height="900" loading="lazy" decoding="async">
      </picture>
    </figure>
    <figure class="cat-win cat-win-front">
      <picture>
        <source srcset="{{ '/assets/img/helena-cat-2.webp' | relative_url }}" type="image/webp">
        <img src="{{ '/assets/img/helena-cat-2.jpg' | relative_url }}" alt="Helena, a green-eyed tabby cat, sitting and looking at the camera" width="767" height="900" loading="lazy" decoding="async">
      </picture>
    </figure>
  </div>
</div>

## Why Helena exists

Most desktop API clients are capable, but they ship a browser engine inside
their binaries (hundreds of MB on disk) and store collections in formats that
don't diff cleanly. Helena makes three deliberate trade-offs in the other
direction:

1. **Native, no Electron.** Fyne renders the UI through OpenGL; the binary is
   ~35&nbsp;MB and starts instantly.
2. **Open Collection YAML.** Plain files that diff and merge like any other
   source code, so a collection lives happily in your repo.
3. **Boring, debuggable Go.** No JS sandbox to babysit, no fancy DI framework,
   no exotic concurrency - the whole runtime is `go test`-able.

When an abstraction would fight one of those trade-offs, the trade-off gets
revisited before the abstraction gets added.

## How it's built

Helena is written in **Go** with the **[Fyne](https://fyne.io)** toolkit. The
JavaScript scripting runs on the pure-Go [goja](https://github.com/dop251/goja)
engine. Where a protocol needs crypto the standard library omits - like MD4 for
NTLM - it's implemented from the spec and pinned to published test vectors
rather than pulling in a dependency. Every behaviour-affecting change ships with
tests and docs, and the non-UI packages hold a 90% coverage floor.

## The author

<div class="author-bio">
  <figure class="author-figure">
    <span class="avatar photo"><img src="{{ '/assets/img/author.jpg' | relative_url }}" alt="Bartosz Pachołek" width="256" height="256" loading="lazy" decoding="async"></span>
    <a class="idct-badge" href="{{ site.author_url }}" title="IDCT - idct.tech"><img src="{{ '/assets/img/idct-logo.png' | relative_url }}" alt="IDCT - idct.tech" width="229" height="240" loading="lazy" decoding="async"></a>
  </figure>
  <p>Helena is a solo project by <strong>Bartosz Pachołek</strong> at <a href="{{ site.author_url }}">IDCT</a> — designed, built, and maintained by one developer, with a good deal of help from AI, and kept free and open source. The source lives on <a href="{{ site.repo }}">GitHub</a>; say hello on the <a href="{{ '/contact/' | relative_url }}">contact page</a> or in the <a href="{{ site.discord }}">Discord</a>.</p>
</div>

## Get involved

- ⭐ **Star or fork** the project on [GitHub]({{ site.repo }}).
- 🐛 **Report bugs or request features** via the [issue tracker]({{ site.repo }}/issues).
- 🤝 **Contribute** - see [CONTRIBUTING]({{ site.repo }}/blob/main/CONTRIBUTING.md)
  and [HUMANS.md]({{ site.repo }}/blob/main/HUMANS.md) for the build/test setup
  and the project's invariants.

<p style="margin-top:26px"><a class="btn btn-primary" href="{{ site.releases }}">Download Helena</a> <a class="btn btn-ghost" href="{{ site.repo }}">View source</a></p>
