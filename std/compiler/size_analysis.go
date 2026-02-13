//go:build !no_size_analysis

package main

import "os"

// FuncSize records the compiled size of a single function.
type FuncSize struct {
	Name string
	Size int
}

// funcSizes accumulates per-function compiled sizes across all backends.
var funcSizes []FuncSize

// sizeAnalysisPath is set by the -size-analysis flag. Empty means disabled.
var sizeAnalysisPath string

// collectNativeFuncSizes computes per-function sizes from funcOffsets for native backends.
// codeLen is the total length of the code buffer after all functions are compiled.
func collectNativeFuncSizes(irmod *IRModule, funcOffsets map[string]int, codeLen int) {
	if sizeAnalysisPath == "" {
		return
	}
	// Build ordered list matching irmod.Funcs
	for i, f := range irmod.Funcs {
		offset, ok := funcOffsets[f.Name]
		if !ok {
			continue
		}
		var size int
		if i+1 < len(irmod.Funcs) {
			nextOffset, ok2 := funcOffsets[irmod.Funcs[i+1].Name]
			if ok2 {
				size = nextOffset - offset
			} else {
				size = codeLen - offset
			}
		} else {
			size = codeLen - offset
		}
		funcSizes = append(funcSizes, FuncSize{Name: f.Name, Size: size})
	}
}

// writeSizeAnalysis writes the size analysis JSON to sizeAnalysisPath.
func writeSizeAnalysis() {
	if sizeAnalysisPath == "" {
		return
	}

	// Compute total
	total := 0
	for _, fs := range funcSizes {
		total = total + fs.Size
	}

	// Build JSON manually (no encoding/json dependency)
	buf := make([]byte, 0, 4096)
	buf = append(buf, '{')

	// "target"
	buf = append(buf, '"', 't', 'a', 'r', 'g', 'e', 't', '"', ':')
	buf = appendJSONString(buf, targetGOOS+"/"+targetGOARCH)
	buf = append(buf, ',')

	// "total"
	buf = append(buf, '"', 't', 'o', 't', 'a', 'l', '"', ':')
	buf = appendInt(buf, total)
	buf = append(buf, ',')

	// "functions"
	buf = append(buf, '"', 'f', 'u', 'n', 'c', 't', 'i', 'o', 'n', 's', '"', ':', '[')
	for i, fs := range funcSizes {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = append(buf, '{')

		// "name"
		buf = append(buf, '"', 'n', 'a', 'm', 'e', '"', ':')
		buf = appendJSONString(buf, fs.Name)
		buf = append(buf, ',')

		// "pkg"
		buf = append(buf, '"', 'p', 'k', 'g', '"', ':')
		pkg := extractPackage(fs.Name)
		buf = appendJSONString(buf, pkg)
		buf = append(buf, ',')

		// "size"
		buf = append(buf, '"', 's', 'i', 'z', 'e', '"', ':')
		buf = appendInt(buf, fs.Size)

		buf = append(buf, '}')
	}
	buf = append(buf, ']', '}', '\n')

	os.WriteFile(sizeAnalysisPath, buf, 0644)
}

// extractPackage returns the package portion of a qualified function name.
// e.g. "fmt.Println" → "fmt", "main.main" → "main"
func extractPackage(name string) string {
	for i := 0; i < len(name); i++ {
		if name[i] == '.' {
			return name[0:i]
		}
	}
	return name
}

// appendJSONString appends a JSON-encoded string (with quotes) to buf.
func appendJSONString(buf []byte, s string) []byte {
	buf = append(buf, '"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' {
			buf = append(buf, '\\', '"')
		} else if c == '\\' {
			buf = append(buf, '\\', '\\')
		} else if c < 32 {
			buf = append(buf, '\\', 'u', '0', '0')
			buf = append(buf, hexDigit(c>>4), hexDigit(c&0xf))
		} else {
			buf = append(buf, c)
		}
	}
	buf = append(buf, '"')
	return buf
}

// appendInt appends the decimal representation of n to buf.
func appendInt(buf []byte, n int) []byte {
	if n == 0 {
		return append(buf, '0')
	}
	if n < 0 {
		buf = append(buf, '-')
		n = -n
	}
	// Write digits in reverse, then flip
	start := len(buf)
	for n > 0 {
		buf = append(buf, byte('0'+n%10))
		n = n / 10
	}
	// Reverse the digits
	lo := start
	hi := len(buf) - 1
	for lo < hi {
		tmp := buf[lo]
		buf[lo] = buf[hi]
		buf[hi] = tmp
		lo = lo + 1
		hi = hi - 1
	}
	return buf
}
