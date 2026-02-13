package main

import "os"
import "runtime"

func writeStr(s string) {
	ptr := runtime.Stringptr(s)
	n := len(s)
	runtime.Syscall(1, 1, ptr, uintptr(n), 0, 0, 0)
}

func main() {
	writeStr("test 1: ")
	a := 3
	b := 7
	c := a + b
	if c == 10 {
		writeStr("PASS\n")
	} else {
		writeStr("FAIL\n")
	}

	writeStr("test 2: ")
	if c > 5 && c < 20 {
		writeStr("PASS\n")
	} else {
		writeStr("FAIL\n")
	}

	writeStr("test 3: ")
	x := 1
	i := 0
	for i < 10 {
		x = x * 2
		i++
	}
	if x == 1024 {
		writeStr("PASS\n")
	} else {
		writeStr("FAIL\n")
	}

	os.Exit(0)
}
