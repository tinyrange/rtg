//go:build windows

package runtime

const (
	winGenericRead       = 0x80000000
	winGenericWrite      = 0x40000000
	winGenericReadWrite  = 0xC0000000
	winFileShareRW       = 0x00000003
	winOpenExisting      = 3
	winCreateAlways      = 2
	winFileAttrNormal    = 0x00000080
	winMemCommitReserve  = 0x3000
	winPageReadWrite     = 0x04
	winErrorAlreadyExist = 183
	winErrorNoMoreFiles  = 18
	winInfinite          = 0xFFFFFFFF

	// Matches std/os flag constants used by callers.
	winFlagWriteCreateTrunc = 577
	winFlagRDWR             = 2
)

func winErrnoFromLastError() int32 {
	errv, _, _ := winGetLastError()
	if errv == 0 {
		return 1
	}
	return int32(errv)
}

func winFdToHandle(fd uintptr) uintptr {
	if fd > 2 {
		return fd
	}
	// STD_INPUT_HANDLE=-10, STD_OUTPUT_HANDLE=-11, STD_ERROR_HANDLE=-12.
	std := ^uintptr(9 + fd)
	h, _, _ := winGetStdHandle(std)
	return h
}

//rtg:linkstatic kernel32.dll,GetLastError,rawptr
func winGetLastError() (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,GetStdHandle,rawptr
func winGetStdHandle(stdHandle uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,ReadFile,rawptr
func winReadFile(handle, buf, count, nreadPtr, overlapped uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,WriteFile,rawptr
func winWriteFile(handle, buf, count, nwrittenPtr, overlapped uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,CreateFileA,rawptr
func winCreateFile(path, access, shareMode, securityAttrs, creationDisposition, flagsAndAttrs, templateFile uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,CloseHandle,rawptr
func winCloseHandle(handle uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,GetFileAttributesExA,rawptr
func winGetFileAttributesEx(path, infoLevel, fileInfo uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,ExitProcess,noreturn
func winExitProcess(code uintptr)

//rtg:linkstatic kernel32.dll,VirtualAlloc,rawptr
func winVirtualAlloc(addr, size, allocType, protect uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,CreateDirectoryA,rawptr
func winCreateDirectory(path, securityAttrs uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,RemoveDirectoryA,rawptr
func winRemoveDirectory(path uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,DeleteFileA,rawptr
func winDeleteFile(path uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,GetCurrentDirectoryA,rawptr
func winGetCurrentDirectory(size, buf uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,GetCommandLineA,rawptr
func winGetCommandLine() (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,GetEnvironmentStringsA,rawptr
func winGetEnvironmentStrings() (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,FindFirstFileA,rawptr
func winFindFirstFile(pattern, findData uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,FindNextFileA,rawptr
func winFindNextFile(handle, findData uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,FindClose,rawptr
func winFindClose(handle uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,CreateProcessA,rawptr
func winCreateProcess(appName, cmdLine, processAttrs, threadAttrs, inheritHandles, creationFlags, env, currentDir, startupInfo, processInfo uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,WaitForSingleObject,rawptr
func winWaitForSingleObject(handle, timeoutMS uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,GetExitCodeProcess,rawptr
func winGetExitCodeProcess(handle, exitCodePtr uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,CreatePipe,rawptr
func winCreatePipe(readPtr, writePtr, securityAttrs, size uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,SetStdHandle,rawptr
func winSetStdHandle(stdHandle, handle uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,GetCurrentProcessId,rawptr
func winGetCurrentProcessID() (uintptr, uintptr, int32)

func SysRead(fd, buf, count uintptr) (uintptr, uintptr, int32) {
	nreadPtr := Alloc(PtrSize)
	Memzero(nreadPtr, PtrSize)
	ok, _, _ := winReadFile(winFdToHandle(fd), buf, count, nreadPtr, 0)
	if ok == 0 {
		return 0, 0, winErrnoFromLastError()
	}
	return ReadPtr(nreadPtr), 0, 0
}

func SysWrite(fd, buf, count uintptr) (uintptr, uintptr, int32) {
	nwrittenPtr := Alloc(PtrSize)
	Memzero(nwrittenPtr, PtrSize)
	ok, _, _ := winWriteFile(winFdToHandle(fd), buf, count, nwrittenPtr, 0)
	if ok == 0 {
		return 0, 0, winErrnoFromLastError()
	}
	return ReadPtr(nwrittenPtr), 0, 0
}

func SysOpen(path, flags, mode uintptr) (uintptr, uintptr, int32) {
	_ = mode
	access := uintptr(winGenericRead)
	disposition := uintptr(winOpenExisting)
	if flags == winFlagWriteCreateTrunc {
		access = winGenericWrite
		disposition = winCreateAlways
	} else if flags == winFlagRDWR {
		access = winGenericReadWrite
	}
	h, _, _ := winCreateFile(path, access, winFileShareRW, 0, disposition, winFileAttrNormal, 0)
	if h == ^uintptr(0) {
		return 0, 0, winErrnoFromLastError()
	}
	return h, 0, 0
}

func SysClose(fd uintptr) (uintptr, uintptr, int32) {
	// Do not close std handles.
	if fd <= 2 {
		return 0, 0, 0
	}
	ok, _, _ := winCloseHandle(fd)
	if ok == 0 {
		return 0, 0, winErrnoFromLastError()
	}
	return 0, 0, 0
}

func SysStat(path, buf uintptr) (uintptr, uintptr, int32) {
	_ = buf
	tmp := Alloc(48)
	Memzero(tmp, 48)
	ok, _, _ := winGetFileAttributesEx(path, 0, tmp)
	if ok == 0 {
		return 0, 0, winErrnoFromLastError()
	}
	return 0, 0, 0
}

func SysExit(code uintptr) {
	winExitProcess(code)
}

func SysMmap(addr, length, prot, flags, fd, offset uintptr) (uintptr, uintptr, int32) {
	_ = addr
	_ = prot
	_ = flags
	_ = fd
	_ = offset
	p, _, _ := winVirtualAlloc(0, length, winMemCommitReserve, winPageReadWrite)
	if p == 0 {
		return 0, 0, winErrnoFromLastError()
	}
	return p, 0, 0
}

func SysMkdir(path, mode uintptr) (uintptr, uintptr, int32) {
	_ = mode
	ok, _, _ := winCreateDirectory(path, 0)
	if ok != 0 {
		return 0, 0, 0
	}
	errn := winErrnoFromLastError()
	if errn == winErrorAlreadyExist {
		return 0, 0, 0
	}
	return 0, 0, errn
}

func SysRmdir(path uintptr) (uintptr, uintptr, int32) {
	ok, _, _ := winRemoveDirectory(path)
	if ok == 0 {
		return 0, 0, winErrnoFromLastError()
	}
	return 0, 0, 0
}

func SysUnlink(path uintptr) (uintptr, uintptr, int32) {
	ok, _, _ := winDeleteFile(path)
	if ok == 0 {
		return 0, 0, winErrnoFromLastError()
	}
	return 0, 0, 0
}

func SysGetcwd(buf, size uintptr) (uintptr, uintptr, int32) {
	n, _, _ := winGetCurrentDirectory(size, buf)
	if n == 0 {
		return 0, 0, winErrnoFromLastError()
	}

	// Normalize separators in place for existing os package expectations.
	b := Makeslice(buf, int(n), int(n))
	i := 0
	for i < len(b) {
		if b[i] == '\\' {
			b[i] = '/'
		}
		i++
	}
	return n, 0, 0
}

func SysGetCommandLine() (uintptr, uintptr, int32) {
	p, _, _ := winGetCommandLine()
	return p, 0, 0
}

func SysGetEnvStrings() (uintptr, uintptr, int32) {
	p, _, _ := winGetEnvironmentStrings()
	return p, 0, 0
}

func SysFindFirstFile(pattern, findData uintptr) (uintptr, uintptr, int32) {
	h, _, _ := winFindFirstFile(pattern, findData)
	if h == ^uintptr(0) {
		return 0, 0, winErrnoFromLastError()
	}
	return h, 0, 0
}

func SysFindNextFile(handle, findData uintptr) (uintptr, uintptr, int32) {
	ok, _, _ := winFindNextFile(handle, findData)
	if ok == 0 {
		return 0, 0, winErrorNoMoreFiles
	}
	return 1, 0, 0
}

func SysFindClose(handle uintptr) (uintptr, uintptr, int32) {
	winFindClose(handle)
	return 0, 0, 0
}

func SysCreateProcess(appName, cmdLine, startupInfo, processInfo, envp uintptr) (uintptr, uintptr, int32) {
	ok, _, _ := winCreateProcess(appName, cmdLine, 0, 0, 1, 0, envp, 0, startupInfo, processInfo)
	if ok == 0 {
		return 0, 0, winErrnoFromLastError()
	}
	return 1, 0, 0
}

func SysWaitProcess(handle, exitCodeBuf uintptr) (uintptr, uintptr, int32) {
	winWaitForSingleObject(handle, winInfinite)
	winGetExitCodeProcess(handle, exitCodeBuf)
	return ReadPtr(exitCodeBuf), 0, 0
}

func SysCreatePipe(readBuf, writeBuf uintptr) (uintptr, uintptr, int32) {
	saSize := 12
	if PtrSize == 8 {
		saSize = 24
	}
	sa := Alloc(saSize)
	Memzero(sa, saSize)
	WritePtr(sa, uintptr(saSize))
	WritePtr(sa+uintptr(PtrSize), 0)
	WritePtr(sa+uintptr(PtrSize*2), 1)

	ok, _, _ := winCreatePipe(readBuf, writeBuf, sa, 0)
	if ok == 0 {
		return 0, 0, winErrnoFromLastError()
	}
	return 1, 0, 0
}

func SysSetStdHandle(stdHandle, handle uintptr) (uintptr, uintptr, int32) {
	winSetStdHandle(stdHandle, handle)
	return 0, 0, 0
}

func SysGetpid() (uintptr, uintptr, int32) {
	pid, _, _ := winGetCurrentProcessID()
	return pid, 0, 0
}
