//go:build amd64

package main

import (
	"fmt"
	"os"

	"j5.nz/rtg/x/asm/amd64"
)

//rtg:assemble amd64
func mulBig(x int) int {
	var a amd64.Assembler
	a.Load(a.Temp0, x)
	a.Mul(a.Temp0, 100000)
	a.Push(a.Temp0)
	a.Ret()
}

func main() {
	if mulBig(2) != 200000 {
		fmt.Printf("FAIL\n")
		os.Exit(1)
	}
	fmt.Printf("PASS\n")
}
