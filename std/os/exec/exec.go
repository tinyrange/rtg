//go:build linux

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

func makeCString(s string) uintptr {
	n := len(s)
	ptr := runtime.Alloc(n + 1)
	if n > 0 {
		runtime.Memcopy(ptr, runtime.Stringptr(s), n)
	}
	runtime.WriteByte(ptr+uintptr(n), 0)
	return ptr
}

func makeCStringArray(strs []string) uintptr {
	n := len(strs)
	// Array of pointers + null terminator
	psz := runtime.PtrSize
	arr := runtime.Alloc((n + 1) * psz)
	i := 0
	for i < n {
		cstr := makeCString(strs[i])
		runtime.WritePtr(arr+uintptr(i*psz), cstr)
		i++
	}
	runtime.WritePtr(arr+uintptr(n*psz), 0)
	return arr
}

func getDefaultEnvp() uintptr {
	env := os.Environ()
	if len(env) == 0 {
		// Return array with just null terminator
		arr := runtime.Alloc(runtime.PtrSize)
		runtime.WritePtr(arr, 0)
		return arr
	}
	return makeCStringArray(env)
}

func dup2(f *os.File, newfd int) {
	runtime.Syscall(os.SYS_DUP2, uintptr(f.Fd()), uintptr(newfd), 0, 0, 0, 0)
}

func (c *Cmd) Run() error {
	pathPtr := makeCString(c.Path)

	args := c.Args
	if len(args) == 0 {
		args = make([]string, 1)
		args[0] = c.Path
	}
	argvPtr := makeCStringArray(args)

	var envpPtr uintptr
	if len(c.Env) > 0 {
		envpPtr = makeCStringArray(c.Env)
	} else {
		envpPtr = getDefaultEnvp()
	}

	pid, _, errn := runtime.Syscall(os.SYS_FORK, 0, 0, 0, 0, 0, 0)
	if errn != 0 {
		return os.Errno(errn)
	}

	if pid == 0 {
		// Child process
		if c.Stdin != nil {
			dup2(c.Stdin, 0)
		}
		if c.Stdout != nil {
			dup2(c.Stdout, 1)
		}
		if c.Stderr != nil {
			dup2(c.Stderr, 2)
		}

		runtime.Syscall(os.SYS_EXECVE, pathPtr, argvPtr, envpPtr, 0, 0, 0)
		// If execve returns, it failed
		os.Exit(127)
	}

	// Parent: wait for child
	statusBuf := make([]byte, 8)
	runtime.Memzero(runtime.Sliceptr(statusBuf), 8)
	_, _, errn = runtime.Syscall(os.SYS_WAIT4, pid, runtime.Sliceptr(statusBuf), 0, 0, 0, 0)
	if errn != 0 {
		return os.Errno(errn)
	}

	// Read status (little-endian int32)
	status := int(statusBuf[0]) + int(statusBuf[1])*256 + int(statusBuf[2])*65536 + int(statusBuf[3])*16777216
	exitCode := (status >> 8) & 0xff

	if exitCode != 0 {
		return &ExitError{code: exitCode}
	}
	return nil
}

func (c *Cmd) Output() ([]byte, error) {
	// Create pipe
	pipeBuf := make([]byte, 8)
	_, _, errn := runtime.Syscall(os.SYS_PIPE2, runtime.Sliceptr(pipeBuf), 0, 0, 0, 0, 0)
	if errn != 0 {
		return nil, os.Errno(errn)
	}

	// Read pipe fds (two int32s, little-endian)
	readFd := int(pipeBuf[0]) + int(pipeBuf[1])*256 + int(pipeBuf[2])*65536 + int(pipeBuf[3])*16777216
	writeFd := int(pipeBuf[4]) + int(pipeBuf[5])*256 + int(pipeBuf[6])*65536 + int(pipeBuf[7])*16777216

	readFile := os.NewFile(readFd)
	writeFile := os.NewFile(writeFd)

	c.Stdout = writeFile

	pathPtr := makeCString(c.Path)

	args := c.Args
	if len(args) == 0 {
		args = make([]string, 1)
		args[0] = c.Path
	}
	argvPtr := makeCStringArray(args)

	var envpPtr uintptr
	if len(c.Env) > 0 {
		envpPtr = makeCStringArray(c.Env)
	} else {
		envpPtr = getDefaultEnvp()
	}

	pid, _, errn := runtime.Syscall(os.SYS_FORK, 0, 0, 0, 0, 0, 0)
	if errn != 0 {
		readFile.Close()
		writeFile.Close()
		return nil, os.Errno(errn)
	}

	if pid == 0 {
		// Child process
		readFile.Close()
		if c.Stdin != nil {
			dup2(c.Stdin, 0)
		}
		dup2(writeFile, 1)
		if c.Stderr != nil {
			dup2(c.Stderr, 2)
		}
		runtime.Syscall(os.SYS_EXECVE, pathPtr, argvPtr, envpPtr, 0, 0, 0)
		os.Exit(127)
	}

	// Parent: close write end, read all from read end
	writeFile.Close()

	var data []byte
	chunk := make([]byte, 4096)
	for {
		n, _ := readFile.Read(chunk)
		if n > 0 {
			data = append(data, chunk[0:n]...)
		}
		if n == 0 {
			break
		}
	}
	readFile.Close()

	// Wait for child
	statusBuf := make([]byte, 8)
	runtime.Memzero(runtime.Sliceptr(statusBuf), 8)
	_, _, errn = runtime.Syscall(os.SYS_WAIT4, pid, runtime.Sliceptr(statusBuf), 0, 0, 0, 0)
	if errn != 0 {
		return data, os.Errno(errn)
	}

	status := int(statusBuf[0]) + int(statusBuf[1])*256 + int(statusBuf[2])*65536 + int(statusBuf[3])*16777216
	exitCode := (status >> 8) & 0xff

	if exitCode != 0 {
		return data, &ExitError{code: exitCode}
	}
	return data, nil
}
