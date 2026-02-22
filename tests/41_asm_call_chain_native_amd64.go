//go:build amd64

package main

import (
	"fmt"
	"os"

	"j5.nz/rtg/x/asm/amd64"
)

//rtg:assemble amd64
func g(x int) int {
	var a amd64.Assembler
	a.Load(a.Temp0, x)
	a.Mul(a.Temp0, 2)
	a.Push(a.Temp0)
	a.Ret()
}

//rtg:assemble amd64
func f(x int) int {
	var a amd64.Assembler
	a.Load(a.Temp0, x)
	a.Call(a.Temp0, g, a.Temp0)
	a.Mul(a.Temp0, 5)
	a.Push(a.Temp0)
	a.Ret()
}

func main() {
	if f(3) != 30 {
		fmt.Printf("FAIL\n")
		os.Exit(1)
	}
	fmt.Printf("PASS\n")
}
