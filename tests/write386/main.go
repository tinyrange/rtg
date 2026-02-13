package main

import "runtime"

func main() {
	msg := "Hello i386!\n"
	runtime.SysWrite(1, runtime.Stringptr(msg), uintptr(len(msg)))
}
