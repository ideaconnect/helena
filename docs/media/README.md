# Media assets (screenshots & demo GIF)

The README embeds the images below from this directory. They must be captured
from the running app (headless CI can't produce them), so drop the files here
with these exact names and the README renders them automatically.

| File | What to capture |
| ---- | --------------- |
| `hero.gif` | A short (~10s) loop: open the sample collection, pick a request, Send, show the response. The README hero. |
| `request.png` | The request editor — method + URL, a Query/Headers/Body tab, the sidebar tree. |
| `response.png` | A response: pretty-printed JSON body, headers, status + timing. |
| `environments.png` | The environment manager / Variables editor (with a Secret var masked). |

## Capture tips

- Load the bundled sample first (**?** → *Load sample*, or the empty-state
  panel) so the screenshots show real data.
- Use the default window size (1100×720) and the Dark theme for the hero;
  a Light shot for one still is nice for contrast.
- Keep secrets out of shots (the sample has none).
- For the GIF on Linux: [`peek`](https://github.com/phw/peek) or
  `asciinema`+`agg` for terminals; for the window, `peek` or `byzanz`.
- Keep files reasonably small (optimize PNGs; cap the GIF ~2–4 MB).

Until the files exist the README image links show a broken-image icon; adding
the files (no other change) fixes it. See issue #60.
