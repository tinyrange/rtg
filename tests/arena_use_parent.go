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
		buf := make([]byte, 0, 256)
		j := 0
		for j < 256 {
			buf = append(buf, byte((i*3+j)&255))
			j++
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

func parentSlice() []byte {
	arena.Enter("tests.arena.parentSlice")
	defer arena.Leave()

	arena.UseParent()
	buf := fillBytes("parent-routed slice survives child leave")
	arena.Restore()

	scratch := make([]byte, 0, 1024)
	i := 0
	for i < 1024 {
		scratch = append(scratch, byte(i&255))
		i++
	}
	if len(scratch) == 0 {
		fmt.Printf("unreachable\n")
	}
	return buf
}

func parentMap() map[string]int {
	arena.Enter("tests.arena.parentMap")
	defer arena.Leave()

	arena.UseParent()
	m := make(map[string]int)
	m["left"] = 7
	m["right"] = 13
	m["center"] = 29
	arena.Restore()

	tmp := make([]byte, 0, 512)
	i := 0
	for i < 512 {
		tmp = append(tmp, byte((i*5)&255))
		i++
	}
	if len(tmp) == 1025 {
		fmt.Printf("unreachable\n")
	}
	return m
}

func main() {
	passed := true

	buf := parentSlice()
	m := parentMap()
	churnArena(256)

	if !equalBytes(buf, "parent-routed slice survives child leave") {
		fmt.Printf("FAIL: parent-routed slice contents changed\n")
		passed = false
	}
	if len(m) != 3 || m["left"] != 7 || m["right"] != 13 || m["center"] != 29 {
		fmt.Printf("FAIL: parent-routed map contents changed len=%d left=%d right=%d center=%d\n", len(m), m["left"], m["right"], m["center"])
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
