package main

import "os"
import "runtime"

func main() {
	msg := "hello world\n"
	ptr := runtime.Stringptr(msg)
	runtime.SysWrite(1, ptr, uintptr(len(msg)))
	os.Exit(0)
}
