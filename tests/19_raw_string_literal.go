package main

import "fmt"

func main() {
	s := `hello\nworld`
	if len(s) == 12 && s[5] == '\\' && s[6] == 'n' {
		fmt.Print("PASS")
		return
	}
	panic("raw string literal failed")
}
