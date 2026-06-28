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

.PHONY: run build test vet fmt lint tidy clean coverage coverage-html coverage-gate mutation mutation-chain mutation-storage mutation-httpclient mutation-scripting mutation-auth website website-build website-docker

# The project website is a self-contained Jekyll site under website/ (#64).
WEBSITE_DIR := website

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

# website: build and serve the Jekyll project site with live reload so you can
# see it in a browser. Needs Ruby + Bundler (gem install bundler); the gems are
# installed on first run. Serves at http://localhost:4000/helena/ (the baseurl
# in website/_config.yml). Ctrl-C to stop.
website:
	@command -v bundle >/dev/null 2>&1 || { echo "bundler not found — install Ruby + 'gem install bundler', or run 'make website-docker' (no Ruby needed). See $(WEBSITE_DIR)/README.md"; exit 1; }
	@echo "Serving $(WEBSITE_DIR) at http://localhost:4000/helena/ — Ctrl-C to stop"
	cd $(WEBSITE_DIR) && { bundle check >/dev/null 2>&1 || bundle install; } && bundle exec jekyll serve --livereload

# website-build: build the static site into website/_site without serving.
website-build:
	@command -v bundle >/dev/null 2>&1 || { echo "bundler not found — install Ruby + 'gem install bundler', or run 'make website-docker'. See $(WEBSITE_DIR)/README.md"; exit 1; }
	cd $(WEBSITE_DIR) && { bundle check >/dev/null 2>&1 || bundle install; } && bundle exec jekyll build
	@echo "built $(WEBSITE_DIR)/_site"

# website-docker: serve the site with no local Ruby — uses the official ruby
# image to install the gems and run jekyll serve, reachable at
# http://localhost:4000/helena/. Ctrl-C to stop. First run pulls the image.
website-docker:
	@command -v docker >/dev/null 2>&1 || { echo "docker not found — install Docker, or use 'make website' with Ruby"; exit 1; }
	@echo "Serving $(WEBSITE_DIR) via Docker at http://localhost:4000/helena/ — Ctrl-C to stop"
	docker run --rm -it -p 4000:4000 -v "$(CURDIR)/$(WEBSITE_DIR):/site" -w /site ruby:3.3 \
		sh -c "bundle install && bundle exec jekyll serve -H 0.0.0.0 --livereload --force_polling"

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
