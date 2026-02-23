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

	info, ierr := out.GetScreenBufferInfo()
	if ierr != 0 {
		panic("GetConsoleScreenBufferInfo failed")
	}

	left := winconsole.ScreenBufferInfo.WindowLeft(info)
	top := winconsole.ScreenBufferInfo.WindowTop(info)
	width := uint32(winconsole.ScreenBufferInfo.WindowRight(info)-left) + 1

	_, _ = out.FillOutputCharacter('=', width, winconsole.Coord{X: left, Y: top})
	_, _ = out.FillOutputAttribute(
		winconsole.ForegroundRed|winconsole.ForegroundGreen|winconsole.ForegroundIntensity,
		width,
		winconsole.Coord{X: left, Y: top},
	)
	_ = out.SetCursorPosition(winconsole.Coord{X: left, Y: top + 1})

	_ = out.SetTextAttribute(winconsole.ForegroundGreen | winconsole.ForegroundIntensity)
	_, _ = out.WriteString("green text\r\n")
	_ = out.SetTextAttribute(winconsole.ForegroundRed | winconsole.ForegroundIntensity)
	_, _ = out.WriteString("red text\r\n")
	_ = out.SetTextAttribute(winconsole.ScreenBufferInfo.AttributesValue(info))

	fmt.Println("colors example done")
}
