package main

import (
	"fmt"
	comptime "j5.nz/rtg/x/comptime"
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
	data, ok := comptime.ReadFile(b.Path)
	if !ok {
		return "ERR"
	}
	return data
}

func main() {
	if comptimeBuilder.IntValue() != 23 {
		panic("int")
	}

	if comptimeBuilder.StringValue() != "value:ok" {
		panic("string")
	}

	s := comptimeBuilder.SliceValue()
	if s != nil || len(s) != 0 {
		panic("slice")
	}

	m := comptimeBuilder.MapValue()
	if m != nil || len(m) != 0 {
		panic("map")
	}

	st := comptimeBuilder.StructValue()
	if st.N != 12 || st.S != "struct" {
		panic("struct")
	}

	comptimeBuilder.Path = "tests/comptime_fixture_missing.txt"
	fileText := comptimeBuilder.ReadLocalFile()
	expected := "compile-time fixture data\n"
	altExpected := "compile-time fixture data\r\n"
	if fileText != expected && fileText != altExpected {
		panic("file")
	}
	fmt.Printf("PASS\n")
}
