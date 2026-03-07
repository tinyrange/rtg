//go:build 386

package main

import (
	"fmt"
	"os"

	"j5.nz/rtg/x/asm/i386"
)

func plus11(x int) int {
	return x + 11
}

//rtg:assemble 386
func g(x int) int {
	var a i386.Assembler
	a.Load(a.Temp0, x)
	a.Call(a.Temp0, plus11, a.Temp0)
	a.Push(a.Temp0)
	a.Ret()
}

func main() {
	if g(1) != 12 {
		fmt.Printf("FAIL\n")
		os.Exit(1)
	}
	if g(9) != 20 {
		fmt.Printf("FAIL\n")
		os.Exit(1)
	}
	fmt.Printf("PASS\n")
}
