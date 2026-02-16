package fmt

import (
	"os"
	"runtime"
)

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
	s := Sprintf(format, a...)
	if runtime.GOOS == "dos" {
		fd := uintptr(1)
		if w == os.Stderr {
			fd = uintptr(2)
		} else if w != os.Stdout {
			fd = uintptr(w.Fd())
		}
		wrote, _, errn := runtime.SysWrite(fd, runtime.Stringptr(s), uintptr(len(s)))
		if errn != 0 {
			return int(wrote), os.Errno(errn)
		}
		return int(wrote), nil
	}
	return os.Write(w, []byte(s))
}

func Printf(format string, a ...interface{}) (n int, err error) {
	if runtime.GOOS == "dos" {
		s := Sprintf(format, a...)
		b := []byte(s)
		wrote, _, errn := runtime.SysWrite(uintptr(1), runtime.Sliceptr(b), uintptr(len(b)))
		if errn != 0 {
			return int(wrote), os.Errno(errn)
		}
		return int(wrote), nil
	}
	return Fprintf(os.Stdout, format, a...)
}

func Println(a ...interface{}) (int, error) {
	var result []byte
	i := 0
	for i < len(a) {
		if i > 0 {
			result = append(result, ' ')
		}
		s := runtime.Tostring(a[i])
		result = append(result, []byte(s)...)
		i++
	}
	result = append(result, '\n')
	if runtime.GOOS == "dos" {
		wrote, _, errn := runtime.SysWrite(uintptr(1), runtime.Sliceptr(result), uintptr(len(result)))
		if errn != 0 {
			return int(wrote), os.Errno(errn)
		}
		return int(wrote), nil
	}
	return os.Write(os.Stdout, result)
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
