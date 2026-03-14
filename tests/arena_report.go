package main

import (
	"fmt"
	"os"
	"runtime"

	"j5.nz/rtg/std/compiler/arena"
)

func reportInner() []byte {
	arena.Enter("tests.arena.report.inner")
	defer arena.Leave()

	buf := make([]byte, 0, 768)
	i := 0
	for i < 768 {
		buf = append(buf, byte((i*9)&255))
		i++
	}
	arena.RetainCurrent()
	return buf
}

func reportOuter() []byte {
	arena.Enter("tests.arena.report.outer")
	defer arena.Leave()

	buf := reportInner()
	arena.RetainCurrent()
	return buf
}

func reportChurn(rounds int) {
	i := 0
	for i < rounds {
		arena.Enter("tests.arena.report.churn")
		tmp := make([]byte, 0, 320)
		j := 0
		for j < 320 {
			tmp = append(tmp, byte((i+j)&255))
			j++
		}
		if len(tmp) == 321 {
			fmt.Printf("unreachable\n")
		}
		arena.Leave()
		i++
	}
}

func main() {
	buf := reportOuter()
	reportChurn(64)
	runtime.ArenaFlush()
	if len(buf) != 768 {
		fmt.Printf("FAIL: len=%d\n", len(buf))
		os.Exit(1)
	}
	fmt.Printf("PASS\n")
}
