package main

import (
	"fmt"
	"os"
)

type ReadWriter interface {
	Read() string
	Write(s string)
}

type Buffer struct {
	data string
}

func (b *Buffer) Read() string {
	return b.data
}

func (b *Buffer) Write(s string) {
	b.data = s
}

func useRW(rw ReadWriter) string {
	rw.Write("test data")
	return rw.Read()
}

func main() {
	passed := true

	buf := &Buffer{}
	result := useRW(buf)
	if result != "test data" {
		fmt.Printf("FAIL: multi method iface result=%s\n", result)
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
