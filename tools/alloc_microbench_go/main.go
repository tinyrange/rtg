package main

import (
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"time"
	"unsafe"
)

type benchCase struct {
	name    string
	n       int
	details string
}

var sinkU64 uint64
var sinkInt int
var sinkStr string
var sinkBytes []byte

func benchAllocFixed(size int, n int) uint64 {
	var acc uint64
	i := 0
	for i < n {
		b := make([]byte, size)
		if size > 0 {
			b[0] = byte(i)
			acc = acc ^ uint64(uintptr(unsafe.Pointer(&b[0])))
		}
		sinkBytes = b
		i++
	}
	return acc
}

func benchAllocMixed(n int) uint64 {
	sizes := []int{8, 16, 24, 32, 48, 64, 96, 128}
	var acc uint64
	i := 0
	for i < n {
		size := sizes[i&7]
		b := make([]byte, size)
		b[0] = byte(i)
		acc = acc ^ uint64(uintptr(unsafe.Pointer(&b[0])))
		sinkBytes = b
		i++
	}
	return acc
}

func benchSliceAppendByte(n int) uint64 {
	buf := make([]byte, 0)
	i := 0
	for i < n {
		buf = append(buf, byte(i))
		i++
	}
	sinkBytes = buf
	sinkInt = sinkInt + len(buf)
	return uint64(len(buf))
}

func benchSliceAppendWord(n int) uint64 {
	buf := make([]uintptr, 0)
	i := 0
	for i < n {
		buf = append(buf, uintptr(i))
		i++
	}
	sinkInt = sinkInt + len(buf)
	return uint64(len(buf))
}

func benchSliceAppendSlice16(n int) uint64 {
	dst := make([]int, 0)
	src := make([]int, 16)
	i := 0
	for i < len(src) {
		src[i] = i + 1
		i++
	}
	i = 0
	for i < n {
		dst = append(dst, src...)
		i++
	}
	sinkInt = sinkInt + len(dst)
	return uint64(len(dst))
}

func benchMapSetInt(n int) uint64 {
	m := map[int]int{}
	i := 0
	for i < n {
		m[i] = i + 1
		i++
	}
	sinkInt = sinkInt + len(m)
	return uint64(len(m))
}

func benchMapSetString(n int) uint64 {
	m := map[string]int{}
	i := 0
	for i < n {
		k := strconv.Itoa(i)
		m[k] = i
		i++
	}
	sinkInt = sinkInt + len(m)
	return uint64(len(m))
}

func buildStringKeys(n int) []string {
	keys := make([]string, n)
	i := 0
	for i < n {
		keys[i] = strconv.Itoa(i)
		i++
	}
	return keys
}

func benchMapGetIntHot(n int) uint64 {
	const keyCount = 4096
	m := map[int]int{}
	i := 0
	for i < keyCount {
		m[i] = i + 1
		i++
	}
	var acc uint64
	i = 0
	for i < n {
		v, ok := m[i&(keyCount-1)]
		if ok {
			acc = acc + uint64(v)
		}
		i++
	}
	sinkU64 = sinkU64 ^ acc
	return acc
}

func benchMapGetStringHot(n int) uint64 {
	const keyCount = 1024
	keys := buildStringKeys(keyCount)
	m := map[string]int{}
	i := 0
	for i < keyCount {
		m[keys[i]] = i + 1
		i++
	}
	var acc uint64
	i = 0
	for i < n {
		v, ok := m[keys[i&(keyCount-1)]]
		if ok {
			acc = acc + uint64(v)
		}
		i++
	}
	sinkU64 = sinkU64 ^ acc
	return acc
}

func benchMapSetIntUpdateHot(n int) uint64 {
	const keyCount = 2048
	m := map[int]int{}
	i := 0
	for i < keyCount {
		m[i] = i
		i++
	}
	i = 0
	for i < n {
		k := i & (keyCount - 1)
		m[k] = i
		i++
	}
	sinkInt = sinkInt + len(m)
	return uint64(len(m))
}

func benchMapSetStringUpdateHot(n int) uint64 {
	const keyCount = 512
	keys := buildStringKeys(keyCount)
	m := map[string]int{}
	i := 0
	for i < keyCount {
		m[keys[i]] = i
		i++
	}
	i = 0
	for i < n {
		k := keys[i&(keyCount-1)]
		m[k] = i
		i++
	}
	sinkInt = sinkInt + len(m)
	return uint64(len(m))
}

