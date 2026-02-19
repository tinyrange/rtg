package main

import "fmt"

func markDefer() {
	fmt.Print("PASS")
}

func run() {
	defer markDefer()
}

func main() {
	run()
}
