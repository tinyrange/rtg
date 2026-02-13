package main

import (
	"fmt"
	"os"
	"strings"
)

func assert(name string, got string, want string) {
	if got != want {
		fmt.Fprintf(os.Stderr, "FAIL %s: got %q want %q\n", name, got, want)
		os.Exit(1)
	}
	fmt.Printf("PASS %s\n", name)
}

func assertBool(name string, got bool, want bool) {
	if got != want {
		fmt.Fprintf(os.Stderr, "FAIL %s: got %v want %v\n", name, got, want)
		os.Exit(1)
	}
	fmt.Printf("PASS %s\n", name)
}

func assertInt(name string, got int, want int) {
	if got != want {
		fmt.Fprintf(os.Stderr, "FAIL %s: got %d want %d\n", name, got, want)
		os.Exit(1)
	}
	fmt.Printf("PASS %s\n", name)
}

func main() {
	// Index
	assertInt("Index found", strings.Index("hello world", "world"), 6)
	assertInt("Index not found", strings.Index("hello", "xyz"), -1)
	assertInt("Index empty", strings.Index("hello", ""), 0)

	// Contains
	assertBool("Contains true", strings.Contains("hello world", "world"), true)
	assertBool("Contains false", strings.Contains("hello", "xyz"), false)

	// HasPrefix / HasSuffix
	assertBool("HasPrefix true", strings.HasPrefix("hello world", "hello"), true)
	assertBool("HasPrefix false", strings.HasPrefix("hello", "world"), false)
	assertBool("HasSuffix true", strings.HasSuffix("hello world", "world"), true)
	assertBool("HasSuffix false", strings.HasSuffix("hello", "world"), false)

	// TrimPrefix / TrimSuffix
	assert("TrimPrefix match", strings.TrimPrefix("hello world", "hello "), "world")
	assert("TrimPrefix no match", strings.TrimPrefix("hello", "world"), "hello")
	assert("TrimSuffix match", strings.TrimSuffix("hello world", " world"), "hello")

	// TrimSpace
	assert("TrimSpace", strings.TrimSpace("  hello  "), "hello")
	assert("TrimSpace tabs", strings.TrimSpace("\thello\n"), "hello")

	// TrimRight
	assert("TrimRight", strings.TrimRight("hello  ", " "), "hello")

	// Split
	parts := strings.Split("a,b,c", ",")
	assertInt("Split len", len(parts), 3)
	assert("Split[0]", parts[0], "a")
	assert("Split[1]", parts[1], "b")
	assert("Split[2]", parts[2], "c")

	// SplitN
	parts2 := strings.SplitN("a,b,c,d", ",", 3)
	assertInt("SplitN len", len(parts2), 3)
	assert("SplitN[2]", parts2[2], "c,d")

	// Join
	assert("Join", strings.Join(parts, "-"), "a-b-c")

	// Count
	assertInt("Count", strings.Count("hello", "l"), 2)

	// Fields
	fields := strings.Fields("  hello   world  ")
	assertInt("Fields len", len(fields), 2)
	assert("Fields[0]", fields[0], "hello")
	assert("Fields[1]", fields[1], "world")

	// Builder
	b := strings.Builder{}
	b.WriteString("hello")
	b.WriteByte(' ')
	b.WriteString("world")
	assert("Builder", b.String(), "hello world")
	assertInt("Builder Len", b.Len(), 11)
	b.Reset()
	assertInt("Builder Reset", b.Len(), 0)

	fmt.Printf("All strings tests passed!\n")
}
