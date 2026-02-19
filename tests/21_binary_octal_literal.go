package main

import "fmt"

const b = 0b1010
const o = 0o12

func main() {
	if b == 10 && o == 10 {
		fmt.Print("PASS")
		return
	}
	panic("binary/octal literals failed")
}
