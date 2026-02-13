package main

import (
	"fmt"
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

func makeCStringArray(strs []string) uintptr {
	n := len(strs)
	arr := runtime.Alloc((n + 1) * 8)
	i := 0
	for i < n {
		cstr := makeCString(strs[i])
		runtime.WritePtr(arr+uintptr(i*8), cstr)
		i++
	}
	runtime.WritePtr(arr+uintptr(n*8), 0)
	return arr
}

func main() {
	pathPtr := makeCString("/bin/echo")
	var args []string
	args = append(args, "/bin/echo")
	args = append(args, "hello from child")
	argvPtr := makeCStringArray(args)

	env := os.Environ()
	fmt.Printf("env count: %d\n", len(env))
	var envpPtr uintptr
	if len(env) > 0 {
		envpPtr = makeCStringArray(env)
	} else {
		envpPtr = runtime.Alloc(8)
		runtime.WritePtr(envpPtr, 0)
	}

	fmt.Printf("about to fork\n")
	pid, _, errn := runtime.SysFork()
	fmt.Printf("fork returned pid=%d errn=%d\n", pid, errn)

	if errn != 0 {
		fmt.Printf("fork failed\n")
		os.Exit(1)
	}

	if pid == 0 {
		fmt.Printf("child: about to execve\n")
		_, _, eerr := runtime.SysExecve(pathPtr, argvPtr, envpPtr)
		fmt.Printf("execve failed: %d\n", eerr)
		os.Exit(127)
	}

	fmt.Printf("parent: waiting for pid %d\n", pid)
	statusBuf := make([]byte, 8)
	runtime.Memzero(runtime.Sliceptr(statusBuf), 8)
	_, _, werr := runtime.SysWait4(pid, runtime.Sliceptr(statusBuf), 0, 0)
	fmt.Printf("wait4 returned werr=%d\n", werr)

	if werr != 0 {
		fmt.Printf("wait4 failed\n")
		os.Exit(1)
	}

	status := int(statusBuf[0]) + int(statusBuf[1])*256 + int(statusBuf[2])*65536 + int(statusBuf[3])*16777216
	exitCode := (status >> 8) & 0xff
	fmt.Printf("child exit code: %d\n", exitCode)
	fmt.Printf("done\n")
}
