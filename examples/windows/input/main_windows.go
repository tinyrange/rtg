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

	_ = winconsole.SetTitle("RTG winconsole input example")
	_, _ = out.WriteString("Type a line and press Enter: ")
	line, rerr := in.ReadString(120)
	if rerr != 0 {
		panic("ReadConsoleA failed")
	}

	_, _ = out.WriteString("\r\nEcho: " + line + "\r\n")
	title, terr := winconsole.GetTitle(200)
	if terr == 0 {
		fmt.Printf("Console title: %s\r\n", title)
	}
	_ = in.FlushInputBuffer()
}
