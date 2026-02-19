package main

import "fmt"

func main() {
	ok := false
	switch x := 2; x {
	case 2:
		ok = true
	}
	if ok {
		fmt.Print("PASS")
		return
	}
	panic("switch init failed")
}
