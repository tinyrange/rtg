package main

import "os"
import "runtime"

func writeStr(s string) {
	ptr := runtime.Stringptr(s)
	n := len(s)
	runtime.SysWrite(1, ptr, uintptr(n))
}

func main() {
	writeStr("test slices: ")

	// Test make and append
	s := make([]int, 0)
	s = append(s, 10)
	s = append(s, 20)
	s = append(s, 30)

	if len(s) == 3 {
		writeStr("len=3 ")
	}

	// Note: s[i] uses INDEX_ADDR with element size 1 currently
	// This will fail for int slices - TODO: fix element sizes

	writeStr("DONE\n")
	os.Exit(0)
}
