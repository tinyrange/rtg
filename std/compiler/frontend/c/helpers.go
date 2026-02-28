package frontend

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func digitValue(ch byte) int {
	if ch >= '0' && ch <= '9' {
		return int(ch - '0')
	}
	if ch >= 'a' && ch <= 'f' {
		return int(ch-'a') + 10
	}
	if ch >= 'A' && ch <= 'F' {
		return int(ch-'A') + 10
	}
	return -1
}

func parseUintBase(text string, base int, bitSize int) (uint64, error) {
	if base < 2 || base > 16 {
		return 0, fmt.Errorf("invalid base %d", base)
	}
	if text == "" {
		return 0, fmt.Errorf("empty numeric text")
	}

	max := ^uint64(0)
	if bitSize > 0 && bitSize < 64 {
		max = (uint64(1) << uint(bitSize)) - 1
	}

	var out uint64
	mul := uint64(base)
	for i := 0; i < len(text); i++ {
		d := digitValue(text[i])
		if d < 0 || d >= base {
			return 0, fmt.Errorf("invalid digit %q for base %d", text[i], base)
		}
		ud := uint64(d)
		if out > (max-ud)/mul {
			return 0, fmt.Errorf("value out of range")
		}
		out = out*mul + ud
	}
	return out, nil
}

func decimalItoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := false
	if v < 0 {
		neg = true
		v = -v
	}
	var b [32]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + (v % 10))
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func quoteTokenText(s string) string {
	b := &strings.Builder{}
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\':
			b.WriteString("\\\\")
		case '"':
			b.WriteString("\\\"")
		case '\n':
			b.WriteString("\\n")
		case '\r':
			b.WriteString("\\r")
		case '\t':
			b.WriteString("\\t")
		default:
			if c < 32 || c > 126 {
				hex := "0123456789abcdef"
				b.WriteString("\\x")
				b.WriteByte(hex[(c>>4)&0xf])
				b.WriteByte(hex[c&0xf])
			} else {
				b.WriteByte(c)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

func unquoteCString(q string) (string, error) {
	if len(q) < 2 || q[0] != '"' || q[len(q)-1] != '"' {
		return "", fmt.Errorf("invalid quoted string %q", q)
	}

	var b strings.Builder
	i := 1
	for i < len(q)-1 {
		ch := q[i]
		if ch != '\\' {
			b.WriteByte(ch)
			i++
			continue
		}
		i++
		if i >= len(q)-1 {
			return "", fmt.Errorf("unterminated escape in %q", q)
		}
		esc := q[i]
		i++
		switch esc {
		case 'a':
			b.WriteByte(7)
		case 'b':
			b.WriteByte(8)
		case 'f':
			b.WriteByte(12)
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'v':
			b.WriteByte(11)
		case '\\':
			b.WriteByte('\\')
		case '\'':
			b.WriteByte('\'')
		case '"':
			b.WriteByte('"')
		case '?':
			b.WriteByte('?')
		case 'x':
			start := i
			for i < len(q)-1 && digitValue(q[i]) >= 0 {
				i++
			}
			if start == i {
				return "", fmt.Errorf("invalid hex escape in %q", q)
			}
			u, err := parseUintBase(q[start:i], 16, 8)
			if err != nil {
				return "", err
			}
			b.WriteByte(byte(u))
		default:
			if esc >= '0' && esc <= '7' {
				start := i - 1
				for i < len(q)-1 && i-start < 3 && q[i] >= '0' && q[i] <= '7' {
					i++
				}
				u, err := parseUintBase(q[start:i], 8, 8)
				if err != nil {
					return "", err
				}
				b.WriteByte(byte(u))
				continue
			}
			b.WriteByte(esc)
		}
	}

	return b.String(), nil
}

func isAbsPath(path string) bool {
	if path == "" {
		return false
	}
	if path[0] == '/' {
		return true
	}
	if len(path) >= 3 && path[1] == ':' && (path[2] == '/' || path[2] == '\\') {
		return true
	}
	return false
}

func cleanPath(path string) string {
	if path == "" {
		return "."
	}

	drive := ""
	rest := path
	if len(rest) >= 2 && rest[1] == ':' {
		drive = rest[:2]
		rest = rest[2:]
	}

	abs := len(rest) > 0 && (rest[0] == '/' || rest[0] == '\\')
	if abs {
		for len(rest) > 0 && (rest[0] == '/' || rest[0] == '\\') {
			rest = rest[1:]
		}
	}

	parts := strings.Split(rest, "/")
	stack := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" || p == "." {
			continue
		}
		if p == ".." {
			if len(stack) > 0 && stack[len(stack)-1] != ".." {
				stack = stack[:len(stack)-1]
				continue
			}
			if !abs {
				stack = append(stack, p)
			}
			continue
		}
		stack = append(stack, p)
	}

	body := strings.Join(stack, "/")
	if abs {
		if body == "" {
			body = "/"
		} else {
			body = "/" + body
		}
	}

	if drive != "" {
		if body == "" {
			return drive + "."
		}
		return drive + body
	}
	if body == "" {
		return "."
	}
	return body
}

func absPath(path string) (string, error) {
	if isAbsPath(path) {
		return cleanPath(path), nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return cleanPath(filepath.Join(cwd, path)), nil
}
