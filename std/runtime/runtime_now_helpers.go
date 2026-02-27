package runtime

const nowNanosPerSecond int = 1000000000

func nowMaxInt() int {
	if PtrSize <= 4 {
		return 2147483647
	}
	hi := 2147483647
	lo := (hi << 1) + 1
	return (hi << 32) + lo
}

func nowSaturatingAdd(a int, b int) int {
	max := nowMaxInt()
	if a >= max {
		return max
	}
	if b <= 0 {
		return a
	}
	if b > max-a {
		return max
	}
	return a + b
}

func nowSaturatingMul(a int, b int) int {
	max := nowMaxInt()
	if a <= 0 || b <= 0 {
		return 0
	}
	if a > max/b {
		return max
	}
	return a * b
}

func nowMulDiv(value int, mul int, div int) int {
	if value <= 0 || mul <= 0 || div <= 0 {
		return 0
	}
	q := value / div
	r := value % div
	left := nowSaturatingMul(q, mul)
	right := nowSaturatingMul(r, mul) / div
	return nowSaturatingAdd(left, right)
}

func nowSecNsecToNs(sec int, nsec int) int {
	if sec < 0 {
		return 0
	}
	nsFromSec := nowSaturatingMul(sec, nowNanosPerSecond)
	if nsec < 0 {
		return nsFromSec
	}
	return nowSaturatingAdd(nsFromSec, nsec)
}

func nowReadU32(ptr uintptr) int {
	b := Makeslice(ptr, 4, 4)
	return int(b[0]) | int(b[1])<<8 | int(b[2])<<16 | int(b[3])<<24
}

func nowReadU64(ptr uintptr) int {
	lo := nowReadU32(ptr)
	hi := nowReadU32(ptr + 4)
	if PtrSize <= 4 {
		if hi != 0 {
			return nowMaxInt()
		}
		return lo
	}
	v := lo + (hi << 16 << 16)
	if v < 0 {
		return nowMaxInt()
	}
	if hi != 0 && v < lo {
		return nowMaxInt()
	}
	return v
}

func nowDeltaWord(now uintptr, start uintptr) int {
	if now < start {
		return nowMaxInt()
	}
	delta := now - start
	if PtrSize <= 4 {
		max := uintptr(nowMaxInt())
		if delta > max {
			return nowMaxInt()
		}
	}
	v := int(delta)
	if v < 0 {
		return nowMaxInt()
	}
	return v
}
