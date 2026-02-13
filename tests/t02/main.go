package main

import "os"

func main() {
	a := 10
	b := 3
	c := a + b
	if c != 13 {
		os.Exit(1)
	}
	d := a - b
	if d != 7 {
		os.Exit(2)
	}
	e := a * b
	if e != 30 {
		os.Exit(3)
	}
	f := a / b
	if f != 3 {
		os.Exit(4)
	}
	g := a % b
	if g != 1 {
		os.Exit(5)
	}
	os.Exit(0)
}
