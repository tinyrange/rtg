//go:build c

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

func buildCmdString(args []string) []byte {
	var cmd []byte
	i := 0
	for i < len(args) {
		if i > 0 {
			cmd = append(cmd, ' ')
		}
		arg := args[i]
		needsQuote := false
		j := 0
		for j < len(arg) {
			if arg[j] == ' ' || arg[j] == '\t' || arg[j] == '"' {
				needsQuote = true
			}
			j++
		}
		if needsQuote {
			cmd = append(cmd, '"')
			j = 0
			for j < len(arg) {
				if arg[j] == '"' {
					cmd = append(cmd, '\\')
				}
				cmd = append(cmd, arg[j])
				j++
			}
			cmd = append(cmd, '"')
		} else {
			j = 0
			for j < len(arg) {
				cmd = append(cmd, arg[j])
				j++
			}
		}
		i++
	}
	cmd = append(cmd, 0)
	return cmd
}

func (c *Cmd) Run() error {
	args := c.Args
	if len(args) == 0 {
		args = make([]string, 1)
		args[0] = c.Path
	}
	cmd := buildCmdString(args)
	rv, _, errn := runtime.SysSystem(runtime.Sliceptr(cmd))
	if errn != 0 {
		return os.Errno(errn)
	}
	exitCode := int(rv)
	if exitCode != 0 {
		return &ExitError{code: exitCode}
	}
	return nil
}

func (c *Cmd) Output() ([]byte, error) {
	args := c.Args
	if len(args) == 0 {
		args = make([]string, 1)
		args[0] = c.Path
	}
	cmd := buildCmdString(args)
	fd, _, errn := runtime.SysPopen(runtime.Sliceptr(cmd))
	if errn != 0 {
		return nil, os.Errno(errn)
	}

	var data []byte
	chunk := make([]byte, 4096)
	for {
		n, _, errn := runtime.SysRead(fd, runtime.Sliceptr(chunk), 4096)
		if errn != 0 || n == 0 {
			break
		}
		data = append(data, chunk[0:int(n)]...)
	}

	rv, _, _ := runtime.SysPclose(fd)
	exitCode := int(rv)
	if exitCode != 0 {
		return data, &ExitError{code: exitCode}
	}
	return data, nil
}
