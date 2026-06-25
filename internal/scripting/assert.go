package scripting

import "github.com/dop251/goja"

// bindTest installs the test()/expect() assertion API (#87). A small Go
// recorder (__helenaRecordTest) appends each test outcome to res.Tests; the
// matcher library itself is defined in JS (testPrelude) so the Chai-like
// surface stays readable and runs entirely in the sandbox. Bound in both
// phases, though assertions are most useful post-response.
func bindTest(vm *goja.Runtime, res *Result) error {
	if err := vm.Set("__helenaRecordTest", func(call goja.FunctionCall) goja.Value {
		res.Tests = append(res.Tests, TestResult{
			Name:   call.Argument(0).String(),
			Passed: call.Argument(1).ToBoolean(),
			Error:  call.Argument(2).String(),
		})
		return goja.Undefined()
	}); err != nil {
		return err
	}
	_, err := vm.RunString(testPrelude)
	return err
}

// testPrelude defines the global test() runner and expect() matcher chain in
// JavaScript. test(name, fn) runs fn and records pass/fail (a thrown matcher
// error → fail with its message). expect(actual) exposes a focused Chai-like
// matcher set, each negatable via `.not`.
const testPrelude = `
function __helenaExpect(actual, negate) { this.actual = actual; this.negate = !!negate; }
__helenaExpect.prototype._assert = function (pass, msg) {
	if (this.negate) pass = !pass;
	if (!pass) throw new Error(msg);
};
function __helenaFmt(v) { try { return JSON.stringify(v); } catch (e) { return String(v); } }
__helenaExpect.prototype.toBe = function (exp) {
	this._assert(this.actual === exp, "expected " + __helenaFmt(this.actual) + (this.negate ? " not" : "") + " to be " + __helenaFmt(exp));
};
__helenaExpect.prototype.toEqual = function (exp) {
	this._assert(JSON.stringify(this.actual) === JSON.stringify(exp), "expected " + __helenaFmt(this.actual) + (this.negate ? " not" : "") + " to equal " + __helenaFmt(exp));
};
__helenaExpect.prototype.toBeTruthy = function () { this._assert(!!this.actual, "expected " + __helenaFmt(this.actual) + " to be truthy"); };
__helenaExpect.prototype.toBeFalsy = function () { this._assert(!this.actual, "expected " + __helenaFmt(this.actual) + " to be falsy"); };
__helenaExpect.prototype.toBeNull = function () { this._assert(this.actual === null, "expected " + __helenaFmt(this.actual) + " to be null"); };
__helenaExpect.prototype.toBeDefined = function () { this._assert(typeof this.actual !== "undefined", "expected value to be defined"); };
__helenaExpect.prototype.toBeUndefined = function () { this._assert(typeof this.actual === "undefined", "expected value to be undefined"); };
__helenaExpect.prototype.toContain = function (sub) {
	this._assert(this.actual != null && typeof this.actual.indexOf === "function" && this.actual.indexOf(sub) >= 0, "expected " + __helenaFmt(this.actual) + " to contain " + __helenaFmt(sub));
};
__helenaExpect.prototype.toHaveLength = function (n) {
	this._assert(this.actual != null && this.actual.length === n, "expected length " + (this.actual == null ? "<nil>" : this.actual.length) + " to be " + n);
};
__helenaExpect.prototype.toBeGreaterThan = function (n) { this._assert(this.actual > n, "expected " + __helenaFmt(this.actual) + " to be greater than " + __helenaFmt(n)); };
__helenaExpect.prototype.toBeLessThan = function (n) { this._assert(this.actual < n, "expected " + __helenaFmt(this.actual) + " to be less than " + __helenaFmt(n)); };
Object.defineProperty(__helenaExpect.prototype, "not", { get: function () { return new __helenaExpect(this.actual, !this.negate); } });
function expect(actual) { return new __helenaExpect(actual, false); }
function test(name, fn) {
	try { fn(); __helenaRecordTest(String(name), true, ""); }
	catch (e) { __helenaRecordTest(String(name), false, (e && e.message) ? e.message : String(e)); }
}
`
