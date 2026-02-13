package main

import "runtime"

func main() {
	buf := make([]byte, 5)
	buf[0] = 72
	runtime.SysWrite(1, runtime.Sliceptr(buf), 5)
	runtime.SysExit(0)
}
