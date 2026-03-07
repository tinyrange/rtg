//go:build !dos && !baremetal && !armv8m

package main

import (
	"runtime"
)

func waitForIncrease(prev int) int {
	cur := prev
	i := 0
	for i < 200000000 {
		cur = runtime.Now()
		if cur > prev {
			return cur
		}
		i = i + 1
	}
	return cur
}

func main() {
	t0 := runtime.Now()
	t1 := waitForIncrease(t0)
	if t1 <= t0 {
		println("FAIL: now did not increase", t0, t1)
		panic("runtime.Now did not increase")
	}
	t2 := waitForIncrease(t1)
	if t2 <= t1 {
		println("FAIL: now did not keep increasing", t1, t2)
		panic("runtime.Now did not keep increasing")
	}
	println("PASS")
}
