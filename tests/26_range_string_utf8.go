package main

import "fmt"

func main() {
	s := "é"
	sum := 0
	count := 0
	for _, r := range s {
		sum += int(r)
		count++
	}
	if sum == int('é') && count == 1 {
		fmt.Print("PASS")
		return
	}
	panic("range over utf8 string failed")
}
