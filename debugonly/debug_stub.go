//go:build !debugger

package debugonly

// BreakHere is a no-op stub used as a breakpoint target in non-debugger builds.
func BreakHere() {}

// Enabled reports whether the debugger build tag is active. Always false in production builds.
func Enabled() bool { return false }
