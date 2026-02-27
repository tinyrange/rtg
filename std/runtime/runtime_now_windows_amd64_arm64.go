//go:build windows && (amd64 || arm64)

package runtime

var winNowStartCounter int
var winNowLast int
var winNowFreq int = 1

func init() {
	winNowFreq = winQueryPerformanceFrequencyValue()
	if winNowFreq <= 0 {
		winNowFreq = 1
	}
	winNowStartCounter = winQueryPerformanceCounterValue()
	winNowLast = 0
}

func Now() int {
	counter := winQueryPerformanceCounterValue()
	if counter <= winNowStartCounter {
		return 0
	}
	delta := counter - winNowStartCounter
	ns := nowMulDiv(delta, nowNanosPerSecond, winNowFreq)
	if ns < winNowLast {
		ns = winNowLast
	}
	winNowLast = ns
	return ns
}

func winQueryPerformanceCounterValue() int {
	buf := Alloc(8)
	Memzero(buf, 8)
	ok, _, _ := winQueryPerformanceCounter(buf)
	if ok == 0 {
		return 0
	}
	return nowReadU64(buf)
}

func winQueryPerformanceFrequencyValue() int {
	buf := Alloc(8)
	Memzero(buf, 8)
	ok, _, _ := winQueryPerformanceFrequency(buf)
	if ok == 0 {
		return 0
	}
	return nowReadU64(buf)
}

//rtg:linkstatic kernel32.dll,QueryPerformanceCounter,rawptr
func winQueryPerformanceCounter(lpPerformanceCount uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,QueryPerformanceFrequency,rawptr
func winQueryPerformanceFrequency(lpFrequency uintptr) (uintptr, uintptr, int32)
