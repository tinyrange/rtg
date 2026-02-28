//go:build amd64

package main

import (
	"fmt"
	"os"

	"j5.nz/rtg/x/asm/amd64"
)

func plus7(x int) int {
	return x + 7
}

//rtg:assemble amd64
func f(x int) int {
	var a amd64.Assembler
	a.Load(a.Temp0, x)
	a.Call(a.Temp0, plus7, a.Temp0)
	a.Mul(a.Temp0, 3)
	a.Push(a.Temp0)
	a.Ret()
}

func main() {
	if f(1) != 24 {
		fmt.Printf("FAIL\n")
		os.Exit(1)
	}
	if f(10) != 51 {
		fmt.Printf("FAIL\n")
		os.Exit(1)
	}
	fmt.Printf("PASS\n")
}
