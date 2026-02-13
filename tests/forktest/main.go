package main

import (
	"fmt"
	"os"
	"runtime"
)

func makeCStr(s string) uintptr {
	n := len(s)
	ptr := runtime.Alloc(n + 1)
	if n > 0 {
		runtime.Memcopy(ptr, runtime.Stringptr(s), n)
	}
	runtime.WriteByte(ptr+uintptr(n), 0)
	return ptr
}

func main() {
	pathPtr := makeCStr("/bin/echo")
	arg0 := makeCStr("/bin/echo")
	arg1 := makeCStr("hello from exec")

	// argv = [arg0, arg1, NULL]
	argv := runtime.Alloc(3 * 8)
	runtime.WritePtr(argv, arg0)
	runtime.WritePtr(argv+8, arg1)
	runtime.WritePtr(argv+16, 0)

	// envp = [NULL]
	envp := runtime.Alloc(8)
	runtime.WritePtr(envp, 0)

	fmt.Printf("about to fork\n")
	pid, _, errn := runtime.Syscall(57, 0, 0, 0, 0, 0, 0)
	if errn != 0 {
		fmt.Printf("fork failed: %d\n", errn)
		os.Exit(1)
	}

	if pid == 0 {
		// child: execve
		_, _, eerr := runtime.Syscall(59, pathPtr, argv, envp, 0, 0, 0)
		fmt.Printf("execve failed: %d\n", eerr)
		os.Exit(127)
	}

	// parent: wait
	statusBuf := make([]byte, 8)
	runtime.Memzero(runtime.Sliceptr(statusBuf), 8)
	_, _, werr := runtime.Syscall(61, pid, runtime.Sliceptr(statusBuf), 0, 0, 0, 0)
	if werr != 0 {
		fmt.Printf("wait4 failed: %d\n", werr)
		os.Exit(1)
	}
	status := int(statusBuf[0]) + int(statusBuf[1])*256 + int(statusBuf[2])*65536 + int(statusBuf[3])*16777216
	exitCode := (status >> 8) & 0xff
	fmt.Printf("child exited with code %d\n", exitCode)
	fmt.Printf("done\n")
}
