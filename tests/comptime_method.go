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
func (b ComptimeBuilder) IntValue(ctx comptime.Context) int {
	_ = ctx
	return b.Base*3 + 2
}

//rtg:comptime
func (b ComptimeBuilder) StringValue(ctx comptime.Context) string {
	_ = ctx
	return "value:" + "ok"
}

//rtg:comptime
func (b ComptimeBuilder) SliceValue(ctx comptime.Context) []int {
	_ = ctx
	return nil
}

//rtg:comptime
func (b ComptimeBuilder) MapValue(ctx comptime.Context) map[string]int {
	_ = ctx
	return nil
}

//rtg:comptime
func (b ComptimeBuilder) StructValue(ctx comptime.Context) ComptimeStruct {
	_ = ctx
	return ComptimeStruct{N: b.Base + 5, S: "struct"}
}

//rtg:comptime
func (b ComptimeBuilder) ReadLocalFile(ctx comptime.Context) string {
	data, ok := ctx.ReadFile(b.Path)
	if !ok {
		return "ERR"
	}
	return data
}

func main() {
	if comptimeBuilder.IntValue(comptime.Host()) != 23 {
		panic("int")
	}

	if comptimeBuilder.StringValue(comptime.Host()) != "value:ok" {
		panic("string")
	}

	s := comptimeBuilder.SliceValue(comptime.Host())
	if s != nil || len(s) != 0 {
		panic("slice")
	}

	m := comptimeBuilder.MapValue(comptime.Host())
	if m != nil || len(m) != 0 {
		panic("map")
	}

	st := comptimeBuilder.StructValue(comptime.Host())
	if st.N != 12 || st.S != "struct" {
		panic("struct")
	}

	comptimeBuilder.Path = "tests/comptime_fixture_missing.txt"
	fileText := comptimeBuilder.ReadLocalFile(comptime.Host())
	expected := "compile-time fixture data\n"
	altExpected := "compile-time fixture data\r\n"
	if fileText != expected && fileText != altExpected {
		panic("file")
	}
	fmt.Printf("PASS\n")
}
