//go:build debugger

package debugonly

import (
	"runtime"
)

// BreakHere (Important) runtime.Breakpoint() 이걸 직접 쓰는 것은 금지. 프로덕션 코드에 들어갈 수 있어서 운영상 사고가 발생할 수 있음.
func BreakHere() {
	runtime.Breakpoint()
}

func Enabled() bool {
	return true
}
