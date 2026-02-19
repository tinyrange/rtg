package main

import "fmt"

func main() {
	if 'é' == 233 {
		fmt.Print("PASS")
		return
	}
	panic("utf8 rune literal failed")
}
