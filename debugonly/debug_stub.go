//go:build !debugger

package debugonly

func BreakHere()    {}
func Enabled() bool { return false }
