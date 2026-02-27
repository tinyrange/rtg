//go:build wasi && wasm32

package runtime

var wasiNowStart int
var wasiNowLast int

func init() {
	wasiNowStart = int(SysNanoTime())
	wasiNowLast = 0
}

func Now() int {
	now := int(SysNanoTime())
	if now <= wasiNowStart {
		return 0
	}
	delta := now - wasiNowStart
	if delta < wasiNowLast {
		delta = wasiNowLast
	}
	wasiNowLast = delta
	return delta
}

//rtg:internal SysNanoTime
func SysNanoTime() uintptr
