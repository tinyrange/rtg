package main

import "fmt"

func main() {
	var x any = 123
	switch v := x.(type) {
	case int:
		if v == 123 {
			fmt.Print("PASS")
			return
		}
	}
	panic("type switch failed")
}
