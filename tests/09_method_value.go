package main

import "fmt"

type Person struct{ Name string }

func (p Person) Greet() string { return "hi " + p.Name }

func main() {
	p := Person{Name: "A"}
	f := p.Greet
	if f() == "hi A" {
		fmt.Print("PASS")
		return
	}
	panic("method value call failed")
}
