//go:build c

package runtime

var cNowStart int
var cNowLast int

func init() {
	cNowStart = int(SysNanoTime())
	cNowLast = 0
}

func Now() int {
	now := int(SysNanoTime())
	if now <= cNowStart {
		return 0
	}
	delta := now - cNowStart
	if delta < cNowLast {
		delta = cNowLast
	}
	cNowLast = delta
	return delta
}

//rtg:internal SysNanoTime
func SysNanoTime() uintptr
