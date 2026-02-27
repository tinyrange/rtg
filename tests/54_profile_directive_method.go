package main

import "fmt"

type profileCounter struct {
	base int
}

//rtg:profile
func (c profileCounter) Sum(n int) int {
	total := 0
	i := 0
	for i <= n {
		total = total + c.base + i
		i++
	}
	return total
}

func main() {
	c := profileCounter{base: 2}
	if c.Sum(4) != 20 {
		panic("profiled method returned wrong value")
	}
	fmt.Print("PASS")
}
