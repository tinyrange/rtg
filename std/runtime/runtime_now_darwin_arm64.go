//go:build darwin && arm64

package runtime

const darwinClockMonotonic = 6

var darwinNowStart int
var darwinNowLast int
var darwinNowReady bool

func Now() int {
	now := darwinMonotonicNow()
	if !darwinNowReady {
		darwinNowStart = now
		darwinNowLast = 0
		darwinNowReady = true
		return 0
	}
	if now <= darwinNowStart {
		return 0
	}
	delta := now - darwinNowStart
	if delta < darwinNowLast {
		delta = darwinNowLast
	}
	darwinNowLast = delta
	return delta
}

func darwinMonotonicNow() int {
	ts := Alloc(16)
	Memzero(ts, 16)
	rv, _, err := darwinClockGettime(darwinClockMonotonic, ts)
	if err != 0 || rv != 0 {
		return 0
	}
	sec := nowReadU64(ts)
	nsec := nowReadU64(ts + 8)
	return nowSecNsecToNs(sec, nsec)
}

//rtg:linkstatic libSystem.dylib,_clock_gettime
func darwinClockGettime(clockID, ts uintptr) (uintptr, uintptr, int32)
