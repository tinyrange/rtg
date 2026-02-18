package main

import (
	f "fmt"
	"os"
)

func main() {
	passed := true

	s := f.Sprintf("%s-%d", "alias", 1)
	if s != "alias-1" {
		f.Printf("FAIL: alias import call=%q\n", s)
		passed = false
	}

	if passed {
		f.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
