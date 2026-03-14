package main

import (
	"fmt"
	"os"
	"runtime"

	"j5.nz/rtg/std/compiler/arena"
)

func childDeferredRouting() {
	arena.Enter("tests.arena.defer.child")
	defer arena.Leave()

	arena.UseParent()
	defer arena.Restore()

	buf := make([]byte, 2048)
	i := 0
	for i < len(buf) {
		buf[i] = byte((i * 5) & 255)
		i++
	}
}

func parentDeferredRouting() {
	arena.Enter("tests.arena.defer.parent")
	defer arena.Leave()

	childDeferredRouting()

	after := make([]byte, 2048)
	i := 0
	for i < len(after) {
		after[i] = byte((i * 9) & 255)
		i++
	}
}

func main() {
	parentDeferredRouting()
	runtime.ArenaFlush()
	fmt.Printf("PASS\n")
	if false {
		os.Exit(1)
	}
}
