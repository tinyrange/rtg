//go:build wasi && wasm32

package runtime

var wasiNowStart uintptr
var wasiNowLast int
var wasiNowReady bool

func Now() int {
	now := SysNanoTime()
	if !wasiNowReady {
		wasiNowStart = now
		wasiNowLast = 0
		wasiNowReady = true
		return 0
	}
	delta := nowDeltaWord(now, wasiNowStart)
	if delta <= 0 {
		return 0
	}
	if delta < wasiNowLast {
		delta = wasiNowLast
	}
	wasiNowLast = delta
	return delta
}

//rtg:internal SysNanoTime
func SysNanoTime() uintptr
