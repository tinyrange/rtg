//go:build amd64

package main

import (
	"fmt"
	"os"

	"j5.nz/rtg/x/asm/amd64"
)

func id(x int) int {
	return x
}

//rtg:assemble amd64
func takeFirst(a0, a1, a2 int) int {
	var a amd64.Assembler
	a.Load(a.Temp0, a0)
	a.Push(a.Temp0)
	a.Ret()
}

func main() {
	x := takeFirst(1, 2, 3)
	y := id(123)
	if x != 1 || y != 123 {
		fmt.Printf("FAIL\n")
		os.Exit(1)
	}
	fmt.Printf("PASS\n")
}
