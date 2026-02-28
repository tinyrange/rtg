package main

import "fmt"

func main() {
	ok := false
	goto done
	ok = false
done:
	ok = true
	if ok {
		fmt.Print("PASS")
		return
	}
	panic("goto forward label failed")
}
