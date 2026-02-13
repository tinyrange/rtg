//go:build windows

package exec

import (
	"os"
	"runtime"
)

func makeCString(s string) uintptr {
	n := len(s)
	ptr := runtime.Alloc(n + 1)
	if n > 0 {
		runtime.Memcopy(ptr, runtime.Stringptr(s), n)
	}
	runtime.WriteByte(ptr+uintptr(n), 0)
	return ptr
}

func buildCommandLine(args []string) uintptr {
	// Build a single command line string with space-separated, quoted args
	var cmdLine []byte
	i := 0
	for i < len(args) {
		if i > 0 {
			cmdLine = append(cmdLine, ' ')
		}
		arg := args[i]
		// Check if quoting is needed
		needsQuote := false
		j := 0
		for j < len(arg) {
			if arg[j] == ' ' || arg[j] == '\t' || arg[j] == '"' {
				needsQuote = true
			}
			j++
		}
		if needsQuote {
			cmdLine = append(cmdLine, '"')
			j = 0
			for j < len(arg) {
				if arg[j] == '"' {
					cmdLine = append(cmdLine, '\\')
				}
				cmdLine = append(cmdLine, arg[j])
				j++
			}
			cmdLine = append(cmdLine, '"')
		} else {
			j = 0
			for j < len(arg) {
				cmdLine = append(cmdLine, arg[j])
				j++
			}
		}
		i++
	}

	ptr := runtime.Alloc(len(cmdLine) + 1)
	if len(cmdLine) > 0 {
		runtime.Memcopy(ptr, runtime.Sliceptr(cmdLine), len(cmdLine))
	}
	runtime.WriteByte(ptr+uintptr(len(cmdLine)), 0)
	return ptr
}

func buildEnvBlock(env []string) uintptr {
	if len(env) == 0 {
		return 0
	}
	// Environment block: KEY=VALUE\0KEY=VALUE\0\0
	totalLen := 0
	i := 0
	for i < len(env) {
		totalLen = totalLen + len(env[i]) + 1
		i++
	}
	totalLen = totalLen + 1 // final null

	ptr := runtime.Alloc(totalLen)
	offset := 0
	i = 0
	for i < len(env) {
		s := env[i]
		if len(s) > 0 {
			runtime.Memcopy(ptr+uintptr(offset), runtime.Stringptr(s), len(s))
		}
		offset = offset + len(s)
		runtime.WriteByte(ptr+uintptr(offset), 0)
		offset = offset + 1
		i++
	}
	runtime.WriteByte(ptr+uintptr(offset), 0) // final null
	return ptr
}

func (c *Cmd) Run() error {
	args := c.Args
	if len(args) == 0 {
		args = make([]string, 1)
		args[0] = c.Path
	}
	cmdLinePtr := buildCommandLine(args)

	var envPtr uintptr
	if len(c.Env) > 0 {
		envPtr = buildEnvBlock(c.Env)
	} else {
		defaultEnv := os.Environ()
		if len(defaultEnv) > 0 {
			envPtr = buildEnvBlock(defaultEnv)
		}
	}

	// STARTUPINFOA: 68 bytes, all zeros except cb=68
	siSize := 68
	si := runtime.Alloc(siSize)
	runtime.Memzero(si, siSize)
	runtime.WritePtr(si, uintptr(siSize)) // cb = 68

	// Set up stdio redirection if needed
	if c.Stdin != nil || c.Stdout != nil || c.Stderr != nil {
		// dwFlags = STARTF_USESTDHANDLES (0x100)
		runtime.WritePtr(si+44, 0x100)

		// Get default handles for any not overridden
		var stdinH uintptr
		var stdoutH uintptr
		var stderrH uintptr

		if c.Stdin != nil {
			stdinH = uintptr(c.Stdin.Fd())
		} else {
			stdinH = uintptr(os.Stdin.Fd())
		}
		if c.Stdout != nil {
			stdoutH = uintptr(c.Stdout.Fd())
		} else {
			stdoutH = uintptr(os.Stdout.Fd())
		}
		if c.Stderr != nil {
			stderrH = uintptr(c.Stderr.Fd())
		} else {
			stderrH = uintptr(os.Stderr.Fd())
		}

		// hStdInput at offset 56, hStdOutput at 60, hStdError at 64
		runtime.WritePtr(si+56, stdinH)
		runtime.WritePtr(si+60, stdoutH)
		runtime.WritePtr(si+64, stderrH)
	}

	// PROCESS_INFORMATION: 16 bytes
	piSize := 16
	pi := runtime.Alloc(piSize)
	runtime.Memzero(pi, piSize)

	// CreateProcessA(NULL, cmdLine, NULL, NULL, TRUE, 0, envp, NULL, &si, &pi)
	// a0=NULL (appName), a1=cmdLine, a2=startupInfo, a3=processInfo, a4=envp
	_, _, errn := runtime.SysCreateProcess(0, cmdLinePtr, si, pi, envPtr)
	if errn != 0 {
		return os.Errno(errn)
	}

	// Read hProcess from PROCESS_INFORMATION (offset 0)
	hProcess := runtime.ReadPtr(pi)

	// WaitForSingleObject + GetExitCodeProcess
	// a0=hProcess, a1=exitCodeBuf
	exitCodeBuf := runtime.Alloc(4)
	runtime.Memzero(exitCodeBuf, 4)
	exitCode, _, _ := runtime.SysWaitProcess(hProcess, exitCodeBuf)

	// Close process and thread handles
	hThread := runtime.ReadPtr(pi + 4)
	runtime.SysClose(hThread)
	runtime.SysClose(hProcess)

	if int(exitCode) != 0 {
		return &ExitError{code: int(exitCode)}
	}
	return nil
}

