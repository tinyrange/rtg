package main

import (
	"fmt"
	"os"
)

type Greeting struct {
	msg string
}

func (g Greeting) Message() string {
	return g.msg
}

func main() {
	g := Greeting{msg: "hello world"}
	fmt.Printf("msg: %s\n", g.Message())
	if g.Message() == "" {
		fmt.Printf("ERROR: empty message\n")
		os.Exit(1)
	}
	fmt.Printf("done\n")
}
