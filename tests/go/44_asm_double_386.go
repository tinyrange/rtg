//go:build 386

package main

import (
	"fmt"
	"os"

	"j5.nz/rtg/x/asm/i386"
)

//rtg:assemble 386
func f(x int) int {
	var a i386.Assembler
	a.Load(a.Temp0, x)
	a.Mul(a.Temp0, 2)
	a.Add(a.Temp0, a.Temp0)
	a.Push(a.Temp0)
	a.Ret()
}

func main() {
	if f(3) != 12 {
		fmt.Printf("FAIL\n")
		os.Exit(1)
	}
	if f(-2) != -8 {
		fmt.Printf("FAIL\n")
		os.Exit(1)
	}
	fmt.Printf("PASS\n")
}
