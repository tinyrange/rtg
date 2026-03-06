//go:build linux && rv64

package runtime

const (
	linuxClockMonotonicRV64  = 1
	linuxSysClockGettimeRV64 = 113
)

var linuxNowStartRV64 int
var linuxNowLastRV64 int
var linuxNowReadyRV64 bool
var linuxNowTSRV64 uintptr

func Now() int {
	now := linuxMonotonicNowRV64()
	if !linuxNowReadyRV64 {
		linuxNowStartRV64 = now
		linuxNowLastRV64 = 0
		linuxNowReadyRV64 = true
		return 0
	}
	if now <= linuxNowStartRV64 {
		return 0
	}
	delta := now - linuxNowStartRV64
	if delta < linuxNowLastRV64 {
		delta = linuxNowLastRV64
	}
	linuxNowLastRV64 = delta
	return delta
}

func linuxMonotonicNowRV64() int {
	ts := linuxNowTSRV64
	if ts == 0 {
		ts = Alloc(16)
		linuxNowTSRV64 = ts
	}
	_, _, err := Syscall(linuxSysClockGettimeRV64, linuxClockMonotonicRV64, ts, 0, 0, 0, 0)
	if err != 0 {
		return 0
	}
	sec := nowReadU64(ts)
	nsec := nowReadU64(ts + 8)
	return nowSecNsecToNs(sec, nsec)
}
