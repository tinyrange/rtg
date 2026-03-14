package main

import (
	"fmt"
	"os"

	"j5.nz/rtg/std/compiler/arena"
)

func makeDynamicString(s string) string {
	buf := make([]byte, len(s))
	i := 0
	for i < len(s) {
		buf[i] = s[i]
		i++
	}
	return string(buf)
}

func churnStrings(rounds int) {
	i := 0
	for i < rounds {
		arena.Enter("tests.arena.string.churn")
		s := makeDynamicString("arena child string must not alias scratch memory after leave")
		if len(s) == 0 {
			fmt.Printf("unreachable\n")
		}
		arena.Leave()
		i++
	}
}

func parentRoutedString() []string {
	arena.Enter("tests.arena.string.child")
	defer arena.Leave()

	s := makeDynamicString("arena child string must not alias scratch memory after leave")

	arena.UseParent()
	out := make([]string, 1)
	out[0] = s
	arena.Restore()
	return out
}

func main() {
	passed := true

	out := parentRoutedString()
	churnStrings(256)
	want := "arena child string must not alias scratch memory after leave"

	if len(out) != 1 || out[0] != want {
		fmt.Printf("FAIL: routed string changed got=%q want=%q\n", out[0], want)
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