func (c *Cmd) Output() ([]byte, error) {
	// Create pipe
	readBuf := runtime.Alloc(4)
	writeBuf := runtime.Alloc(4)
	runtime.Memzero(readBuf, 4)
	runtime.Memzero(writeBuf, 4)

	_, _, errn := runtime.SysCreatePipe(readBuf, writeBuf)
	if errn != 0 {
		return nil, os.Errno(errn)
	}

	readHandle := int(runtime.ReadPtr(readBuf))
	writeHandle := int(runtime.ReadPtr(writeBuf))

	readFile := os.NewFile(readHandle)
	writeFile := os.NewFile(writeHandle)

	c.Stdout = writeFile

	args := c.Args
	if len(args) == 0 {
		args = make([]string, 1)
		args[0] = c.Path
	}
	cmdLinePtr := buildCommandLine(args)

	var envPtr uintptr
	if len(c.Env) > 0 {
		envPtr = buildEnvBlock(c.Env)
	} else {
		defaultEnv := os.Environ()
		if len(defaultEnv) > 0 {
			envPtr = buildEnvBlock(defaultEnv)
		}
	}

	siSize := 68
	si := runtime.Alloc(siSize)
	runtime.Memzero(si, siSize)
	runtime.WritePtr(si, uintptr(siSize))

	// STARTF_USESTDHANDLES
	runtime.WritePtr(si+44, 0x100)
	runtime.WritePtr(si+56, uintptr(os.Stdin.Fd()))  // hStdInput
	runtime.WritePtr(si+60, uintptr(writeHandle))     // hStdOutput = write end
	runtime.WritePtr(si+64, uintptr(os.Stderr.Fd()))  // hStdError

	piSize := 16
	pi := runtime.Alloc(piSize)
	runtime.Memzero(pi, piSize)

	_, _, errn = runtime.SysCreateProcess(0, cmdLinePtr, si, pi, envPtr)
	if errn != 0 {
		readFile.Close()
		writeFile.Close()
		return nil, os.Errno(errn)
	}

	// Close write end in parent
	writeFile.Close()

	// Read all output from read end
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

	// Wait for process
	hProcess := runtime.ReadPtr(pi)
	exitCodeBuf := runtime.Alloc(4)
	runtime.Memzero(exitCodeBuf, 4)
	exitCode, _, _ := runtime.SysWaitProcess(hProcess, exitCodeBuf)

	hThread := runtime.ReadPtr(pi + 4)
	runtime.SysClose(hThread)
	runtime.SysClose(hProcess)

	if int(exitCode) != 0 {
		return data, &ExitError{code: int(exitCode)}
	}
	return data, nil
}
