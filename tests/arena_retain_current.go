package main

import (
	"fmt"
	"os"

	"j5.nz/rtg/std/compiler/arena"
)

func churnArena(rounds int) {
	i := 0
	for i < rounds {
		arena.Enter("tests.arena.churn")
		buf := make([]byte, 0, 512)
		j := 0
		for j < 512 {
			buf = append(buf, byte((i+j)&255))
			j++
		}
		if len(buf) == 513 {
			fmt.Printf("unreachable\n")
		}
		arena.Leave()
		i++
	}
}

func fillBytes(s string) []byte {
	buf := make([]byte, len(s))
	i := 0
	for i < len(s) {
		buf[i] = s[i]
		i++
	}
	return buf
}

func equalBytes(buf []byte, s string) bool {
	if len(buf) != len(s) {
		return false
	}
	i := 0
	for i < len(s) {
		if buf[i] != s[i] {
			return false
		}
		i++
	}
	return true
}

func retainedSlice() []byte {
	arena.Enter("tests.arena.retainedSlice")
	defer arena.Leave()

	buf := fillBytes("retained slice survives arena leave")
	arena.RetainCurrent()
	return buf
}

func retainedMap() map[string]int {
	arena.Enter("tests.arena.retainedMap")
	defer arena.Leave()

	m := make(map[string]int)
	m["alpha"] = 11
	m["beta"] = 29
	m["gamma"] = 47
	arena.RetainCurrent()
	return m
}

func main() {
	passed := true

	buf := retainedSlice()
	m := retainedMap()
	churnArena(256)

	if !equalBytes(buf, "retained slice survives arena leave") {
		fmt.Printf("FAIL: retained slice contents changed\n")
		passed = false
	}
	if len(m) != 3 || m["alpha"] != 11 || m["beta"] != 29 || m["gamma"] != 47 {
		fmt.Printf("FAIL: retained map contents changed len=%d alpha=%d beta=%d gamma=%d\n", len(m), m["alpha"], m["beta"], m["gamma"])
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
