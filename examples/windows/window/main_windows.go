//go:build windows

package main

import (
	"fmt"

	"x/os/windows/winui"
)

const (
	idLabel uint16 = 1001
	idEdit  uint16 = 1002
	idBtn   uint16 = 1003
)

func main() {
	w, err := winui.NewWindow("RTG winui demo", 520, 180)
	if err != 0 {
		panic("NewWindow failed")
	}

	label, lerr := w.CreateStatic(
		idLabel,
		"Type something and click Echo:",
		16,
		16,
		480,
		20,
	)
	if lerr != 0 {
		panic("CreateStatic failed")
	}

	edit, eerr := w.CreateEdit(idEdit, "", 16, 44, 360, 26)
	if eerr != 0 {
		panic("CreateEdit failed")
	}

	_, berr := w.CreateButton(idBtn, "Echo", 392, 44, 96, 26)
	if berr != 0 {
		panic("CreateButton failed")
	}

	winui.SetEchoCommand(idBtn, edit, label, "Echo: ")

	w.Show(winui.SW_SHOW)

	code, rerr := winui.Run()
	if rerr != 0 {
		fmt.Printf("Message loop failed: %v\n", rerr)
	}
	_ = code
}
