package main

import "fmt"

func main() {
	numbers := []int{3, 5, 8, 13}
	total := 0
	for _, n := range numbers {
		total += n
	}

	labels := map[string]int{
		"count": len(numbers),
		"sum":   total,
	}

	fmt.Println("numbers:", numbers)
	fmt.Println("labels:", labels)
}
