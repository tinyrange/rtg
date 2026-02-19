package main

import "fmt"

var order string

func markDefer() {
	order = order + "d"
}

func run() {
	defer markDefer()
	order = order + "b"
}

func main() {
	order = ""
	run()
	if order == "bd" {
		fmt.Print("PASS")
		return
	}
	panic("defer did not run")
}
