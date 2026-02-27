//go:build linux && 386

package runtime

const (
	linuxClockMonotonic386  = 1
	linuxSysClockGettime386 = 265
)

var linuxNowStartSec386 int
var linuxNowStartNSec386 int
var linuxNowLast386 int
var linuxNowReady386 bool

func Now() int {
	sec, nsec := linuxMonotonicNowSplit386()
	if !linuxNowReady386 {
		linuxNowStartSec386 = sec
		linuxNowStartNSec386 = nsec
		linuxNowLast386 = 0
		linuxNowReady386 = true
		return 0
	}
	deltaSec := sec - linuxNowStartSec386
	deltaNSec := nsec - linuxNowStartNSec386
	if deltaNSec < 0 {
		deltaNSec = deltaNSec + nowNanosPerSecond
		deltaSec = deltaSec - 1
	}
	if deltaSec < 0 {
		return 0
	}
	delta := nowSecNsecToNs(deltaSec, deltaNSec)
	if delta < linuxNowLast386 {
		delta = linuxNowLast386
	}
	linuxNowLast386 = delta
	return delta
}

func linuxMonotonicNowSplit386() (int, int) {
	ts := Alloc(8)
	Memzero(ts, 8)
	_, _, err := Syscall(linuxSysClockGettime386, linuxClockMonotonic386, ts, 0, 0, 0, 0)
	if err != 0 {
		return 0, 0
	}
	sec := nowReadU32(ts)
	nsec := nowReadU32(ts + 4)
	return sec, nsec
}
