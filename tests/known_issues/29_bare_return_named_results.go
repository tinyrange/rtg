package main

import "fmt"

func f() (a int) { a = 3; return }

func main() {
	fmt.Println("before")
	_ = f()
	fmt.Println("after")
}
