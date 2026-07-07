---
layout: page
hero_image: /assets/img/shot-request-fancy.png
hero_alt: Helena showing request headers and a nested JSON response
title: Download &amp; install
eyebrow: Get Helena
lead: Grab a pre-built binary, or build from source in a couple of commands.
description: Download pre-built Helena binaries or build from source on Linux, Windows, or macOS.
---

**Helena is free and open source, and it's meant to stay that way.** It's built
by a single developer, with a good deal of help from AI - nothing to buy, no
paywall, and no feature held back.

## Pre-built binaries

Pre-built **Linux (amd64)**, **Windows (amd64 &amp; arm64)**, and **macOS
(arm64)** binaries are attached to each release.

<p><a class="btn btn-primary" href="{{ site.releases }}">Releases on GitHub →</a></p>

Helena is a single self-contained binary - no installer, no runtime to set up.
Download it, make it executable, and run it.

> **macOS builds are unsigned**, for two entirely mundane reasons: I don't have
> a (paid) Apple Developer account, and I don't own a Mac to notarize on. The
> binaries are still built and tested in CI - you'll just need to allow them past
> Gatekeeper (right-click the app &rarr; **Open**). If you'd like to sponsor
> code-signing and notarization - or a Mac to do it on - I'm more than open to it
> (see [Support the project](#support-the-project) below). More detail in
> [PACKAGING]({{ site.repo }}/blob/main/docs/PACKAGING.md).

> Release builds omit Fyne's bundled colour-emoji font to cut resident memory
> (~75 MB): colour emoji in response bodies render as blank glyphs; all other
> text is unaffected. Build from source without `-tags no_emoji` if you want
> colour emoji.

Helena never checks for updates at runtime (part of its no-background-traffic
guarantee) - update by re-downloading a release or via a package manager as
those land. Run `helena --version` to see your build.

## Support the project

Helena is free - none of this is required, and nothing is locked behind it. But
if it's useful to you and you'd like to chip in toward its development, thank you.
A few entirely voluntary ways:

- **Microsoft Store** - a minimal-price listing for a one-click install that also
  supports the work *(coming soon)*.
- **Buy Me a Coffee** - a one-off tip at [buymeacoffee.com/idct]({{ site.coffee }}).
- **GitHub Sponsors** - recurring support at [github.com/sponsors/ideaconnect]({{ site.sponsor }}).

Sponsorship also directly unblocks things like **signed macOS builds** (see the
note above).

<p style="margin-top:18px">
  <a class="btn btn-primary" href="{{ site.coffee }}">Buy me a coffee</a>
  <a class="btn btn-ghost" href="{{ site.sponsor }}">GitHub Sponsors</a>
</p>

## Build from source

Building Helena yourself is free, forever, on every platform. The quick version
is below; the full copy-paste guide (per-distro dependencies, release-grade
build flags, cross-arch notes, troubleshooting) is in
[BUILDING]({{ site.repo }}/blob/main/docs/BUILDING.md).

**Requirements**

- **Go 1.26+** (the toolchain is pinned in `go.mod`).
- A **C compiler** - Fyne uses cgo + OpenGL.
- **Linux:** `sudo apt-get install -y build-essential libgl1-mesa-dev xorg-dev`
- **Windows:** TDM-GCC or MSYS2 mingw-w64 on `PATH`
- **macOS:** Xcode Command Line Tools (`xcode-select --install`)

**Linux / macOS / WSL**

```sh
git clone https://github.com/ideaconnect/helena
cd helena
make build   # → ./bin/helena   (or: go build -o helena ./cmd/helena)
make run     # build & run
make test    # run all tests
```

**Windows** (Cmd or PowerShell)

```cmd
make.bat build   :: writes bin\helena.exe
make.bat run
```

A local `go build` reports version `dev`; released binaries report their tag and
commit. For bug reports, `helena --verbose` raises the log level and
`--log-file PATH` tees logs to a file (credentials are redacted, so logs are
safe to attach).

## Next steps

- [Getting started]({{ site.repo }}/blob/main/docs/USER_GUIDE.md) - your first request.
- [Features]({{ '/features/' | relative_url }}) - what's in the box.
- [Examples]({{ '/examples/' | relative_url }}) - representative workflows.
