APP := helena
PKG := ./cmd/helena

# Phase 8 coverage gate: every internal package outside internal/ui
# must stay at or above this floor. UI tests are deferred to Phase 11.
# Non-core packages are excluded: internal/ui (deferred), cmd (entrypoints +
# tooling), features/integration (BDD/integration harnesses), and examples
# (the bundled sample collection's loader — demo support, not core logic).
COVERAGE_FLOOR    := 90
COVERAGE_EXCLUDES := internal/ui,cmd,features,integration,examples
COVERAGE_PROFILE  := coverage.out
COVERAGE_HTML     := coverage.html

.PHONY: run build test vet fmt lint tidy clean coverage coverage-html coverage-gate mutation mutation-chain mutation-storage mutation-httpclient mutation-scripting mutation-auth website website-build screenshots screenshots-fancy webp

# The project website is a self-contained Jekyll site under website/ (#64). It
# builds with dockerized Ruby — no local Ruby toolchain needed. The container
# runs as the host user (so generated files stay user-owned and `make clean`
# can remove them) with the gem cache in website/.bundle (gitignored, persisted
# between runs so only the first build installs gems).
WEBSITE_DIR  := website
WEBSITE_PORT := 4000
DOCKER_RUBY  := docker run --rm -u $$(id -u):$$(id -g) -e HOME=/tmp \
	-e BUNDLE_PATH=/site/.bundle -v "$(CURDIR)/$(WEBSITE_DIR):/site" -w /site

# Phase 8.6 mutation testing: run gremlins against the five load-bearing
# packages. Each target is invokable individually for iteration; the
# parent `mutation` target runs all five sequentially.
#
# gremlins runs the test suite once per generated mutation, so the
# wall-clock is "test-suite-runtime × mutation-count". Not gated per
# PR — 8.7 wires this as a nightly CI job.
GREMLINS := $(shell go env GOPATH)/bin/gremlins

$(GREMLINS):
	go install github.com/go-gremlins/gremlins/cmd/gremlins@latest

run:
	go run $(PKG)

build:
	go build -o bin/$(APP) $(PKG)

test:
	go test ./...

# coverage: write the per-package profile and print the per-package
# summary. Excluded packages are still printed so drift is visible.
coverage:
	go test ./... -coverprofile=$(COVERAGE_PROFILE) -covermode=atomic
	@go run ./cmd/covergate -profile $(COVERAGE_PROFILE) -exclude $(COVERAGE_EXCLUDES)

# coverage-html: render the profile to an HTML report and print where
# it landed. Requires `coverage` (or any other run that produced the
# profile) to have completed first — checked at runtime so make's
# pattern rules don't try to synthesise the profile.
coverage-html:
	@test -f $(COVERAGE_PROFILE) || { echo "$(COVERAGE_PROFILE) not found - run 'make coverage' first"; exit 1; }
	go tool cover -html=$(COVERAGE_PROFILE) -o $(COVERAGE_HTML)
	@echo "wrote $(COVERAGE_HTML)"

# coverage-gate: re-run coverage and enforce the per-package floor.
# Used by CI; fails the build when any gated package is below floor.
coverage-gate:
	go test ./... -coverprofile=$(COVERAGE_PROFILE) -covermode=atomic
	go run ./cmd/covergate -profile $(COVERAGE_PROFILE) -exclude $(COVERAGE_EXCLUDES) -floor $(COVERAGE_FLOOR)

# gremlins uses go test under the hood. A stale test cache from a
# previous run can short-circuit individual mutation runs and skew
# the baseline timing, so each target invalidates the cache first.
# --timeout-coefficient=6 gives mutated tests 6× the baseline budget
# before being recorded as timed-out (rather than killed); some
# chain cap-checks legitimately need the extra slack.
GREMLINS_FLAGS := --timeout-coefficient 6

mutation-chain: $(GREMLINS)
	go clean -testcache
	$(GREMLINS) unleash $(GREMLINS_FLAGS) ./internal/chain
mutation-storage: $(GREMLINS)
	go clean -testcache
	$(GREMLINS) unleash $(GREMLINS_FLAGS) ./internal/storage
mutation-httpclient: $(GREMLINS)
	go clean -testcache
	$(GREMLINS) unleash $(GREMLINS_FLAGS) ./internal/httpclient
mutation-scripting: $(GREMLINS)
	go clean -testcache
	$(GREMLINS) unleash $(GREMLINS_FLAGS) ./internal/scripting
