package main

import (
	"fmt"
	"os"
)

var panicOrder string

func appendPanicDigit(n int) {
	panicOrder = panicOrder + string([]byte{byte('0' + n)})
}

func done() {
	if panicOrder == "21" {
		fmt.Print("PASS")
		os.Exit(0)
	}
	fmt.Printf("FAIL: panic defer order=%s\n", panicOrder)
	os.Exit(1)
}

func main() {
	panicOrder = ""
	defer done()
	i := 1
	for i <= 2 {
		defer appendPanicDigit(i)
		i++
	}
	panic("boom")
}
