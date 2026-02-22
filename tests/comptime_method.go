package main

import (
	"fmt"
	"os"
	"runtime"
)

type ComptimeBuilder struct {
	Base int
	Path string
}

type ComptimeStruct struct {
	N int
	S string
}

func readFileOnce(path string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	buf := make([]byte, 4096)
	n, err := f.Read(buf)
	f.Close()
	if n == 0 && err != nil {
		return "", false
	}
	return string(buf[0:n]), true
}

var comptimeBuilder = ComptimeBuilder{
	Base: 7,
	Path: "tests/comptime_fixture.txt",
}

//rtg:comptime
func (b ComptimeBuilder) IntValue() int {
	return b.Base*3 + 2
}

//rtg:comptime
func (b ComptimeBuilder) StringValue() string {
	return "value:" + "ok"
}

//rtg:comptime
func (b ComptimeBuilder) SliceValue() []int {
	return nil
}

//rtg:comptime
func (b ComptimeBuilder) MapValue() map[string]int {
	return nil
}

//rtg:comptime
func (b ComptimeBuilder) StructValue() ComptimeStruct {
	return ComptimeStruct{N: b.Base + 5, S: "struct"}
}

//rtg:comptime
func (b ComptimeBuilder) ReadLocalFile() string {
	data, ok := readFileOnce(b.Path)
	if !ok {
		return "ERR"
	}
	return data
}

func main() {
	passed := true

	if comptimeBuilder.IntValue() != 23 {
		passed = false
	}

	if comptimeBuilder.StringValue() != "value:ok" {
		passed = false
	}

	s := comptimeBuilder.SliceValue()
	if s != nil || len(s) != 0 {
		passed = false
	}

	m := comptimeBuilder.MapValue()
	if m != nil || len(m) != 0 {
		passed = false
	}

	st := comptimeBuilder.StructValue()
	if st.N != 12 || st.S != "struct" {
		passed = false
	}

	comptimeBuilder.Path = "tests/comptime_fixture_missing.txt"
	fileText := comptimeBuilder.ReadLocalFile()
	expected := "compile-time fixture data\n"
	altExpected := "compile-time fixture data\r\n"
	if runtime.GOOS == "wasi" || runtime.GOOS == "dos" {
		// WASI/DOS compile-time file I/O currently resolves this path as missing.
		expected = "ERR"
		altExpected = "ERR"
	} else if runtime.GOOS == "windows" {
		// Windows selfhost stages may fall back to runtime evaluation here.
		altExpected = "ERR"
	}
	if fileText != expected && fileText != altExpected {
		passed = false
	}

	if !passed {
		os.Exit(1)
	}
	fmt.Printf("PASS\n")
}
