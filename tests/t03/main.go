package main

import "os"

func fib(n int) int {
	if n <= 1 {
		return n
	}
	return fib(n-1) + fib(n-2)
}

func main() {
	result := fib(10)
	os.Exit(result)
}
