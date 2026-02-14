//go:build wasi

package exec

import (
	"os"
	"runtime"
)

type ExitError struct {
	code int
}

func (e *ExitError) Error() string {
	return "exit status " + runtime.IntToString(e.code)
}

type Cmd struct {
	Path   string
	Args   []string
	Env    []string
	Stdin  *os.File
	Stdout *os.File
	Stderr *os.File
}

func Command(name string, arg ...interface{}) *Cmd {
	args := make([]string, 1+len(arg))
	args[0] = name
	i := 0
	for i < len(arg) {
		args[i+1] = runtime.Tostring(arg[i])
		i++
	}
	return &Cmd{
		Path: name,
		Args: args,
	}
}

func (c *Cmd) Run() error {
	return os.Errno(1)
}

func (c *Cmd) Output() ([]byte, error) {
	return nil, os.Errno(1)
}
