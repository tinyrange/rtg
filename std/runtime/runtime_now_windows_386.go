//go:build windows && 386

package runtime

var winNowStartTick32 uint32
var winNowLast32 int

func init() {
	winNowStartTick32 = uint32(winGetTickCount())
	winNowLast32 = 0
}

func Now() int {
	cur := uint32(winGetTickCount())
	deltaMS := int(cur - winNowStartTick32)
	ns := nowSaturatingMul(deltaMS, 1000000)
	if ns < winNowLast32 {
		ns = winNowLast32
	}
	winNowLast32 = ns
	return ns
}

//rtg:linkstatic kernel32.dll,GetTickCount,rawptr
func winGetTickCount() (uintptr, uintptr, int32)
