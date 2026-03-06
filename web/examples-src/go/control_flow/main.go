package main

import "fmt"

func classify(n int) string {
	switch {
	case n%15 == 0:
		return "fizzbuzz"
	case n%3 == 0:
		return "fizz"
	case n%5 == 0:
		return "buzz"
	default:
		return fmt.Sprintf("%d", n)
	}
}

func main() {
	for i := 1; i <= 15; i++ {
		fmt.Println(classify(i))
	}
}
