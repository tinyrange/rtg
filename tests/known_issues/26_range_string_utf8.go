package main

import "fmt"

func main() {
	s := "é"
	sum := 0
	for _, r := range s {
		sum += int(r)
	}
	fmt.Println(sum)
}
