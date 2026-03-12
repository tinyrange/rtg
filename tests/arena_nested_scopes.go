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
		buf := make([]byte, 0, 384)
		j := 0
		for j < 384 {
			buf = append(buf, byte((i*7+j)&255))
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

func retainedChildMap() map[string]int {
	arena.Enter("tests.arena.child")
	defer arena.Leave()

	m := make(map[string]int)
	m["oak"] = 3
	m["pine"] = 5
	m["elm"] = 8
	arena.RetainCurrent()
	return m
}

func nestedRetainedValues() ([]byte, map[string]int) {
	arena.Enter("tests.arena.outer")
	defer arena.Leave()

	arena.UseParent()
	buf := fillBytes("nested outer parent value")
	arena.Restore()

	m := retainedChildMap()
	arena.RetainCurrent()
	return buf, m
}

func main() {
	passed := true

	buf, m := nestedRetainedValues()
	churnArena(256)

	if !equalBytes(buf, "nested outer parent value") {
		fmt.Printf("FAIL: nested parent value changed\n")
		passed = false
	}
	if len(m) != 3 || m["oak"] != 3 || m["pine"] != 5 || m["elm"] != 8 {
		fmt.Printf("FAIL: nested retained map contents changed len=%d oak=%d pine=%d elm=%d\n", len(m), m["oak"], m["pine"], m["elm"])
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
