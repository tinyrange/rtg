//go:build arm64

package main

import (
	"fmt"
	"os"

	"j5.nz/rtg/x/asm/arm64"
)

func plus9(x int) int {
	return x + 9
}

//rtg:assemble arm64
func g(x int) int {
	var a arm64.Assembler
	a.Load(a.Temp0, x)
	a.Call(a.Temp0, plus9, a.Temp0)
	a.Push(a.Temp0)
	a.Ret()
}

func main() {
	if g(1) != 10 {
		fmt.Printf("FAIL\n")
		os.Exit(1)
	}
	if g(10) != 19 {
		fmt.Printf("FAIL\n")
		os.Exit(1)
	}
	fmt.Printf("PASS\n")
}
