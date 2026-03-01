package log

import (
	"fmt"
	"io"
	"os"
	"runtime"
)

const (
	Ldate         = 1 << 0
	Ltime         = 1 << 1
	Lmicroseconds = 1 << 2
	Llongfile     = 1 << 3
	Lshortfile    = 1 << 4
	LUTC          = 1 << 5
	Lmsgprefix    = 1 << 6
	LstdFlags     = Ldate | Ltime
)

type Logger struct {
	out    io.Writer
	prefix string
	flag   int
}

func New(out io.Writer, prefix string, flag int) *Logger {
	return &Logger{
		out:    out,
		prefix: prefix,
		flag:   flag,
	}
}

func joinArgs(v []interface{}, sep string) string {
	if len(v) == 0 {
		return ""
	}
	var out []byte
	i := 0
	for i < len(v) {
		if i > 0 && len(sep) > 0 {
			out = append(out, []byte(sep)...)
		}
		out = append(out, []byte(runtime.Tostring(v[i]))...)
		i++
	}
	return string(out)
}

func (l *Logger) outputLine(msg string) error {
	if l == nil || l.out == nil {
		return nil
	}
	if len(msg) == 0 || msg[len(msg)-1] != '\n' {
		msg = msg + "\n"
	}
	w := l.out
	_, err := w.Write([]byte(l.prefix + msg))
	return err
}

func (l *Logger) Print(v ...interface{}) {
	_ = l.outputLine(joinArgs(v, ""))
}

func (l *Logger) Printf(format string, v ...interface{}) {
	_ = l.outputLine(fmt.Sprintf(format, v...))
}

func (l *Logger) Println(v ...interface{}) {
	_ = l.outputLine(joinArgs(v, " "))
}

func (l *Logger) Fatal(v ...interface{}) {
	l.Print(v...)
	os.Exit(1)
}

func (l *Logger) Fatalf(format string, v ...interface{}) {
	l.Printf(format, v...)
	os.Exit(1)
}

func (l *Logger) Fatalln(v ...interface{}) {
	l.Println(v...)
	os.Exit(1)
}

func (l *Logger) SetOutput(w io.Writer) {
	l.out = w
}

func (l *Logger) SetPrefix(prefix string) {
	l.prefix = prefix
}

func (l *Logger) Prefix() string {
	return l.prefix
}

func (l *Logger) Flags() int {
	return l.flag
}

func (l *Logger) SetFlags(flag int) {
	l.flag = flag
}

var stdPrefix string
var stdFlag int = LstdFlags

func SetOutput(w io.Writer) {
	_ = w
}

func SetPrefix(prefix string) {
	stdPrefix = prefix
}

func Prefix() string {
	return stdPrefix
}

func SetFlags(flag int) {
	stdFlag = flag
}

func Flags() int {
	return stdFlag
}

func Print(v ...interface{}) {
	_, _ = fmt.Print(stdPrefix + joinArgs(v, "") + "\n")
}

func Printf(format string, v ...interface{}) {
	_, _ = fmt.Printf(stdPrefix+format+"\n", v...)
}

func Println(v ...interface{}) {
	_, _ = fmt.Print(stdPrefix + joinArgs(v, " ") + "\n")
}

func Fatal(v ...interface{}) {
	Print(v...)
	os.Exit(1)
}

func Fatalf(format string, v ...interface{}) {
	Printf(format, v...)
	os.Exit(1)
}

func Fatalln(v ...interface{}) {
	Println(v...)
	os.Exit(1)
}
