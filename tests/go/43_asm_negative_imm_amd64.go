//go:build amd64

package main

import (
	"fmt"
	"os"

	"j5.nz/rtg/x/asm/amd64"
)

//rtg:assemble amd64
func mulNeg(x int) int {
	var a amd64.Assembler
	a.Load(a.Temp0, x)
	a.Mul(a.Temp0, -3)
	a.Push(a.Temp0)
	a.Ret()
}

func main() {
	if mulNeg(7) != -21 {
		fmt.Printf("FAIL\n")
		os.Exit(1)
	}
	if mulNeg(-2) != 6 {
		fmt.Printf("FAIL\n")
		os.Exit(1)
	}
	fmt.Printf("PASS\n")
}
