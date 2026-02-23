//go:build windows

package main

import (
	"fmt"

	"os/winconsole"
)

func main() {
	out, err := winconsole.Stdout()
	if err != 0 {
		panic("Stdout handle unavailable")
	}
	in, err := winconsole.Stdin()
	if err != 0 {
		panic("Stdin handle unavailable")
	}

	_ = winconsole.SetTitle("RTG winconsole basic example")
	_ = winconsole.EnableUTF8Console(out, in)

	mode, _ := out.GetMode()
	events, _ := in.InputEventCount()

	_, _ = out.WriteString("winconsole basic example\r\n")
	fmt.Printf("Output mode: 0x%x\r\n", mode)
	fmt.Printf("Pending input events: %d\r\n", events)
}
