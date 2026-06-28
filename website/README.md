# Helena project website

A self-contained **Jekyll** site for the Helena project — landing page,
features, a graphical roadmap, examples, and an about page. It lives in this
subfolder so it can be deployed to GitHub Pages independently of the code.

It uses **custom layouts + CSS only** — no gem theme, no plugins — so it builds
on GitHub Pages' stock Jekyll without extra configuration.

```
website/
├── _config.yml          # site config + nav
├── _layouts/            # default.html (chrome) + page.html (content pages)
├── index.html           # landing / hero
├── features.md          # feature catalogue
├── roadmap.md           # graphical timeline (shipped / planned)
├── examples.md          # workflows + screenshots
├── about.md             # about the author
└── assets/
    ├── css/style.css    # the whole design system (Helena green accent)
    └── img/*.svg         # logo + UI mockups used as example "screenshots"
```

## Run locally

The site builds with **dockerized Ruby** — you only need **Docker**, no local
Ruby toolchain. From the repo root:

```bash
make website          # serve with live reload → http://localhost:4000/helena/  (Ctrl-C to stop)
make website-build    # build the static site into website/_site
```

The first run pulls `ruby:3.3` and installs the gems into `website/.bundle`
(gitignored); later runs are fast. The container runs as your user, so the
generated files stay yours and `make clean` removes them. On Windows use
`make.bat website`.

> **Prefer a native Ruby?** With Ruby + Bundler installed you can also run
> Jekyll directly — `cd website && bundle install && bundle exec jekyll serve
> --livereload`.

(`baseurl: "/helena"` in `_config.yml` matches GitHub **project** pages served
at `https://<user>.github.io/helena/`. Set it to `""` for a user/org or
custom-domain site.)

## Deploy to GitHub Pages

GitHub Pages can't build from a subfolder directly, so build it in CI and
publish the result. Add this workflow (and set **Settings → Pages → Source:
GitHub Actions**):

```yaml
name: Website
on:
  push:
    branches: [main]
    paths: [website/**, .github/workflows/website.yml]
  workflow_dispatch:
permissions: { contents: read, pages: write, id-token: write }
concurrency: { group: pages, cancel-in-progress: false }
jobs:
  build:
    runs-on: ubuntu-latest
    defaults: { run: { working-directory: website } }
    steps:
      - uses: actions/checkout@v4
      - uses: ruby/setup-ruby@v1
        with: { ruby-version: "3.3", bundler-cache: true, working-directory: website }
      - run: bundle exec jekyll build --baseurl "/helena"
      - uses: actions/upload-pages-artifact@v3
        with: { path: website/_site }
  deploy:
    needs: build
    runs-on: ubuntu-latest
    environment: { name: github-pages, url: "${{ steps.d.outputs.page_url }}" }
    steps:
      - id: d
        uses: actions/deploy-pages@v4
```

> **Heads-up:** a repo has a single GitHub Pages site. The repo also ships a
> MkDocs reference-docs deploy (`.github/workflows/docs.yml`). Only **one** can
> own Pages — pick this Jekyll site *or* the MkDocs docs as the Pages source and
> disable the other workflow (or publish one to a custom domain / separate repo).

## Screenshots

The example visuals in `assets/img/*.svg` are **interface mockups** so the site
renders without a running app. Replace them with real PNG/GIF captures (the repo
keeps a capture guide in [`docs/media/`](../docs/media/)) — keep the same file
names and the pages pick them up.
