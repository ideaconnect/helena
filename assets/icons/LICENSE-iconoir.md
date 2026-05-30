# Iconoir icons

The SVG files in this directory are sourced from
[Iconoir](https://iconoir.com) (the
[iconoir-icons/iconoir](https://github.com/iconoir-icons/iconoir)
repository).

Iconoir is released under the MIT License (compatible with Helena's
BSD-4 License). Original copyright and license text below.

```
MIT License

Copyright (c) 2021-present Luca Burgio

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

## Files in this directory

| File | Source path in iconoir/main |
| --- | --- |
| `copy.svg` | `icons/regular/copy.svg` |
| `input-field.svg` | `icons/regular/input-field.svg` |
| `nav-arrow-right.svg` | `icons/regular/nav-arrow-right.svg` |
| `send-diagonal-solid.svg` | `icons/solid/send-diagonal.svg` (renamed to keep the `-solid` suffix convention Helena uses internally) |
| `xmark-circle-solid.svg` | `icons/solid/xmark-circle.svg` (same renaming) |

## Adding more

To pull additional icons, fetch them from the upstream repo and drop
them under this directory:

```sh
cd assets/icons
curl -fsSL -o NAME.svg https://raw.githubusercontent.com/iconoir-icons/iconoir/main/icons/regular/NAME.svg
# or .../solid/NAME.svg for the solid variant — keep the `-solid` suffix
# in the local filename to match Helena's convention.
```

Then reference via `assets.Icon("NAME")` in widget code.
