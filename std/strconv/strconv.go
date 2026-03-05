package strconv

import (
	"runtime"
)

type strconvError string

func (e strconvError) Error() string {
	return string(e)
}

var ErrSyntax error = strconvError("invalid syntax")
var ErrRange error = strconvError("value out of range")

type NumError struct {
	Func string
	Num  string
	Err  error
}

func (e *NumError) Error() string {
	if e == nil {
		return ""
	}
	msg := e.Func + ": parsing " + `"` + e.Num + `"`
	if e.Err != nil {
		msg = msg + ": " + runtime.Tostring(e.Err)
	}
	return msg
}

func digitValue(ch byte) int {
	if ch >= '0' && ch <= '9' {
		return int(ch - '0')
	}
	if ch >= 'a' && ch <= 'z' {
		return int(ch-'a') + 10
	}
	if ch >= 'A' && ch <= 'Z' {
		return int(ch-'A') + 10
	}
	return -1
}

func parseBasePrefix(s string, base int) (int, int) {
	start := 0
	if base != 0 {
		return start, base
	}
	base = 10
	if len(s) >= 2 && s[0] == '0' {
		if s[1] == 'x' || s[1] == 'X' {
			return 2, 16
		}
		if s[1] == 'b' || s[1] == 'B' {
			return 2, 2
		}
		base = 8
		start = 1
	}
	return start, base
}

func ParseInt(s string, base int, bitSize int) (int64, error) {
	if len(s) == 0 {
		return 0, &NumError{Func: "ParseInt", Num: s, Err: ErrSyntax}
	}
	if bitSize < 0 || bitSize > 64 {
		return 0, &NumError{Func: "ParseInt", Num: s, Err: ErrRange}
	}
	neg := false
	if s[0] == '+' || s[0] == '-' {
		neg = s[0] == '-'
		s = s[1:]
	}
	if len(s) == 0 {
		return 0, &NumError{Func: "ParseInt", Num: s, Err: ErrSyntax}
	}
	start, b := parseBasePrefix(s, base)
	if b < 2 || b > 36 {
		return 0, &NumError{Func: "ParseInt", Num: s, Err: ErrSyntax}
	}
	if start >= len(s) {
		return 0, &NumError{Func: "ParseInt", Num: s, Err: ErrSyntax}
	}
	var n int64
	i := start
	for i < len(s) {
		d := digitValue(s[i])
		if d < 0 || d >= b {
			return 0, &NumError{Func: "ParseInt", Num: s, Err: ErrSyntax}
		}
		n = n*int64(b) + int64(d)
		i++
	}
	if neg {
		n = -n
	}
	if bitSize > 0 && bitSize < 64 {
		max := (int64(1) << uint(bitSize-1)) - 1
		min := -int64(1) << uint(bitSize-1)
		if n < min || n > max {
			return 0, &NumError{Func: "ParseInt", Num: s, Err: ErrRange}
		}
	}
	return n, nil
}

func Atoi(s string) (int, error) {
	n, err := ParseInt(s, 10, 0)
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

func Itoa(i int) string {
	return runtime.IntToString(i)
}

func FormatInt(i int64, base int) string {
	if base < 2 || base > 36 {
		base = 10
	}
	if i == 0 {
		return "0"
	}
	neg := i < 0
	var u uint64
	if neg {
		// Use unsigned subtraction to avoid any signed overflow edge cases.
		u = uint64(0) - uint64(i)
	} else {
		u = uint64(i)
	}
	digits := "0123456789abcdefghijklmnopqrstuvwxyz"
	buf := make([]byte, 65)
	pos := len(buf)
	b := uint64(base)
	for u > 0 {
		pos--
		buf[pos] = digits[int(u%b)]
		u = u / b
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
