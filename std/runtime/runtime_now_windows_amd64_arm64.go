//go:build windows && (amd64 || arm64)

package runtime

var winNowStartCounter int
var winNowLast int
var winNowFreq int = 1
var winNowReady bool
var winNowScratch uintptr

func Now() int {
	if !winNowReady {
		winNowFreq = winQueryPerformanceFrequencyValue()
		if winNowFreq <= 0 {
			winNowFreq = 1
		}
		winNowStartCounter = winQueryPerformanceCounterValue()
		winNowLast = 0
		winNowReady = true
		return 0
	}
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
	buf := winNowScratchPtr()
	ok, _, _ := winQueryPerformanceCounter(buf)
	if ok == 0 {
		return 0
	}
	return nowReadU64(buf)
}

func winQueryPerformanceFrequencyValue() int {
	buf := winNowScratchPtr()
	ok, _, _ := winQueryPerformanceFrequency(buf)
	if ok == 0 {
		return 0
	}
	return nowReadU64(buf)
}

func winNowScratchPtr() uintptr {
	buf := winNowScratch
	if buf == 0 {
		buf = Alloc(8)
		winNowScratch = buf
	}
	return buf
}

//rtg:linkstatic kernel32.dll,QueryPerformanceCounter,rawptr
func winQueryPerformanceCounter(lpPerformanceCount uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,QueryPerformanceFrequency,rawptr
func winQueryPerformanceFrequency(lpFrequency uintptr) (uintptr, uintptr, int32)
