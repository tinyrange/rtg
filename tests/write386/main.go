package main

import (
	"os"
	"runtime"
)

func main() {
	msg := "Hello i386!\n"
	runtime.Syscall(os.SYS_WRITE, 1, runtime.Stringptr(msg), uintptr(len(msg)), 0, 0, 0)
}
