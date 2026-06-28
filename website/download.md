---
layout: page
title: Download &amp; install
eyebrow: Get Helena
lead: Grab a pre-built binary, or build from source in a couple of commands.
description: Download pre-built Helena binaries or build from source on Linux, Windows, or macOS.
---

## Pre-built binaries

Pre-built **Linux (amd64)**, **Windows (amd64)**, and **macOS (arm64)** binaries
are attached to each release.

<p><a class="btn btn-primary" href="{{ site.releases }}">Releases on GitHub →</a></p>

Helena is a single self-contained binary - no installer, no runtime to set up.
Download it, make it executable, and run it.

> macOS binaries are built in CI but **not yet signed / notarized** for
> Gatekeeper - see [PACKAGING]({{ site.repo }}/blob/main/docs/PACKAGING.md).

Helena never checks for updates at runtime (part of its no-background-traffic
guarantee) - update by re-downloading a release or via a package manager as
those land. Run `helena --version` to see your build.

## Build from source

**Requirements**

- **Go 1.23+** (the toolchain is pinned in `go.mod`).
- A **C compiler** - Fyne uses cgo + OpenGL.
- **Linux:** `sudo apt-get install -y libgl1-mesa-dev xorg-dev`
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
