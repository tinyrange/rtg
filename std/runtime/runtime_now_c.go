//go:build c

package runtime

var cNowStart uintptr
var cNowLast int
var cNowReady bool

func Now() int {
	now := SysNanoTime()
	if !cNowReady {
		cNowStart = now
		cNowLast = 0
		cNowReady = true
		return 0
	}
	delta := nowDeltaWord(now, cNowStart)
	if delta < cNowLast {
		delta = cNowLast
	}
	cNowLast = delta
	return delta
}

//rtg:internal SysNanoTime
func SysNanoTime() uintptr
