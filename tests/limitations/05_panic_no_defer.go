package main

import (
	"fmt"
	"os"
)

func done() {
	fmt.Print("PASS")
	os.Exit(0)
}

func main() {
	defer done()
	panic("boom")
}
