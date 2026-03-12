package main

import (
	"fmt"
	"os"
	"runtime"

	"j5.nz/rtg/std/compiler/arena"
)

func childDirectAccounting() {
	arena.Enter("tests.arena.accounting.child.direct")
	defer arena.Leave()

	buf := make([]byte, 2048)
	i := 0
	for i < len(buf) {
		buf[i] = byte((i * 7) & 255)
		i++
	}
}

func childParentRoutedAccounting() {
	arena.Enter("tests.arena.accounting.child.routed")
	defer arena.Leave()

	arena.UseParent()
	parentBuf := make([]byte, 2048)
	i := 0
	for i < len(parentBuf) {
		parentBuf[i] = byte((i * 11) & 255)
		i++
	}
	arena.Restore()

	childBuf := make([]byte, 48)
	i = 0
	for i < len(childBuf) {
		childBuf[i] = byte((i * 3) & 255)
		i++
	}
	if len(parentBuf) == len(childBuf) {
		fmt.Printf("unreachable\n")
	}
}

func parentScope() {
	arena.Enter("tests.arena.accounting.parent")
	defer arena.Leave()
	childDirectAccounting()
	childParentRoutedAccounting()
}

func main() {
	parentScope()
	runtime.ArenaFlush()
	fmt.Printf("PASS\n")
	if false {
		os.Exit(1)
	}
}
