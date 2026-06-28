---
layout: page
title: About
eyebrow: Who's behind Helena
lead: Helena is built and maintained by Bartosz Pachołek.
description: About Helena and its author, Bartosz Pachołek.
---

<div class="author" style="margin:6px 0 26px">
  <div class="avatar" style="padding:6px"><img src="{{ '/assets/img/helena-icon.png' | relative_url }}" alt="Helena" style="width:100%;height:100%;object-fit:contain"></div>
  <div>
    <h2 style="margin:.1em 0">Bartosz Pachołek</h2>
    <p style="margin:0;color:var(--muted)">Author &amp; maintainer · <a href="{{ site.author_url }}">IDCT (idct.tech)</a> · <a href="{{ site.repo }}">ideaconnect/helena</a></p>
  </div>
</div>

## Why Helena exists

Postman and Bruno are great, but they ship a browser engine inside their
binaries (hundreds of MB on disk) and store collections in formats that don't
diff cleanly. Helena makes three deliberate trade-offs in the other direction:

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

## Get involved

- ⭐ **Star or fork** the project on [GitHub]({{ site.repo }}).
- 🐛 **Report bugs or request features** via the [issue tracker]({{ site.repo }}/issues).
- 🤝 **Contribute** - see [CONTRIBUTING]({{ site.repo }}/blob/main/CONTRIBUTING.md)
  and [HUMANS.md]({{ site.repo }}/blob/main/HUMANS.md) for the build/test setup
  and the project's invariants.

<p style="margin-top:26px"><a class="btn btn-primary" href="{{ site.releases }}">Download Helena</a> <a class="btn btn-ghost" href="{{ site.repo }}">View source</a></p>
