package main

import (
	"fmt"
	"os"
)

func factorial(n int) int {
	if n <= 1 {
		return 1
	}
	return n * factorial(n-1)
}

func fib(n int) int {
	if n <= 1 {
		return n
	}
	return fib(n-1) + fib(n-2)
}

func isEven(n int) bool {
	if n == 0 { return true }
	return isOdd(n - 1)
}

func isOdd(n int) bool {
	if n == 0 { return false }
	return isEven(n - 1)
}

func main() {
	passed := true

	if factorial(5) != 120 {
		fmt.Printf("FAIL: factorial(5)=%d\n", factorial(5))
		passed = false
	}

	if fib(10) != 55 {
		fmt.Printf("FAIL: fib(10)=%d\n", fib(10))
		passed = false
	}

	// Mutual recursion
	if !isEven(10) {
		fmt.Printf("FAIL: isEven(10)\n")
		passed = false
	}
	if !isOdd(11) {
		fmt.Printf("FAIL: isOdd(11)\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
