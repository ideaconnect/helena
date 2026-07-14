//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework AppKit
#include <stdlib.h>

const char* harness_pick_and_bookmark(char** pathOut, char** errOut);
const char* harness_resolve_start(const char* b64, int* staleOut, char** errOut);
void        harness_stop(void);
*/
import "C"

import (
	"errors"
	"unsafe"
)

// pickAndBookmark shows the native NSOpenPanel (folder mode), and on selection
// mints an app-scope security-scoped bookmark from the fresh powerbox URL —
// which can only be done while that URL still carries the extension. Returns
// ("", "", nil) if the user cancelled.
func pickAndBookmark() (path, bookmarkB64 string, err error) {
	var cpath, cerr *C.char
	cbm := C.harness_pick_and_bookmark(&cpath, &cerr)
	if cerr != nil {
		defer C.free(unsafe.Pointer(cerr))
		return "", "", errors.New(C.GoString(cerr))
	}
	if cbm == nil {
		return "", "", nil // cancelled
	}
	defer C.free(unsafe.Pointer(cbm))
	if cpath != nil {
		defer C.free(unsafe.Pointer(cpath))
		path = C.GoString(cpath)
	}
	return path, C.GoString(cbm), nil
}

// resolveAndStart resolves a stored base64 bookmark back to a URL and begins
// security-scoped access (held until stopAccess). stale is true when macOS
// flags the bookmark as needing a re-mint.
func resolveAndStart(bookmarkB64 string) (path string, stale bool, err error) {
	cb := C.CString(bookmarkB64)
	defer C.free(unsafe.Pointer(cb))
	var st C.int
	var cerr *C.char
	cp := C.harness_resolve_start(cb, &st, &cerr)
	if cp == nil {
		if cerr != nil {
			defer C.free(unsafe.Pointer(cerr))
			return "", false, errors.New(C.GoString(cerr))
		}
		return "", false, errors.New("resolve failed (nil URL, no error)")
	}
	defer C.free(unsafe.Pointer(cp))
	return C.GoString(cp), st == 1, nil
}

// stopAccess balances resolveAndStart's startAccessingSecurityScopedResource.
func stopAccess() { C.harness_stop() }
