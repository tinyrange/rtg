//go:build windows && 386

package runtime

var winNowStartTick32 uint32
var winNowLast32 int
var winNowReady32 bool

func Now() int {
	cur := winGetTickCountValue()
	if !winNowReady32 {
		winNowStartTick32 = cur
		winNowLast32 = 0
		winNowReady32 = true
		return 0
	}
	// 32-bit GetTickCount wraps roughly every 49.7 days. The modular delta
	// below keeps monotonic behavior across wrap events.
	deltaMS := nowDeltaU32(cur, winNowStartTick32)
	ns := nowSaturatingMul(deltaMS, 1000000)
	if ns < winNowLast32 {
		ns = winNowLast32
	}
	winNowLast32 = ns
	return ns
}

func winGetTickCountValue() uint32 {
	v, _, _ := winGetTickCount()
	return uint32(v)
}

func nowDeltaU32(now uint32, start uint32) int {
	delta := now - start
	max := uint32(nowMaxInt())
	if delta > max {
		return nowMaxInt()
	}
	return int(delta)
}

//rtg:linkstatic kernel32.dll,GetTickCount,rawptr
func winGetTickCount() (uintptr, uintptr, int32)
