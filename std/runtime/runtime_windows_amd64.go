//go:build windows && amd64

package runtime

const (
	PtrSize        = 8
	SliceHdrSize   = 32
	StringHdrSize  = 16
	IfaceBoxSize   = 16
	SliceOffLen    = 8
	SliceOffCap    = 16
	SliceOffEsz    = 24
	MapEntrySize   = 16
	MapEntryOffVal = 8
	MmapAnonFlags  = 0 // not used on Windows
)

var GOOS string = "windows"
var GOARCH string = "amd64"

//rtg:linkstatic kernel32.dll,ReadFile
func SysRead(fd, buf, count uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,WriteFile
func SysWrite(fd, buf, count uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,CreateFileA
func SysOpen(path, flags, mode uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,CloseHandle
func SysClose(fd uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,GetFileAttributesExA
func SysStat(path, buf uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,ExitProcess,noreturn
func SysExit(code uintptr)

//rtg:linkstatic kernel32.dll,VirtualAlloc
func SysMmap(addr, length, prot, flags, fd, offset uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,CreateDirectoryA
func SysMkdir(path, mode uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,RemoveDirectoryA
func SysRmdir(path uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,DeleteFileA
func SysUnlink(path uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,GetCurrentDirectoryA
func SysGetcwd(buf, size uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,GetCommandLineA,rawptr
func SysGetCommandLine() (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,GetEnvironmentStringsA,rawptr
func SysGetEnvStrings() (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,FindFirstFileA
func SysFindFirstFile(pattern, findData uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,FindNextFileA
func SysFindNextFile(handle, findData uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,FindClose
func SysFindClose(handle uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,CreateProcessA
func SysCreateProcess(appName, cmdLine, startupInfo, processInfo, envp uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,WaitForSingleObject
func SysWaitProcess(handle, exitCodeBuf uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,CreatePipe
func SysCreatePipe(readBuf, writeBuf uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,SetStdHandle
func SysSetStdHandle(stdHandle, handle uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,GetCurrentProcessId
func SysGetpid() (uintptr, uintptr, int32)
