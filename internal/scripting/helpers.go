package scripting

import (
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash"
	"time"

	"github.com/dop251/goja"
)

// bindHelpers attaches the curated, dependency-free helper surface to the
// helena object:
//
//	helena.uuid()                       -> RFC 4122 v4 UUID string
//	helena.hash.md5 / sha1 / sha256 /
//	          sha512(text)              -> hex digest
//	helena.hash.hmacSha1 /
//	          hmacSha256(key, text)     -> hex HMAC digest
//	helena.date.now()                   -> ISO-8601 (RFC 3339) UTC string
//	helena.date.timestamp()             -> Unix seconds (number)
//	helena.base64.encode / decode(s)    -> standard-base64 string (#92)
//	helena.sleep(ms)                    -> block up to ms (capped, ctx-aware) (#92)
//
// Everything here is pure-compute — no filesystem, network, or process
// access — so the helpers respect the scripting sandbox invariant (no new
// I/O surface; see the package README threat model). sleep only delays the
// calling script; it adds no I/O.
func (rt *Runtime) bindHelpers(ctx context.Context, vm *goja.Runtime, helena *goja.Object) error {
	if err := helena.Set("uuid", func(goja.FunctionCall) goja.Value {
		return vm.ToValue(scriptUUID())
	}); err != nil {
		return err
	}

	digest := func(h func() hash.Hash) func(goja.FunctionCall) goja.Value {
		return func(call goja.FunctionCall) goja.Value {
			sum := h()
			sum.Write([]byte(call.Argument(0).String()))
			return vm.ToValue(hex.EncodeToString(sum.Sum(nil)))
		}
	}
	hmacDigest := func(h func() hash.Hash) func(goja.FunctionCall) goja.Value {
		return func(call goja.FunctionCall) goja.Value {
			mac := hmac.New(h, []byte(call.Argument(0).String()))
			mac.Write([]byte(call.Argument(1).String()))
			return vm.ToValue(hex.EncodeToString(mac.Sum(nil)))
		}
	}
	hashObj := vm.NewObject()
	_ = hashObj.Set("md5", digest(md5.New))
	_ = hashObj.Set("sha1", digest(sha1.New))
	_ = hashObj.Set("sha256", digest(sha256.New))
	_ = hashObj.Set("sha512", digest(sha512.New))
	_ = hashObj.Set("hmacSha1", hmacDigest(sha1.New))
	_ = hashObj.Set("hmacSha256", hmacDigest(sha256.New))
	if err := helena.Set("hash", hashObj); err != nil {
		return err
	}

	dateObj := vm.NewObject()
	_ = dateObj.Set("now", func(goja.FunctionCall) goja.Value {
		return vm.ToValue(time.Now().UTC().Format(time.RFC3339))
	})
	_ = dateObj.Set("timestamp", func(goja.FunctionCall) goja.Value {
		return vm.ToValue(time.Now().Unix())
	})
	if err := helena.Set("date", dateObj); err != nil {
		return err
	}

	b64 := vm.NewObject()
	_ = b64.Set("encode", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(base64.StdEncoding.EncodeToString([]byte(call.Argument(0).String())))
	})
	_ = b64.Set("decode", func(call goja.FunctionCall) goja.Value {
		data, err := base64.StdEncoding.DecodeString(call.Argument(0).String())
		if err != nil {
			panic(vm.NewTypeError("helena.base64.decode: invalid base64 input"))
		}
		return vm.ToValue(string(data))
	})
	if err := helena.Set("base64", b64); err != nil {
		return err
	}

	// helena.sleep(ms) blocks the calling script for up to ms milliseconds,
	// clamped to the per-script time budget (ScriptTimeout) and aborting early
	// if the run's context is cancelled. It only delays this script — no I/O is
	// added — so the sandbox invariant holds. A non-positive / NaN argument is a
	// no-op.
	_ = helena.Set("sleep", func(call goja.FunctionCall) goja.Value {
		ms := call.Argument(0).ToInteger()
		if ms <= 0 {
			return goja.Undefined()
		}
		d := time.Duration(ms) * time.Millisecond
		if d > ScriptTimeout {
			d = ScriptTimeout
		}
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-t.C:
		case <-ctx.Done():
		}
		return goja.Undefined()
	})
	return nil
}

// scriptUUID returns a random RFC 4122 version-4 UUID string. Kept local to
// avoid coupling the sandboxed scripting package to internal/vars just for six
// lines of formatting.
func scriptUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
