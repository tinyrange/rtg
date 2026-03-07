//go:build amd64

package main

import (
	"fmt"
	"os"

	"j5.nz/rtg/x/asm/amd64"
)

//rtg:assemble amd64
func scale(x int) int {
	var a amd64.Assembler
	a.Load(a.Temp0, x)
	a.Mul(a.Temp0, 3)
	a.Add(a.Temp0, a.Temp0) // x*6
	a.Mul(a.Temp0, 2)       // x*12
	a.Push(a.Temp0)
	a.Ret()
}

func main() {
	if scale(1) != 12 {
		fmt.Printf("FAIL\n")
		os.Exit(1)
	}
	if scale(7) != 84 {
		fmt.Printf("FAIL\n")
		os.Exit(1)
	}
	fmt.Printf("PASS\n")
}