func benchStringConcat(n int) uint64 {
	s := ""
	i := 0
	for i < n {
		s = s + "x"
		if len(s) > 1024 {
			s = s[256:len(s)]
		}
		i++
	}
	sinkStr = s
	return uint64(len(s))
}

func benchStringConcatEmptyMix(n int) uint64 {
	empty := ""
	s := "payload"
	i := 0
	for i < n {
		if (i & 1) == 0 {
			s = empty + s
		} else {
			s = s + empty
		}
		if len(s) > 32 {
			s = s[0:32]
		}
		i++
	}
	sinkStr = s
	return uint64(len(s))
}

func benchStringConcatSmallPairs(n int) uint64 {
	a := "abc"
	b := "def"
	var acc uint64
	i := 0
	for i < n {
		s := a + b
		acc = acc + uint64(len(s))
		a = b
		b = s[0:3]
		i++
	}
	sinkU64 = sinkU64 ^ acc
	return acc
}

func benchCompilerLikeBuffers(n int) uint64 {
	type inst struct {
		op int
		a  int
		b  int
		c  int
	}
	code := make([]inst, 0)
	fixups := make([]int, 0)
	rodata := make([]byte, 0)
	i := 0
	for i < n {
		code = append(code, inst{op: i & 255, a: i, b: i + 1, c: i + 2})
		if (i & 3) == 0 {
			fixups = append(fixups, i)
		}
		v := i * 2654435761
		rodata = append(rodata, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
		i++
	}
	sinkInt = sinkInt + len(code) + len(fixups) + len(rodata)
	return uint64(len(code) + len(fixups) + len(rodata))
}

func cases() []benchCase {
	return []benchCase{
		{
			name:    "alloc_fixed_24",
			n:       1000000,
			details: "Approximate fixed 24-byte allocation loop using make([]byte, 24).",
		},
		{
			name:    "alloc_fixed_256",
			n:       300000,
			details: "Approximate fixed 256-byte allocation loop using make([]byte, 256).",
		},
		{
			name:    "alloc_mixed_small",
			n:       500000,
			details: "Approximate mixed small-size allocation loop using make([]byte, size).",
		},
		{
			name:    "alloc_large_2mb",
			n:       96,
			details: "Approximate large allocation loop using make([]byte, 2MB).",
		},
		{
			name:    "slice_append_byte",
			n:       1000000,
			details: "append(byte) growth behavior from nil slice.",
		},
		{
			name:    "slice_append_word",
			n:       400000,
			details: "append(uintptr) growth behavior from nil slice.",
		},
		{
			name:    "slice_append_slice16",
			n:       100000,
			details: "append(dst, src...) bulk growth behavior.",
		},
		{
			name:    "map_set_int",
			n:       10000,
			details: "Map growth behavior with int keys.",
		},
		{
			name:    "map_set_string",
			n:       5000,
			details: "Map growth behavior with string keys.",
		},
		{
			name:    "map_get_int_hot",
			n:       400000,
			details: "Hot-path lookups over a fixed int-key set.",
		},
		{
			name:    "map_get_string_hot",
			n:       150000,
			details: "Hot-path lookups over a fixed string-key set.",
		},
		{
			name:    "map_set_int_update_hot",
			n:       300000,
			details: "Map updates on existing int keys.",
		},
		{
			name:    "map_set_string_update_hot",
			n:       120000,
			details: "Map updates on existing string keys.",
		},
		{
			name:    "string_concat_chain",
			n:       50000,
			details: "Repeated string concatenation growth.",
		},
		{
			name:    "string_concat_empty_mix",
			n:       600000,
			details: "Alternating empty-left/empty-right concat forms.",
		},
		{
			name:    "string_concat_small_pairs",
			n:       300000,
			details: "Repeated small-string concatenations.",
		},
		{
			name:    "compiler_like_buffers",
			n:       100000,
			details: "Codegen-like append growth across code/fixup/rodata buffers.",
		},
	}
}

func findCase(name string, all []benchCase) (benchCase, bool) {
	i := 0
	for i < len(all) {
		if all[i].name == name {
			return all[i], true
		}
		i++
	}
	return benchCase{}, false
}

func runCase(name string, n int) (uint64, bool) {
	switch name {
	case "alloc_fixed_24":
		return benchAllocFixed(24, n), true
	case "alloc_fixed_256":
		return benchAllocFixed(256, n), true
	case "alloc_mixed_small":
		return benchAllocMixed(n), true
	case "alloc_large_2mb":
		return benchAllocFixed(2*1024*1024, n), true
	case "slice_append_byte":
		return benchSliceAppendByte(n), true
	case "slice_append_word":
		return benchSliceAppendWord(n), true
	case "slice_append_slice16":
		return benchSliceAppendSlice16(n), true
	case "map_set_int":
		return benchMapSetInt(n), true
	case "map_set_string":
		return benchMapSetString(n), true
	case "map_get_int_hot":
		return benchMapGetIntHot(n), true
	case "map_get_string_hot":
		return benchMapGetStringHot(n), true
	case "map_set_int_update_hot":
		return benchMapSetIntUpdateHot(n), true
	case "map_set_string_update_hot":
		return benchMapSetStringUpdateHot(n), true
	case "string_concat_chain":
		return benchStringConcat(n), true
	case "string_concat_empty_mix":
		return benchStringConcatEmptyMix(n), true
	case "string_concat_small_pairs":
		return benchStringConcatSmallPairs(n), true
	case "compiler_like_buffers":
		return benchCompilerLikeBuffers(n), true
	default:
		return 0, false
	}
}

func printUsage(program string, all []benchCase) {
	fmt.Fprintf(os.Stderr, "usage: %s -case <name> [-n <iters>] [-list]\n", program)
	fmt.Fprintf(os.Stderr, "\navailable cases:\n")
	i := 0
	for i < len(all) {
		c := all[i]
		fmt.Fprintf(os.Stderr, "  %s (default n=%d): %s\n", c.name, c.n, c.details)
		i++
	}
}

func parseIntArg(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	sign := 1
	i := 0
	if s[0] == '-' {
		sign = -1
		i = 1
	}
	if i >= len(s) {
		return 0, false
	}
	v := 0
	for i < len(s) {
		ch := s[i]
		if ch < '0' || ch > '9' {
			return 0, false
		}
		v = v*10 + int(ch-'0')
		i++
	}
	return sign * v, true
}

func main() {
	runtime.GOMAXPROCS(1)
	all := cases()

	caseName := ""
	overrideN := -1
	list := false

	i := 1
	for i < len(os.Args) {
		arg := os.Args[i]
		if arg == "-list" {
			list = true
			i++
			continue
		}
		if arg == "-case" {
			if i+1 >= len(os.Args) {
				printUsage(os.Args[0], all)
				os.Exit(2)
			}
			caseName = os.Args[i+1]
			i += 2
			continue
		}
		if arg == "-n" {
			if i+1 >= len(os.Args) {
				printUsage(os.Args[0], all)
				os.Exit(2)
			}
			n, ok := parseIntArg(os.Args[i+1])
			if !ok || n <= 0 {
				fmt.Fprintf(os.Stderr, "invalid -n value: %q\n", os.Args[i+1])
				os.Exit(2)
			}
			overrideN = n
			i += 2
			continue
		}
		printUsage(os.Args[0], all)
		os.Exit(2)
	}

	if list {
		j := 0
		for j < len(all) {
			fmt.Println(all[j].name)
			j++
		}
		return
	}

	if caseName == "" {
		printUsage(os.Args[0], all)
		os.Exit(2)
	}

	c, ok := findCase(caseName, all)
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown case %q\n", caseName)
		printUsage(os.Args[0], all)
		os.Exit(2)
	}
	n := c.n
	if overrideN > 0 {
		n = overrideN
	}

	runtime.GC()
	debug.FreeOSMemory()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	start := time.Now()
	checksum, ok := runCase(c.name, n)
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown case %q\n", c.name)
		os.Exit(2)
	}
	elapsed := time.Since(start).Nanoseconds()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	allocCalls := int(after.Mallocs - before.Mallocs)
	if allocCalls <= 0 {
		allocCalls = 1
	}
	reqBytes := int(after.TotalAlloc - before.TotalAlloc)
	if reqBytes < 0 {
		reqBytes = 0
	}
	nsPerOp := elapsed
	if n > 0 {
		nsPerOp = elapsed / int64(n)
	}

	fmt.Printf("case\titerations\tns_total\tns_per_op\talloc_calls\treq_bytes\tavg_req_per_alloc\tmmap_calls\tmmap_bytes\tavg_mmap_per_call\tnext_chunk\tchunk_max\theap_avail\tchecksum\n")
	fmt.Printf("%s\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\n",
		c.name,
		n,
		elapsed,
		nsPerOp,
		allocCalls,
		reqBytes,
		reqBytes/allocCalls,
		0,
		0,
		0,
		0,
		0,
		int(after.HeapAlloc),
		checksum,
	)

	sinkU64 = sinkU64 ^ checksum
}
