//go:build linux && 386

package runtime

const (
	linuxClockMonotonic386  = 1
	linuxSysClockGettime386 = 265
)

var linuxNowStart386 int
var linuxNowLast386 int

func init() {
	linuxNowStart386 = linuxMonotonicNow386()
	linuxNowLast386 = 0
}

func Now() int {
	now := linuxMonotonicNow386()
	if now <= linuxNowStart386 {
		return 0
	}
	delta := now - linuxNowStart386
	if delta < linuxNowLast386 {
		delta = linuxNowLast386
	}
	linuxNowLast386 = delta
	return delta
}

func linuxMonotonicNow386() int {
	ts := Alloc(8)
	Memzero(ts, 8)
	_, _, err := Syscall(linuxSysClockGettime386, linuxClockMonotonic386, ts, 0, 0, 0, 0)
	if err != 0 {
		return 0
	}
	sec := nowReadU32(ts)
	nsec := nowReadU32(ts + 4)
	return nowSecNsecToNs(sec, nsec)
}
