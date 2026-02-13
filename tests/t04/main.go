package main

import "os"
import "runtime"

func main() {
	msg := "hello world\n"
	ptr := runtime.Stringptr(msg)
	runtime.Syscall(1, 1, ptr, uintptr(len(msg)), 0, 0, 0)
	os.Exit(0)
}
