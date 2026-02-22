//go:build amd64

package main

import (
	"fmt"
	"os"

	"j5.nz/rtg/x/asm/amd64"
)

func add(a int, b int) int {
	return a + b
}

//rtg:assemble amd64
func f(x int) int {
	var a amd64.Assembler
	a.Load(a.Temp0, x)
	a.Call(a.Temp0, add, a.Temp0, a.Temp0)
	a.Push(a.Temp0)
	a.Ret()
}

func main() {
	if f(1) != 2 {
		fmt.Printf("FAIL\n")
		os.Exit(1)
	}
	if f(10) != 20 {
		fmt.Printf("FAIL\n")
		os.Exit(1)
	}
	fmt.Printf("PASS\n")
}
