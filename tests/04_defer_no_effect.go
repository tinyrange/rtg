package main

import "fmt"

var marker int

func markDefer() {
	marker = 1
}

func run() {
	defer markDefer()
}

func main() {
	marker = 0
	run()
	if marker == 1 {
		fmt.Print("PASS")
		return
	}
	panic("defer did not run")
}
