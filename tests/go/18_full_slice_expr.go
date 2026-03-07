package main

import "fmt"

func main() {
	s := []int{1, 2, 3, 4}
	t := s[1:3:3]
	if len(t) == 2 && cap(t) == 2 && t[0] == 2 && t[1] == 3 {
		fmt.Print("PASS")
		return
	}
	panic("full slice expression failed")
}
