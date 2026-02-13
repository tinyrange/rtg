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
	runtime.SysWrite(1, runtime.Sliceptr(buf), 5)

	// Test append
	buf2 := make([]byte, 0)
	buf2 = append(buf2, 10)
	runtime.SysWrite(1, runtime.Sliceptr(buf2), uintptr(len(buf2)))

	runtime.SysExit(0)
}
