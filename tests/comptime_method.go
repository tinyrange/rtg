package main

import (
	"os"
)

type ComptimeBuilder struct {
	Base int
	Path string
}

type ComptimeStruct struct {
	N int
	S string
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
	return []int{b.Base, b.Base + 1, b.Base + 2}
}

//rtg:comptime
func (b ComptimeBuilder) MapValue() map[string]int {
	m := make(map[string]int)
	m["base"] = b.Base
	m["double"] = b.Base * 2
	return m
}

//rtg:comptime
func (b ComptimeBuilder) StructValue() ComptimeStruct {
	return ComptimeStruct{N: b.Base + 5, S: "struct"}
}

//rtg:comptime
func (b ComptimeBuilder) ReadLocalFile() string {
	data, err := os.ReadFile(b.Path)
	if err != nil {
		return "ERR"
	}
	return string(data)
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
	if len(s) != 3 || s[0] != 7 || s[1] != 8 || s[2] != 9 {
		passed = false
	}

	m := comptimeBuilder.MapValue()
	if len(m) != 2 || m["base"] != 7 || m["double"] != 14 {
		passed = false
	}

	st := comptimeBuilder.StructValue()
	if st.N != 12 || st.S != "struct" {
		passed = false
	}

	orig, err := os.ReadFile(comptimeBuilder.Path)
	if err != nil {
		passed = false
	}
	comptimeBuilder.Path = "tests/comptime_fixture_missing.txt"
	fileText := comptimeBuilder.ReadLocalFile()
	if fileText != string(orig) {
		passed = false
	}

	if !passed {
		os.Exit(1)
	}
}
