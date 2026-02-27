//go:build linux && arm64

package runtime

const (
	linuxClockMonotonicARM64  = 1
	linuxSysClockGettimeARM64 = 113
)

var linuxNowStartARM64 int
var linuxNowLastARM64 int

func init() {
	linuxNowStartARM64 = linuxMonotonicNowARM64()
	linuxNowLastARM64 = 0
}

func Now() int {
	now := linuxMonotonicNowARM64()
	if now <= linuxNowStartARM64 {
		return 0
	}
	delta := now - linuxNowStartARM64
	if delta < linuxNowLastARM64 {
		delta = linuxNowLastARM64
	}
	linuxNowLastARM64 = delta
	return delta
}

func linuxMonotonicNowARM64() int {
	ts := Alloc(16)
	Memzero(ts, 16)
	_, _, err := Syscall(linuxSysClockGettimeARM64, linuxClockMonotonicARM64, ts, 0, 0, 0, 0)
	if err != 0 {
		return 0
	}
	sec := nowReadU64(ts)
	nsec := nowReadU64(ts + 8)
	return nowSecNsecToNs(sec, nsec)
}
