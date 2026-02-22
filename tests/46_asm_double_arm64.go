//go:build arm64

package main

import (
	"fmt"
	"os"

	"j5.nz/rtg/x/asm/arm64"
)

//rtg:assemble arm64
func f(x int) int {
	var a arm64.Assembler
	a.Load(a.Temp0, x)
	a.Mul(a.Temp0, 3)
	a.Add(a.Temp0, a.Temp0)
	a.Push(a.Temp0)
	a.Ret()
}

func main() {
	if f(2) != 12 {
		fmt.Printf("FAIL\n")
		os.Exit(1)
	}
	if f(-1) != -6 {
		fmt.Printf("FAIL\n")
		os.Exit(1)
	}
	fmt.Printf("PASS\n")
}
