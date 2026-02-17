//go:build dos

package fmt

import (
	"os"
	"runtime"
)

func dosWrite(fd uintptr, s string) (int, error) {
	if len(s) == 0 {
		return 0, nil
	}
	wrote, _, errn := runtime.SysWrite(fd, runtime.Stringptr(s), uintptr(len(s)))
	if errn != 0 {
		return int(wrote), os.Errno(errn)
	}
	return int(wrote), nil
}

func dosWriteRange(fd uintptr, s string, start int, end int) (int, error) {
	if end <= start {
		return 0, nil
	}
	n := end - start
	ptr := runtime.Stringptr(s) + uintptr(start)
	wrote, _, errn := runtime.SysWrite(fd, ptr, uintptr(n))
	if errn != 0 {
		return int(wrote), os.Errno(errn)
	}
	return int(wrote), nil
}

func dosFormatWrite(fd uintptr, format string, a []interface{}) (int, error) {
	total := 0
	argIdx := 0
	litStart := 0

	i := 0
	for i < len(format) {
		if format[i] == '%' && i+1 < len(format) {
			n, err := dosWriteRange(fd, format, litStart, i)
			total += n
			if err != nil {
				return total, err
			}
			i++
			spec := format[i]
			if spec == '%' {
				n, err := dosWrite(fd, "%")
				total += n
				if err != nil {
					return total, err
				}
			} else if spec == 's' || spec == 'd' || spec == 'v' || spec == 'w' {
				if argIdx < len(a) {
					s := runtime.Tostring(a[argIdx])
					argIdx++
					n, err := dosWrite(fd, s)
					total += n
					if err != nil {
						return total, err
					}
				}
			} else if spec == 'q' {
				if argIdx < len(a) {
					s := runtime.Tostring(a[argIdx])
					argIdx++
					n, err := dosWrite(fd, "\"")
					total += n
					if err != nil {
						return total, err
					}
					n, err = dosWrite(fd, s)
					total += n
					if err != nil {
						return total, err
					}
					n, err = dosWrite(fd, "\"")
					total += n
					if err != nil {
						return total, err
					}
				}
			} else {
				n, err := dosWrite(fd, "%")
				total += n
				if err != nil {
					return total, err
				}
				n, err = dosWrite(fd, runtime.ByteToString(spec))
				total += n
				if err != nil {
					return total, err
				}
			}
			litStart = i + 1
		}
		i++
	}
	n, err := dosWriteRange(fd, format, litStart, len(format))
	total += n
	if err != nil {
		return total, err
	}
	return total, nil
}

func Sprintf(format string, a ...interface{}) string {
	var result []byte
	argIdx := 0
	i := 0
	for i < len(format) {
		if format[i] == '%' && i+1 < len(format) {
			i++
			if format[i] == '%' {
				result = append(result, '%')
			} else if format[i] == 's' || format[i] == 'd' || format[i] == 'v' || format[i] == 'w' {
				if argIdx < len(a) {
					s := runtime.Tostring(a[argIdx])
					result = append(result, []byte(s)...)
					argIdx++
				}
			} else if format[i] == 'q' {
				if argIdx < len(a) {
					s := runtime.Tostring(a[argIdx])
					result = append(result, '"')
					result = append(result, []byte(s)...)
					result = append(result, '"')
					argIdx++
				}
			} else {
				result = append(result, '%')
				result = append(result, format[i])
			}
		} else {
			result = append(result, format[i])
		}
		i++
	}
	return string(result)
}

func Fprintf(w *os.File, format string, a ...interface{}) (n int, err error) {
	fd := uintptr(1)
	if w == os.Stderr {
		fd = uintptr(2)
	} else if w != os.Stdout {
		fd = uintptr(w.Fd())
	}
	return dosFormatWrite(fd, format, a)
}

func Printf(format string, a ...interface{}) (n int, err error) {
	return dosFormatWrite(uintptr(1), format, a)
}

func Print(a ...interface{}) (int, error) {
	total := 0
	i := 0
	for i < len(a) {
		s := runtime.Tostring(a[i])
		n, err := dosWrite(1, s)
		total += n
		if err != nil {
			return total, err
		}
		i++
	}
	return total, nil
}

func Println(a ...interface{}) (int, error) {
	total := 0
	i := 0
	for i < len(a) {
		if i > 0 {
			n, err := dosWrite(1, " ")
			total += n
			if err != nil {
				return total, err
			}
		}
		s := runtime.Tostring(a[i])
		n, err := dosWrite(1, s)
		total += n
		if err != nil {
			return total, err
		}
		i++
	}
	n, err := dosWrite(1, "\n")
	total += n
	if err != nil {
		return total, err
	}
	return total, nil
}

type fmtError struct {
	msg string
}

func (e *fmtError) Error() string {
	return e.msg
}

func Errorf(format string, a ...interface{}) error {
	return &fmtError{msg: Sprintf(format, a...)}
}
