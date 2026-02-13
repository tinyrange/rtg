package main

import "runtime"

func main() {
	buf := make([]byte, 5)
	buf[0] = 72
	runtime.Syscall(1, 1, runtime.Sliceptr(buf), 5, 0, 0, 0)
	runtime.Syscall(231, 0, 0, 0, 0, 0, 0)
}
