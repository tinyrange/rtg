//go:build linux && rv32

package runtime

const (
	linuxClockMonotonicRV32  = 1
	linuxSysClockGettimeRV32 = 403
)

var linuxNowStartSecRV32 int
var linuxNowStartNSecRV32 int
var linuxNowLastRV32 int
var linuxNowReadyRV32 bool
var linuxNowTSRV32 uintptr

func Now() int {
	sec, nsec := linuxMonotonicNowSplitRV32()
	if !linuxNowReadyRV32 {
		linuxNowStartSecRV32 = sec
		linuxNowStartNSecRV32 = nsec
		linuxNowLastRV32 = 0
		linuxNowReadyRV32 = true
		return 0
	}
	deltaSec := sec - linuxNowStartSecRV32
	deltaNSec := nsec - linuxNowStartNSecRV32
	if deltaNSec < 0 {
		deltaNSec += nowNanosPerSecond
		deltaSec = deltaSec - 1
	}
	if deltaSec < 0 {
		return 0
	}
	delta := nowSecNsecToNs(deltaSec, deltaNSec)
	if delta < linuxNowLastRV32 {
		delta = linuxNowLastRV32
	}
	linuxNowLastRV32 = delta
	return delta
}

func linuxMonotonicNowSplitRV32() (int, int) {
	ts := linuxNowTSRV32
	if ts == 0 {
		ts = Alloc(16)
		linuxNowTSRV32 = ts
	}
	_, _, err := Syscall(linuxSysClockGettimeRV32, linuxClockMonotonicRV32, ts, 0, 0, 0, 0)
	if err != 0 {
		return 0, 0
	}
	sec := nowReadU64(ts)
	nsec := nowReadU64(ts + 8)
	return sec, nsec
}
