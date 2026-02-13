package strings

import "runtime"

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

func containsByte(s string, b byte) bool {
	i := 0
	for i < len(s) {
		if s[i] == b {
			return true
		}
		i++
	}
	return false
}

func Index(s string, substr string) int {
	slen := len(s)
	sublen := len(substr)
	if sublen == 0 {
		return 0
	}
	if sublen > slen {
		return -1
	}
	i := 0
	for i <= slen-sublen {
		match := true
		j := 0
		for j < sublen {
			if s[i+j] != substr[j] {
				match = false
				break
			}
			j++
		}
		if match {
			return i
		}
		i++
	}
	return -1
}

func Contains(s string, substr string) bool {
	return Index(s, substr) >= 0
}

func HasPrefix(s string, prefix string) bool {
	if len(prefix) > len(s) {
		return false
	}
	return s[0:len(prefix)] == prefix
}

func HasSuffix(s string, suffix string) bool {
	if len(suffix) > len(s) {
		return false
	}
	return s[len(s)-len(suffix):len(s)] == suffix
}

func TrimPrefix(s string, prefix string) string {
	if HasPrefix(s, prefix) {
		return s[len(prefix):len(s)]
	}
	return s
}

func TrimSuffix(s string, suffix string) string {
	if HasSuffix(s, suffix) {
		return s[0 : len(s)-len(suffix)]
	}
	return s
}

func TrimSpace(s string) string {
	start := 0
	for start < len(s) && isSpace(s[start]) {
		start++
	}
	end := len(s)
	for end > start && isSpace(s[end-1]) {
		end = end - 1
	}
	if start == 0 && end == len(s) {
		return s
	}
	return s[start:end]
}

func TrimRight(s string, cutset string) string {
	end := len(s)
	for end > 0 && containsByte(cutset, s[end-1]) {
		end = end - 1
	}
	if end == len(s) {
		return s
	}
	return s[0:end]
}

func SplitN(s string, sep string, n int) []string {
	if n == 0 {
		return nil
	}
	if sep == "" {
		// Split into individual bytes
		var result []string
		i := 0
		for i < len(s) {
			if n > 0 && len(result) >= n-1 {
				result = append(result, s[i:len(s)])
				return result
			}
			result = append(result, runtime.ByteToString(s[i]))
			i++
		}
		return result
	}
	var result []string
	start := 0
	for {
		if n > 0 && len(result) >= n-1 {
			result = append(result, s[start:len(s)])
			return result
		}
		idx := Index(s[start:len(s)], sep)
		if idx < 0 {
			result = append(result, s[start:len(s)])
			return result
		}
		result = append(result, s[start:start+idx])
		start = start + idx + len(sep)
	}
}

func Split(s string, sep string) []string {
	return SplitN(s, sep, -1)
}

func Join(elems []string, sep string) string {
	if len(elems) == 0 {
		return ""
	}
	if len(elems) == 1 {
		return elems[0]
	}
	// Calculate total length
	total := 0
	i := 0
	for i < len(elems) {
		total = total + len(elems[i])
		i++
	}
	total = total + len(sep)*(len(elems)-1)

	buf := make([]byte, total)
	pos := 0
	i = 0
	for i < len(elems) {
		if i > 0 {
			j := 0
			for j < len(sep) {
				buf[pos] = sep[j]
				pos++
				j++
			}
		}
		j := 0
		for j < len(elems[i]) {
			buf[pos] = elems[i][j]
			pos++
			j++
		}
		i++
	}
	return string(buf)
}

func Count(s string, substr string) int {
	if len(substr) == 0 {
		return len(s) + 1
	}
	count := 0
	start := 0
	for {
		idx := Index(s[start:len(s)], substr)
		if idx < 0 {
			return count
		}
		count++
		start = start + idx + len(substr)
	}
}

func Fields(s string) []string {
	var result []string
	i := 0
	for i < len(s) {
		// Skip whitespace
		for i < len(s) && isSpace(s[i]) {
			i++
		}
		if i >= len(s) {
			break
		}
		// Find end of field
		start := i
		for i < len(s) && !isSpace(s[i]) {
			i++
		}
		result = append(result, s[start:i])
	}
	return result
}

// Builder is a simple string builder backed by a byte slice.
type Builder struct {
	buf []byte
}

func (b *Builder) WriteByte(c byte) {
	b.buf = append(b.buf, c)
}

func (b *Builder) WriteRune(r rune) {
	b.buf = append(b.buf, byte(r))
}

func (b *Builder) WriteString(s string) {
	b.buf = append(b.buf, []byte(s)...)
}

func (b *Builder) Len() int {
	return len(b.buf)
}

func (b *Builder) String() string {
	return string(b.buf)
}

func (b *Builder) Reset() {
	b.buf = nil
}
