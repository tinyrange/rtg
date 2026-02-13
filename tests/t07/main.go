package main

import "runtime"

func main() {
	// Test make + byte slice operations
	buf := make([]byte, 5)
	buf[0] = 72
	buf[1] = 101
	buf[2] = 108
	buf[3] = 108
	buf[4] = 111
	runtime.Syscall(1, 1, runtime.Sliceptr(buf), 5, 0, 0, 0)

	// Test append
	buf2 := make([]byte, 0)
	buf2 = append(buf2, 10)
	runtime.Syscall(1, 1, runtime.Sliceptr(buf2), uintptr(len(buf2)), 0, 0, 0)

	runtime.Syscall(231, 0, 0, 0, 0, 0, 0)
}