mutation-auth: $(GREMLINS)
	go clean -testcache
	$(GREMLINS) unleash $(GREMLINS_FLAGS) ./internal/auth
mutation: mutation-chain mutation-storage mutation-httpclient mutation-scripting mutation-auth

# website: build and serve the Jekyll site with live reload, via dockerized
# Ruby — no local Ruby needed (only Docker). Reachable at http://localhost:4000/
# (baseurl is "" — the site deploys to the helena.idct.tech root). The first run
# pulls ruby:3.3 and installs the gems into website/.bundle; later runs are fast.
# Ctrl-C to stop.
website:
	@command -v docker >/dev/null 2>&1 || { echo "docker not found — install Docker (the website builds with dockerized Ruby). See $(WEBSITE_DIR)/README.md"; exit 1; }
	@echo "Serving $(WEBSITE_DIR) at http://localhost:$(WEBSITE_PORT)/ — Ctrl-C to stop (first run pulls ruby:3.3)"
	$(DOCKER_RUBY) -it -p $(WEBSITE_PORT):4000 ruby:3.3 \
		sh -c "bundle install && bundle exec jekyll serve -H 0.0.0.0 --livereload --force_polling"

# website-build: build the static site into website/_site (dockerized Ruby), no
# server. Useful for CI-style checks or inspecting the generated HTML. Builds at
# baseurl "" to match production (the custom domain helena.idct.tech root).
website-build:
	@command -v docker >/dev/null 2>&1 || { echo "docker not found — install Docker. See $(WEBSITE_DIR)/README.md"; exit 1; }
	$(DOCKER_RUBY) ruby:3.3 sh -c "bundle install && bundle exec jekyll build --baseurl ''"
	@echo "built $(WEBSITE_DIR)/_site"

# screenshots: (re)generate the website screenshots by rendering the real UI
# against a fake in-memory API with Fyne's software canvas — no display, no
# running app, no C toolchain. Writes website/assets/img/*.png. The generator is
# an env-gated test (skipped by the normal suite); see internal/ui/screenshots_test.go.
screenshots:
	HELENA_SHOTS="$(CURDIR)/$(WEBSITE_DIR)/assets/img" go test ./internal/ui -run TestGenerateScreenshots -count=1 -v
	@echo "wrote $(WEBSITE_DIR)/assets/img/*.png"

# screenshots-fancy: dress the hero captures as floating app windows (title bar,
# rounded corners, drop shadow) for the website hero box. Needs ImageMagick +
# bash; run `make screenshots` first to refresh the source captures. Ends by
# (re)generating the WebP copies so they never drift from the PNGs.
IMG := $(WEBSITE_DIR)/assets/img
screenshots-fancy:
	$(WEBSITE_DIR)/tools/frame-shot.sh $(IMG)/app-hero.png $(IMG)/app-hero-fancy.png
	$(WEBSITE_DIR)/tools/frame-shot.sh $(IMG)/shot-auth.png $(IMG)/shot-auth-fancy.png
	$(WEBSITE_DIR)/tools/frame-shot.sh $(IMG)/shot-request.png $(IMG)/shot-request-fancy.png
	$(WEBSITE_DIR)/tools/frame-shot.sh $(IMG)/shot-chain.png $(IMG)/shot-chain-fancy.png
	$(MAKE) webp

# webp: emit WebP copies of the screenshots the site displays via <picture>
# (webp source, PNG fallback stays). ~90% smaller hero art, ~70% smaller inline
# shots. Wired into screenshots-fancy so a screenshot refresh keeps them in sync;
# also runnable standalone. Needs ImageMagick's `convert` built with libwebp.
WEBP_SHOTS := app-hero shot-auth shot-request shot-chain \
	app-hero-fancy shot-auth-fancy shot-request-fancy shot-chain-fancy
webp:
	@command -v convert >/dev/null 2>&1 || { echo "convert (ImageMagick) not found — install ImageMagick with libwebp"; exit 1; }
	@for f in $(WEBP_SHOTS); do \
		convert "$(IMG)/$$f.png" -quality 88 -define webp:method=6 -define webp:alpha-quality=100 "$(IMG)/$$f.webp" && \
		echo "wrote $(IMG)/$$f.webp"; \
	done

vet:
	go vet ./...

fmt:
	gofmt -w .

lint:
	golangci-lint run

tidy:
	go mod tidy

clean:
	rm -rf bin dist $(COVERAGE_PROFILE) $(COVERAGE_HTML) $(WEBSITE_DIR)/_site $(WEBSITE_DIR)/.jekyll-cache
