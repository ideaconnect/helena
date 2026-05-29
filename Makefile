APP := helena
PKG := ./cmd/helena

# Phase 8 coverage gate: every internal package outside internal/ui
# must stay at or above this floor. UI tests are deferred to Phase 11.
COVERAGE_FLOOR    := 90
COVERAGE_EXCLUDES := internal/ui,cmd
COVERAGE_PROFILE  := coverage.out
COVERAGE_HTML     := coverage.html

.PHONY: run build test vet fmt lint tidy clean coverage coverage-html coverage-gate

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

vet:
	go vet ./...

fmt:
	gofmt -w .

lint:
	golangci-lint run

tidy:
	go mod tidy

clean:
	rm -rf bin dist $(COVERAGE_PROFILE) $(COVERAGE_HTML)
