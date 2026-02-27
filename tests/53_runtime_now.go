//go:build !dos && !baremetal

package main

import (
	"fmt"
	"os"
	"runtime"
)

func waitForIncrease(prev int) int {
	cur := prev
	i := 0
	for i < 5000000 {
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
		fmt.Printf("FAIL: now did not increase (t0=%d t1=%d)\n", t0, t1)
		os.Exit(1)
	}
	t2 := waitForIncrease(t1)
	if t2 <= t1 {
		fmt.Printf("FAIL: now did not keep increasing (t1=%d t2=%d)\n", t1, t2)
		os.Exit(1)
	}
	fmt.Printf("PASS\n")
}
