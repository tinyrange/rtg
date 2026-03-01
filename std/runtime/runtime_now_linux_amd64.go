//go:build linux && amd64

package runtime

const (
	linuxClockMonotonicAMD64  = 1
	linuxSysClockGettimeAMD64 = 228
)

var linuxNowStartAMD64 int
var linuxNowLastAMD64 int
var linuxNowReadyAMD64 bool
var linuxNowTSAMD64 uintptr

func Now() int {
	now := linuxMonotonicNowAMD64()
	if !linuxNowReadyAMD64 {
		linuxNowStartAMD64 = now
		linuxNowLastAMD64 = 0
		linuxNowReadyAMD64 = true
		return 0
	}
	if now <= linuxNowStartAMD64 {
		return 0
	}
	delta := now - linuxNowStartAMD64
	if delta < linuxNowLastAMD64 {
		delta = linuxNowLastAMD64
	}
	linuxNowLastAMD64 = delta
	return delta
}

func linuxMonotonicNowAMD64() int {
	ts := linuxNowTSAMD64
	if ts == 0 {
		ts = Alloc(16)
		linuxNowTSAMD64 = ts
	}
	_, _, err := Syscall(linuxSysClockGettimeAMD64, linuxClockMonotonicAMD64, ts, 0, 0, 0, 0)
	if err != 0 {
		return 0
	}
	sec := nowReadU64(ts)
	nsec := nowReadU64(ts + 8)
	return nowSecNsecToNs(sec, nsec)
}
