package main

import (
	"fmt"
	f "fmt"
)

func main() {
	s := f.Sprintf("%s", "x")
	if s == "x" {
		fmt.Print("PASS")
		return
	}
	panic("import alias failed")
}
