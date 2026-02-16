package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	passed := true

	var b strings.Builder
	b.WriteString("hello")
	b.WriteString(" ")
	b.WriteString("world")

	if b.String() != "hello world" {
		fmt.Printf("FAIL: builder result=%s\n", b.String())
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
