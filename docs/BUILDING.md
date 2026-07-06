# Building Helena from source

Helena is free and open source (BSD-4-Clause). Building it yourself is free,
forever, on every platform — the pre-built release binaries are a convenience,
not a paywall. This page is the canonical, copy-paste build guide for Linux,
Windows, and macOS.

If you just want to *run* Helena, grab a binary from the
[releases page](https://github.com/ideaconnect/helena/releases); come back here
when you want to build, hack on, or repackage it.

## What you need

Helena is a Go program with a Fyne GUI, so the toolchain is small but cgo is
mandatory (Fyne draws through OpenGL):

| Requirement | Why | Notes |
| ----------- | --- | ----- |
| **Go 1.26+** | the language toolchain | The exact version is pinned in [`go.mod`](../go.mod)'s `toolchain` line; `setup-go` and a local `go` both honour it. |
| **A C compiler** | Fyne uses **cgo + OpenGL** | GCC or Clang. Without it the build fails at the first cgo file. |
| **Git** | to clone the source | Any recent version. |
| ~2 GB free disk | Go module + build cache | The stripped binary is ~35 MB; the build tree is larger. |

There is **no** Node, Electron, Ruby, or Docker in the build path — those are
only for the website. The app is one static binary.

### Platform-specific prerequisites

=== "Linux"

    Install a C toolchain and the OpenGL / X11 development headers.

    ```sh
    # Debian / Ubuntu
    sudo apt-get install -y build-essential libgl1-mesa-dev xorg-dev

    # Fedora / RHEL
    sudo dnf install -y @development-tools mesa-libGL-devel libXcursor-devel \
      libXrandr-devel libXinerama-devel libXi-devel libXxf86vm-devel

    # Arch
    sudo pacman -S --needed base-devel mesa libxcursor libxrandr libxinerama libxi
    ```

    Wayland-only sessions still build the X11/GLFW backend; it runs fine under
    XWayland. Install Go from your distro or from <https://go.dev/dl/>.

=== "Windows"

    Install a mingw-w64 GCC and put it on `PATH`:

    - **[TDM-GCC](https://jmeubank.github.io/tdm-gcc/)** (simplest), **or**
    - **[MSYS2](https://www.msys2.org/)** then
      `pacman -S mingw-w64-x86_64-gcc` (add `…\mingw64\bin` to `PATH`).

    Install Go from <https://go.dev/dl/>. Verify both tools resolve:

    ```cmd
    go version
    gcc --version
    ```

=== "macOS"

    Install the Xcode Command Line Tools (they provide Clang):

    ```sh
    xcode-select --install
    ```

    Install Go from Homebrew (`brew install go`) or <https://go.dev/dl/>.
    macOS builds and passes the test suite; note that signed / notarized
    distribution is deferred (see [PACKAGING](PACKAGING.md#macos-distribution-deferred-decided-2026-06-16)),
    so building from source is the supported way to run Helena on a Mac today.

## Build it

Clone once, then use the convenience wrapper — `Makefile` on Linux/macOS/WSL,
`make.bat` on Windows. Both expose identical targets.

```sh
git clone https://github.com/ideaconnect/helena
cd helena
```

=== "Linux / macOS / WSL"

    ```sh
    make build    # → ./bin/helena
    make run      # build and launch
    make test     # go test ./...
    ```

=== "Windows"

    ```cmd
    make.bat build   :: → bin\helena.exe
    make.bat run
    make.bat test
    ```

Prefer raw `go`? The wrappers are thin:

```sh
# Development build — keeps Fyne's colour-emoji font, reports version "dev".
go build -o helena ./cmd/helena

# Run without producing a binary
go run ./cmd/helena
```

### Release-grade build

Shipped binaries are built with `-tags no_emoji` (drops Fyne's bundled 4.2 MB
colour-emoji font — cuts ~75 MB of resident memory and ~4 MB of binary; response
text still renders, only colour-emoji glyphs come out blank) and stamp the
version/commit through `-ldflags`. To reproduce a release binary exactly:

=== "Linux / macOS"

    ```sh
    go build -tags no_emoji -trimpath \
      -ldflags="-s -w -X main.version=v0.4.0 -X main.commit=$(git rev-parse HEAD)" \
      -o helena ./cmd/helena
    ```

=== "Windows"

    ```cmd
    :: -H windowsgui suppresses the console window for the GUI app
    go build -tags no_emoji -trimpath ^
      -ldflags="-s -w -H windowsgui -X main.version=v0.4.0 -X main.commit=%GIT_SHA%" ^
      -o helena-windows-amd64.exe .\cmd\helena
    ```

`helena --version` prints the stamped version and commit; a plain `go build`
reports `dev`.

### A note on cross-compilation

Helena does **not** cross-compile: cgo + OpenGL means each OS/arch is built by
its own native C toolchain (no `fyne-cross`, no Docker — see
[AGENTS.md](../AGENTS.md) invariant 8). Build each target on that platform:

- **Windows on ARM (arm64):** build on a native arm64 Windows machine. The
  stock mingw GCC on some arm64 hosts is x86-64 and can't assemble arm64 cgo;
  install [llvm-mingw](https://github.com/mstorsjo/llvm-mingw)'s native
  `aarch64` toolchain and point cgo at it
  (`CC=aarch64-w64-mingw32-gcc`), exactly as
  [`.github/workflows/ci.yml`](../.github/workflows/ci.yml) does.
- **Everything else:** amd64 Linux, amd64 Windows, and arm64 macOS each build
  natively on their own runner.

## Run the tests

```sh
go test ./...            # fast suite
go test ./... -race      # what CI gates on (the -race suite must be clean)
make coverage            # per-package coverage summary
make coverage-gate       # enforce the ≥90% floor CI enforces
```

`go test ./... -race`, `gofmt -l .` (must be empty), `go vet ./...`, and
`go build ./...` should all be clean before you propose a change — that is the
same bar CI holds. See [TESTING.md](../TESTING.md) for the full test story.

## Troubleshooting

| Symptom | Fix |
| ------- | --- |
| `gcc: command not found` / `exec: "gcc"` | Install the C toolchain for your platform (above). cgo needs it. |
| `fatal error: GL/gl.h: No such file` (Linux) | Install `libgl1-mesa-dev` and `xorg-dev` (or the Fedora/Arch equivalents). |
| `gcc_arm64.S: no such instruction` (Windows/ARM) | Your GCC is x86-64; install llvm-mingw's native aarch64 toolchain and set `CC`/`CXX` (see above). |
| Colour emoji render as blank boxes | Expected in release builds (`-tags no_emoji`). Build without the tag for colour emoji. |
| `build Go version lower than targeted` | Your `go` is older than `go.mod`'s `toolchain`; install Go 1.26+. |
| Binary reports version `dev` | That's a plain `go build`; pass the `-ldflags` above to stamp a version. |

## Where to go next

- [Packaging & distribution](PACKAGING.md) — turning the binary into `.deb` /
  `.rpm` / installer / MSIX, and publishing to stores.
- [Getting started](USER_GUIDE.md) — your first request.
- [CONTRIBUTING](../CONTRIBUTING.md) — the contribution workflow.
