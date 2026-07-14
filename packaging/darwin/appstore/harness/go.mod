// Standalone throwaway module — intentionally separate from the Helena module so
// the main repo's `go build ./...`, tests, and CI never descend into this
// darwin+Cgo+AppKit harness. Build it by hand on a Mac via ./build.sh.
module helena-sandbox-harness

go 1.26
