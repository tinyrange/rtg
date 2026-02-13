//go:build !no_backend_windows_i386

package main

import (
	"fmt"
	"os"
)

// win386Imports lists all kernel32.dll functions needed by the backend.
var win386Imports = []string{
	"VirtualAlloc",
	"ExitProcess",
	"GetStdHandle",
	"WriteFile",
	"ReadFile",
	"CreateFileA",
	"CloseHandle",
	"GetCommandLineA",
	"GetEnvironmentStringsA",
	"FreeEnvironmentStringsA",
	"GetCurrentDirectoryA",
	"CreateDirectoryA",
	"RemoveDirectoryA",
	"DeleteFileA",
	"FindFirstFileA",
	"FindNextFileA",
	"FindClose",
	"GetFileAttributesExA",
	"CreateProcessA",
	"WaitForSingleObject",
	"GetExitCodeProcess",
	"CreatePipe",
	"SetStdHandle",
	"SetHandleInformation",
	"GetLastError",
}

// generateWin386PE compiles an IRModule to a Windows PE32 executable.
func generateWin386PE(irmod *IRModule, outputPath string) error {
	g := &CodeGen{
		funcOffsets:   make(map[string]int),
		labelOffsets:  make(map[int]int),
		stringMap:     make(map[string]int),
		globalOffsets: make([]int, len(irmod.Globals)),
		baseAddr:      0x400000,
		irmod:         irmod,
		wordSize:      4,
	}

	// Allocate .data space for globals (4 bytes each)
	for i := range irmod.Globals {
		g.globalOffsets[i] = i * 4
	}
	g.data = make([]byte, len(irmod.Globals)*4)

	// Emit entry point
	g.emitStart_win386(irmod)

	// Compile all functions
	for _, f := range irmod.Funcs {
		g.funcOffsets[f.Name] = len(g.code)
		g.compileFunc_i386(f)
	}

	collectNativeFuncSizes(irmod, g.funcOffsets, len(g.code))

	// Resolve call fixups (skip $rodata_header$, $data_addr$, $iat$ — handled by buildPE32)
	var unresolved []string
	for _, fix := range g.callFixups {
		if fix.Target == "$rodata_header$" || fix.Target == "$data_addr$" {
			continue
		}
		if len(fix.Target) > 5 && fix.Target[0:5] == "$iat$" {
			continue
		}
		target, ok := g.funcOffsets[fix.Target]
		if !ok {
			unresolved = append(unresolved, fix.Target)
			continue
		}
		g.patchRel32At(fix.CodeOffset, target)
	}
	if len(unresolved) > 0 {
		fmt.Fprintf(os.Stderr, "error: %d unresolved calls:\n", len(unresolved))
		seen := make(map[string]bool)
		for _, name := range unresolved {
			if !seen[name] {
				fmt.Fprintf(os.Stderr, "  %s\n", name)
				seen[name] = true
			}
		}
		return fmt.Errorf("%d unresolved calls", len(unresolved))
	}

	// Build PE32
	pe := g.buildPE32(irmod, win386Imports)
	err := os.WriteFile(outputPath, pe, 0755)
	if err != nil {
		return fmt.Errorf("write output: %v", err)
	}

	return nil
}

// emitStart_win386 generates the Windows entry point.
func (g *CodeGen) emitStart_win386(irmod *IRModule) {
	// Windows entry point: no arguments passed, we use stdcall.
	// EDI = operand stack pointer (callee-saved)
	// EBP = frame pointer (callee-saved)

	// Save callee-saved registers
	g.pushR32(REG32_EBX)
	g.pushR32(REG32_ESI)

	// Allocate 16MB operand stack via VirtualAlloc
	// VirtualAlloc(NULL, 16*1048576, MEM_COMMIT|MEM_RESERVE, PAGE_READWRITE)
	// stdcall: push args right-to-left, callee cleans stack
	g.pushImm32(0x04)            // PAGE_READWRITE
	g.pushImm32(0x3000)          // MEM_COMMIT | MEM_RESERVE
	g.pushImm32(16 * 1048576)    // dwSize = 16MB
	g.pushImm32(0)               // lpAddress = NULL
	g.emitCallIAT("VirtualAlloc")
	// EAX = base of allocation

	// EDI = EAX + 16MB (top of operand stack, grows down)
	g.movRR32(REG32_EDI, REG32_EAX)
	g.addRI32(REG32_EDI, int32(16*1048576))

	// Call init functions
	for _, f := range irmod.Funcs {
		if isInitFunc(f.Name) {
			g.emitCallPlaceholder(f.Name)
		}
	}

	// Call main.main
	g.emitCallPlaceholder("main.main")

	// ExitProcess(0)
	g.pushImm32(0)
	g.emitCallIAT("ExitProcess")
}

// pushImm32 emits `push imm32`
func (g *CodeGen) pushImm32(val uint32) {
	if val < 128 {
		g.emitBytes(0x6a, byte(val)) // push imm8
	} else {
		g.emitByte(0x68) // push imm32
		g.emitU32(val)
	}
}

// === Windows fd→handle translation ===
// Loads fd from local, if 0/1/2 calls GetStdHandle, else uses as-is.
// Result in EAX.
func (g *CodeGen) loadFdAsHandle(localOffset int) {
	g.emitLoadLocal32(localOffset, REG32_EAX) // fd

	// if fd <= 2, call GetStdHandle(-10 - fd)
	g.cmpRI32(REG32_EAX, 2)
	fixNotStd := g.jccRel32(0x87) // ja (unsigned above)

	// fd is 0, 1, or 2: nStdHandle = -10 - fd
	g.negR32(REG32_EAX)
	g.addRI32(REG32_EAX, -10) // eax = -10 - fd
	g.pushR32(REG32_EAX)
	g.emitCallIAT("GetStdHandle")
	// EAX = handle
	fixDone := g.jmpRel32()

	g.patchRel32(fixNotStd)
	// fd > 2: use as-is (handle stored directly)
	g.emitLoadLocal32(localOffset, REG32_EAX)

	g.patchRel32(fixDone)
}

// === Syscall handlers ===

func (g *CodeGen) compileSyscallMmap_win386() {
	// VirtualAlloc(NULL, size, MEM_COMMIT|MEM_RESERVE, PAGE_READWRITE)
	// param 1 = size (local 2)
	g.pushImm32(0x04)       // PAGE_READWRITE
	g.pushImm32(0x3000)     // MEM_COMMIT | MEM_RESERVE
	g.emitLoadLocal32(2*4, REG32_EAX)
	g.pushR32(REG32_EAX)   // size
	g.pushImm32(0)          // lpAddress = NULL
	g.emitCallIAT("VirtualAlloc")

	// EAX = address or NULL on failure
	g.testRR32(REG32_EAX, REG32_EAX)
	fixOk := g.jccRel32(CC32_NE)
	// Failed: r1=0, r2=0, err=1
	g.compileConstI32(0)
	g.compileConstI32(0)
	g.compileConstI32(1)
	fixDone := g.jmpRel32()

	g.patchRel32(fixOk)
	// Success: r1=addr, r2=0, err=0
	g.opPush(REG32_EAX) // r1
	g.compileConstI32(0)
	g.compileConstI32(0)
	g.patchRel32(fixDone)
}

func (g *CodeGen) compileSyscallWrite_win386() {
	// WriteFile(hFile, lpBuffer, nNumberOfBytesToWrite, &lpNumberOfBytesWritten, NULL)
	// param 0=fd (local 1), param 1=buf (local 2), param 2=count (local 3)

	// Allocate stack space for lpNumberOfBytesWritten
	g.subRI32(REG32_ESP, 4)
	g.movRR32(REG32_ECX, REG32_ESP) // &written

	g.pushImm32(0)          // lpOverlapped = NULL
	g.pushR32(REG32_ECX)    // &written
	g.emitLoadLocal32(3*4, REG32_EAX)
	g.pushR32(REG32_EAX)   // nNumberOfBytesToWrite
	g.emitLoadLocal32(2*4, REG32_EAX)
	g.pushR32(REG32_EAX)   // lpBuffer

	// Translate fd to handle
	g.loadFdAsHandle(1 * 4)
	g.pushR32(REG32_EAX) // hFile

	g.emitCallIAT("WriteFile")

	// Pop written count from stack
	g.popR32(REG32_ECX) // written

	// Check return value (EAX = nonzero on success)
	g.testRR32(REG32_EAX, REG32_EAX)
	fixOk := g.jccRel32(CC32_NE)
	// Failed: get error
	g.emitCallIAT("GetLastError")
	g.compileConstI32(0)     // r1 = 0
	g.compileConstI32(0)     // r2 = 0
	g.opPush(REG32_EAX)     // err
	fixDone := g.jmpRel32()

	g.patchRel32(fixOk)
	// Success: r1=written, r2=0, err=0
	g.opPush(REG32_ECX)
	g.compileConstI32(0)
	g.compileConstI32(0)
	g.patchRel32(fixDone)
}

func (g *CodeGen) compileSyscallRead_win386() {
	// ReadFile(hFile, lpBuffer, nNumberOfBytesToRead, &lpNumberOfBytesRead, NULL)
	// param 0=fd (local 1), param 1=buf (local 2), param 2=count (local 3)

	g.subRI32(REG32_ESP, 4)
	g.movRR32(REG32_ECX, REG32_ESP) // &nread

	g.pushImm32(0)          // lpOverlapped = NULL
	g.pushR32(REG32_ECX)    // &nread
	g.emitLoadLocal32(3*4, REG32_EAX)
	g.pushR32(REG32_EAX)   // nNumberOfBytesToRead
	g.emitLoadLocal32(2*4, REG32_EAX)
	g.pushR32(REG32_EAX)   // lpBuffer

	g.loadFdAsHandle(1 * 4)
	g.pushR32(REG32_EAX)   // hFile

	g.emitCallIAT("ReadFile")

	g.popR32(REG32_ECX) // nread

	g.testRR32(REG32_EAX, REG32_EAX)
	fixOk := g.jccRel32(CC32_NE)
	g.emitCallIAT("GetLastError")
	g.compileConstI32(0)
	g.compileConstI32(0)
	g.opPush(REG32_EAX)
	fixDone := g.jmpRel32()

	g.patchRel32(fixOk)
	g.opPush(REG32_ECX)
	g.compileConstI32(0)
	g.compileConstI32(0)
	g.patchRel32(fixDone)
}

func (g *CodeGen) compileSyscallOpen_win386() {
	// CreateFileA(lpFileName, dwDesiredAccess, dwShareMode, lpSecurityAttributes,
	//             dwCreationDisposition, dwFlagsAndAttributes, hTemplateFile)
	// param 0=path (local 1), param 1=flags (local 2), param 2=mode (local 3)

	// Map Linux open flags to Windows CreateFileA parameters
	g.emitLoadLocal32(2*4, REG32_EAX) // flags

	// Default: GENERIC_READ, OPEN_EXISTING
	g.emitMovRegImm32(REG32_ECX, 0x80000000) // dwDesiredAccess = GENERIC_READ
	g.emitMovRegImm32(REG32_EDX, 3)          // dwCreationDisposition = OPEN_EXISTING

	// Check for O_WRONLY|O_CREAT|O_TRUNC (577 = 0x241)
	g.cmpRI32(REG32_EAX, 577)
	fixNotWrite := g.jccRel32(CC32_NE)
	g.emitMovRegImm32(REG32_ECX, 0x40000000) // GENERIC_WRITE
	g.emitMovRegImm32(REG32_EDX, 2)          // CREATE_ALWAYS
	fixOpenDone := g.jmpRel32()

	g.patchRel32(fixNotWrite)
	// Check for O_RDWR (2)
	g.cmpRI32(REG32_EAX, 2)
	fixNotRdwr := g.jccRel32(CC32_NE)
	g.emitMovRegImm32(REG32_ECX, 0xC0000000) // GENERIC_READ | GENERIC_WRITE
	g.emitMovRegImm32(REG32_EDX, 3)          // OPEN_EXISTING

	g.patchRel32(fixNotRdwr)
	g.patchRel32(fixOpenDone)

	// Push CreateFileA args right-to-left
	g.pushImm32(0)          // hTemplateFile = NULL
	g.pushImm32(0x80)       // dwFlagsAndAttributes = FILE_ATTRIBUTE_NORMAL
	g.pushR32(REG32_EDX)    // dwCreationDisposition
	g.pushImm32(0)          // lpSecurityAttributes = NULL
	g.pushImm32(3)          // dwShareMode = FILE_SHARE_READ | FILE_SHARE_WRITE
	g.pushR32(REG32_ECX)    // dwDesiredAccess
	g.emitLoadLocal32(1*4, REG32_EAX)
	g.pushR32(REG32_EAX)   // lpFileName

	g.emitCallIAT("CreateFileA")

	// EAX = handle or INVALID_HANDLE_VALUE (0xFFFFFFFF)
	g.cmpRI32(REG32_EAX, -1)
	fixOpenOk := g.jccRel32(CC32_NE)
	// Failed
	g.emitCallIAT("GetLastError")
	g.compileConstI32(0)
	g.compileConstI32(0)
	g.opPush(REG32_EAX)
	fixOpenEnd := g.jmpRel32()

	g.patchRel32(fixOpenOk)
	// Success: r1=handle (used as fd), r2=0, err=0
	g.opPush(REG32_EAX)
	g.compileConstI32(0)
	g.compileConstI32(0)
	g.patchRel32(fixOpenEnd)
}

func (g *CodeGen) compileSyscallClose_win386() {
	// CloseHandle(hObject)
	// param 0=fd/handle (local 1)
	g.emitLoadLocal32(1*4, REG32_EAX)

	// Don't close std handles (0, 1, 2)
	g.cmpRI32(REG32_EAX, 2)
	fixNotStd := g.jccRel32(0x87) // ja
	// For std handles, just succeed
	g.compileConstI32(0)
	g.compileConstI32(0)
	g.compileConstI32(0)
	fixCloseDone := g.jmpRel32()

	g.patchRel32(fixNotStd)
	g.pushR32(REG32_EAX)
	g.emitCallIAT("CloseHandle")

	g.testRR32(REG32_EAX, REG32_EAX)
	fixCloseOk := g.jccRel32(CC32_NE)
	g.emitCallIAT("GetLastError")
	g.compileConstI32(0)
	g.compileConstI32(0)
	g.opPush(REG32_EAX)
	fixCloseEnd := g.jmpRel32()

	g.patchRel32(fixCloseOk)
	g.compileConstI32(0)
	g.compileConstI32(0)
	g.compileConstI32(0)

	g.patchRel32(fixCloseEnd)
	g.patchRel32(fixCloseDone)
}

func (g *CodeGen) compileSyscallExit_win386() {
	// ExitProcess(uExitCode)
	// param 0=code (local 1)
	g.emitLoadLocal32(1*4, REG32_EAX)
	g.pushR32(REG32_EAX)
	g.emitCallIAT("ExitProcess")

	// ExitProcess doesn't return, but push dummy results
	g.compileConstI32(0)
	g.compileConstI32(0)
	g.compileConstI32(0)
}

func (g *CodeGen) compileSyscallMkdir_win386() {
	// CreateDirectoryA(lpPathName, lpSecurityAttributes)
	// param 0=path (local 1)
	g.pushImm32(0)          // lpSecurityAttributes = NULL
	g.emitLoadLocal32(1*4, REG32_EAX)
	g.pushR32(REG32_EAX)   // lpPathName

	g.emitCallIAT("CreateDirectoryA")

	g.testRR32(REG32_EAX, REG32_EAX)
	fixOk := g.jccRel32(CC32_NE)
	// Failed: check if ERROR_ALREADY_EXISTS (183)
	g.emitCallIAT("GetLastError")
	g.cmpRI32(REG32_EAX, 183)
	fixExists := g.jccRel32(CC32_E)
	// Real error
	g.compileConstI32(0)
	g.compileConstI32(0)
	g.opPush(REG32_EAX)
	fixDone := g.jmpRel32()

	g.patchRel32(fixExists)
	// Already exists = success (like EEXIST handled in os_linux.go)
	g.compileConstI32(0)
	g.compileConstI32(0)
	g.compileConstI32(0)
	fixDone2 := g.jmpRel32()

	g.patchRel32(fixOk)
	g.compileConstI32(0)
	g.compileConstI32(0)
	g.compileConstI32(0)

	g.patchRel32(fixDone)
	g.patchRel32(fixDone2)
}

func (g *CodeGen) compileSyscallRmdir_win386() {
	// RemoveDirectoryA(lpPathName)
	g.emitLoadLocal32(1*4, REG32_EAX)
	g.pushR32(REG32_EAX)
	g.emitCallIAT("RemoveDirectoryA")

	g.testRR32(REG32_EAX, REG32_EAX)
	fixOk := g.jccRel32(CC32_NE)
	g.emitCallIAT("GetLastError")
	g.compileConstI32(0)
	g.compileConstI32(0)
	g.opPush(REG32_EAX)
	fixDone := g.jmpRel32()

	g.patchRel32(fixOk)
	g.compileConstI32(0)
	g.compileConstI32(0)
	g.compileConstI32(0)
	g.patchRel32(fixDone)
}

func (g *CodeGen) compileSyscallUnlink_win386() {
	// DeleteFileA(lpFileName)
	g.emitLoadLocal32(1*4, REG32_EAX)
	g.pushR32(REG32_EAX)
	g.emitCallIAT("DeleteFileA")

	g.testRR32(REG32_EAX, REG32_EAX)
	fixOk := g.jccRel32(CC32_NE)
	g.emitCallIAT("GetLastError")
	g.compileConstI32(0)
	g.compileConstI32(0)
	g.opPush(REG32_EAX)
	fixDone := g.jmpRel32()

	g.patchRel32(fixOk)
	g.compileConstI32(0)
	g.compileConstI32(0)
	g.compileConstI32(0)
	g.patchRel32(fixDone)
}

func (g *CodeGen) compileSyscallGetcwd_win386() {
	// GetCurrentDirectoryA(nBufferLength, lpBuffer)
	// param 0=buf (local 1), param 1=bufsize (local 2)
	g.emitLoadLocal32(1*4, REG32_EAX)
	g.pushR32(REG32_EAX)   // lpBuffer
	g.emitLoadLocal32(2*4, REG32_EAX)
	g.pushR32(REG32_EAX)   // nBufferLength

	g.emitCallIAT("GetCurrentDirectoryA")

	// EAX = number of chars written (not including null), or 0 on error
	g.testRR32(REG32_EAX, REG32_EAX)
	fixOk := g.jccRel32(CC32_NE)
	g.emitCallIAT("GetLastError")
	g.compileConstI32(0)
	g.compileConstI32(0)
	g.opPush(REG32_EAX)
	fixDone := g.jmpRel32()

	g.patchRel32(fixOk)
	// Convert backslashes to forward slashes in-place
	// EAX = length, buf is at local 1
	g.movRR32(REG32_ECX, REG32_EAX) // save length
	g.emitLoadLocal32(1*4, REG32_EDX) // buf ptr
	// Include null terminator in count: n = eax + 1
	g.addRI32(REG32_EAX, 1)
	g.opPush(REG32_ECX) // save length on operand stack

	// Loop: replace '\' with '/'
	g.xorRR32(REG32_ESI, REG32_ESI) // i = 0
	slashLoopStart := len(g.code)
	g.cmpRR32(REG32_ESI, REG32_ECX)
	fixSlashDone := g.jccRel32(CC32_GE)
	g.loadMemByte32(REG32_EAX, REG32_EDX, 0)
	g.cmpRI32(REG32_EAX, '\\')
	fixNotSlash := g.jccRel32(CC32_NE)
	g.emitMovRegImm32(REG32_EAX, '/')
	g.storeMemByte32(REG32_EDX, 0, REG32_EAX)
	g.patchRel32(fixNotSlash)
	g.addRI32(REG32_EDX, 1)
	g.addRI32(REG32_ESI, 1)
	loopBack := g.jmpRel32()
	g.patchRel32At(loopBack, slashLoopStart)

	g.patchRel32(fixSlashDone)

	// r1 = length (already on opstack), r2 = 0, err = 0
	g.compileConstI32(0)
	g.compileConstI32(0)
	g.patchRel32(fixDone)
}

func (g *CodeGen) compileSyscallGetdents_win386() {
	// Windows doesn't have getdents64. The os_windows.go uses FindFirstFile/FindNextFile instead.
	// For the pseudo-syscall compatibility, return error.
	g.compileConstI32(0)
	g.compileConstI32(0)
	g.compileConstI32(1) // ENOSYS
}

func (g *CodeGen) compileSyscallGetCommandLine_win386() {
	// GetCommandLineA() returns a pointer to the command line string
	g.emitCallIAT("GetCommandLineA")
	// r1 = ptr to command line, r2 = 0, err = 0
	g.opPush(REG32_EAX)
	g.compileConstI32(0)
	g.compileConstI32(0)
}

func (g *CodeGen) compileSyscallGetEnvStrings_win386() {
	// GetEnvironmentStringsA() returns a pointer to the environment block
	g.emitCallIAT("GetEnvironmentStringsA")
	g.opPush(REG32_EAX)
	g.compileConstI32(0)
	g.compileConstI32(0)
}

func (g *CodeGen) compileSyscallFindFirstFile_win386() {
	// FindFirstFileA(lpFileName, lpFindFileData)
	// param 0=pattern (local 1), param 1=buf (local 2)
	g.emitLoadLocal32(2*4, REG32_EAX)
	g.pushR32(REG32_EAX)   // lpFindFileData
	g.emitLoadLocal32(1*4, REG32_EAX)
	g.pushR32(REG32_EAX)   // lpFileName

	g.emitCallIAT("FindFirstFileA")

	// EAX = handle or INVALID_HANDLE_VALUE (0xFFFFFFFF)
	g.cmpRI32(REG32_EAX, -1)
	fixOk := g.jccRel32(CC32_NE)
	g.emitCallIAT("GetLastError")
	g.compileConstI32(0)
	g.compileConstI32(0)
	g.opPush(REG32_EAX)
	fixDone := g.jmpRel32()

	g.patchRel32(fixOk)
	g.opPush(REG32_EAX) // r1 = handle
	g.compileConstI32(0)
	g.compileConstI32(0)
	g.patchRel32(fixDone)
}

func (g *CodeGen) compileSyscallFindNextFile_win386() {
	// FindNextFileA(hFindFile, lpFindFileData)
	// param 0=handle (local 1), param 1=buf (local 2)
	g.emitLoadLocal32(2*4, REG32_EAX)
	g.pushR32(REG32_EAX)
	g.emitLoadLocal32(1*4, REG32_EAX)
	g.pushR32(REG32_EAX)

	g.emitCallIAT("FindNextFileA")

	g.testRR32(REG32_EAX, REG32_EAX)
	fixOk := g.jccRel32(CC32_NE)
	// FindNextFileA returned FALSE - no more files
	// Use ERROR_NO_MORE_FILES (18) rather than GetLastError which may return 0
	g.compileConstI32(0)
	g.compileConstI32(0)
	g.compileConstI32(18) // ERROR_NO_MORE_FILES
	fixDone := g.jmpRel32()

	g.patchRel32(fixOk)
	g.compileConstI32(1) // r1 = 1 (success)
	g.compileConstI32(0)
	g.compileConstI32(0)
	g.patchRel32(fixDone)
}

func (g *CodeGen) compileSyscallFindClose_win386() {
	// FindClose(hFindFile)
	// param 0=handle (local 1)
	g.emitLoadLocal32(1*4, REG32_EAX)
	g.pushR32(REG32_EAX)
	g.emitCallIAT("FindClose")

	g.compileConstI32(0)
	g.compileConstI32(0)
	g.compileConstI32(0)
}

func (g *CodeGen) compileSyscallCreateProcess_win386() {
	// CreateProcessA(lpApplicationName, lpCommandLine, lpProcessAttributes,
	//                lpThreadAttributes, bInheritHandles, dwCreationFlags,
	//                lpEnvironment, lpCurrentDirectory, lpStartupInfo, lpProcessInformation)
	// param 0=appName (local 1), param 1=cmdLine (local 2), param 2=startupInfo (local 3),
	// param 3=processInfo (local 4), param 4=envp (local 5)

	g.emitLoadLocal32(4*4, REG32_EAX) // processInfo
	g.pushR32(REG32_EAX)
	g.emitLoadLocal32(3*4, REG32_EAX) // startupInfo
	g.pushR32(REG32_EAX)
	g.pushImm32(0)          // lpCurrentDirectory = NULL
	g.emitLoadLocal32(5*4, REG32_EAX) // lpEnvironment
	g.pushR32(REG32_EAX)
	g.pushImm32(0)          // dwCreationFlags = 0
	g.pushImm32(1)          // bInheritHandles = TRUE
	g.pushImm32(0)          // lpThreadAttributes = NULL
	g.pushImm32(0)          // lpProcessAttributes = NULL
	g.emitLoadLocal32(2*4, REG32_EAX) // lpCommandLine
	g.pushR32(REG32_EAX)
	g.emitLoadLocal32(1*4, REG32_EAX) // lpApplicationName
	g.pushR32(REG32_EAX)

	g.emitCallIAT("CreateProcessA")

	g.testRR32(REG32_EAX, REG32_EAX)
	fixOk := g.jccRel32(CC32_NE)
	g.emitCallIAT("GetLastError")
	g.compileConstI32(0)
	g.compileConstI32(0)
	g.opPush(REG32_EAX)
	fixDone := g.jmpRel32()

	g.patchRel32(fixOk)
	g.compileConstI32(1) // success
	g.compileConstI32(0)
	g.compileConstI32(0)
	g.patchRel32(fixDone)
}

func (g *CodeGen) compileSyscallWaitProcess_win386() {
	// WaitForSingleObject(hHandle, INFINITE) then GetExitCodeProcess(hHandle, &exitCode)
	// param 0=hProcess (local 1), param 1=exitCodeBuf (local 2)

	// WaitForSingleObject(hProcess, INFINITE=0xFFFFFFFF)
	g.emitMovRegImm32(REG32_EAX, 0xFFFFFFFF)
	g.pushR32(REG32_EAX)   // dwMilliseconds = INFINITE
	g.emitLoadLocal32(1*4, REG32_EAX)
	g.pushR32(REG32_EAX)   // hHandle

	g.emitCallIAT("WaitForSingleObject")

	// GetExitCodeProcess(hProcess, &exitCode)
	g.emitLoadLocal32(2*4, REG32_EAX) // exitCodeBuf
	g.pushR32(REG32_EAX)
	g.emitLoadLocal32(1*4, REG32_EAX) // hProcess
	g.pushR32(REG32_EAX)

	g.emitCallIAT("GetExitCodeProcess")

	// Read exit code from buffer
	g.emitLoadLocal32(2*4, REG32_EAX)
	g.loadMem32(REG32_EAX, REG32_EAX, 0) // exit code

	g.opPush(REG32_EAX) // r1 = exit code
	g.compileConstI32(0)
	g.compileConstI32(0)
}

func (g *CodeGen) compileSyscallCreatePipe_win386() {
	// CreatePipe(&hReadPipe, &hWritePipe, lpPipeAttributes, nSize)
	// param 0=readBuf (local 1), param 1=writeBuf (local 2)
	// We need a SECURITY_ATTRIBUTES struct for inheritable handles
	// {nLength=12, lpSecurityDescriptor=0, bInheritHandle=1}
	g.subRI32(REG32_ESP, 12) // allocate SECURITY_ATTRIBUTES on stack
	g.movRR32(REG32_ECX, REG32_ESP)
	g.emitMovRegImm32(REG32_EAX, 12)
	g.storeMem32(REG32_ECX, 0, REG32_EAX)  // nLength = 12
	g.xorRR32(REG32_EAX, REG32_EAX)
	g.storeMem32(REG32_ECX, 4, REG32_EAX)  // lpSecurityDescriptor = NULL
	g.emitMovRegImm32(REG32_EAX, 1)
	g.storeMem32(REG32_ECX, 8, REG32_EAX)  // bInheritHandle = TRUE

	g.pushImm32(0)          // nSize = 0 (default)
	g.pushR32(REG32_ECX)    // lpPipeAttributes
	g.emitLoadLocal32(2*4, REG32_EAX) // &hWritePipe
	g.pushR32(REG32_EAX)
	g.emitLoadLocal32(1*4, REG32_EAX) // &hReadPipe
	g.pushR32(REG32_EAX)

	g.emitCallIAT("CreatePipe")

	g.addRI32(REG32_ESP, 12) // free SECURITY_ATTRIBUTES

	g.testRR32(REG32_EAX, REG32_EAX)
	fixOk := g.jccRel32(CC32_NE)
	g.emitCallIAT("GetLastError")
	g.compileConstI32(0)
	g.compileConstI32(0)
	g.opPush(REG32_EAX)
	fixDone := g.jmpRel32()

	g.patchRel32(fixOk)
	g.compileConstI32(1)
	g.compileConstI32(0)
	g.compileConstI32(0)
	g.patchRel32(fixDone)
}

func (g *CodeGen) compileSyscallSetStdHandle_win386() {
	// SetStdHandle(nStdHandle, hHandle)
	// param 0=nStdHandle (local 1), param 1=hHandle (local 2)
	g.emitLoadLocal32(2*4, REG32_EAX)
	g.pushR32(REG32_EAX)
	g.emitLoadLocal32(1*4, REG32_EAX)
	g.pushR32(REG32_EAX)

	g.emitCallIAT("SetStdHandle")

	g.compileConstI32(0)
	g.compileConstI32(0)
	g.compileConstI32(0)
}

func (g *CodeGen) compileSyscallStat_win386() {
	// GetFileAttributesExA(lpFileName, fInfoLevelId, lpFileInformation)
	// param 0=path (local 1)
	// Allocate WIN32_FILE_ATTRIBUTE_DATA (36 bytes) on stack
	g.subRI32(REG32_ESP, 36)
	g.movRR32(REG32_ECX, REG32_ESP)

	g.pushR32(REG32_ECX)    // lpFileInformation
	g.pushImm32(0)          // fInfoLevelId = GetFileExInfoStandard
	g.emitLoadLocal32(1*4, REG32_EAX)
	g.pushR32(REG32_EAX)   // lpFileName

	g.emitCallIAT("GetFileAttributesExA")

	g.addRI32(REG32_ESP, 36) // free buffer

	g.testRR32(REG32_EAX, REG32_EAX)
	fixOk := g.jccRel32(CC32_NE)
	g.emitCallIAT("GetLastError")
	g.compileConstI32(0)
	g.compileConstI32(0)
	g.opPush(REG32_EAX)
	fixDone := g.jmpRel32()

	g.patchRel32(fixOk)
	g.compileConstI32(0)
	g.compileConstI32(0)
	g.compileConstI32(0)
	g.patchRel32(fixDone)
}

// compilePanic_win386 handles panic on Windows.
func (g *CodeGen) compilePanic_win386() {
	// Pop value from operand stack
	g.opPop(REG32_EAX)

	// Tostring heuristic: if first dword < 256, it's an interface box
	g.loadMem32(REG32_ECX, REG32_EAX, 0)
	g.cmpRI32(REG32_ECX, int32(256))
	g.emitBytes(0x73, 0x03)              // jae +3
	g.loadMem32(REG32_EAX, REG32_EAX, 4) // interface box: extract value (3 bytes)

	// EAX = string header ptr {data_ptr:4, len:4}
	// Save string info to ESI/EBX (callee-saved, safe across stdcall)
	g.pushR32(REG32_ESI)
	g.pushR32(REG32_EBX)
	g.loadMem32(REG32_ESI, REG32_EAX, 0) // ESI = data_ptr
	g.loadMem32(REG32_EBX, REG32_EAX, 4) // EBX = len

	// GetStdHandle(STD_ERROR_HANDLE = -12)
	g.emitMovRegImm32(REG32_EAX, 0xFFFFFFF4)
	g.pushR32(REG32_EAX)
	g.emitCallIAT("GetStdHandle")
	// EAX = stderr handle, save to ECX
	g.movRR32(REG32_ECX, REG32_EAX)

	// WriteFile(hFile, lpBuffer, nBytes, &written, NULL)
	// Allocate stack space for written count
	g.subRI32(REG32_ESP, 4)
	g.movRR32(REG32_EDX, REG32_ESP) // &written

	g.pushImm32(0)       // lpOverlapped = NULL
	g.pushR32(REG32_EDX) // &written
	g.pushR32(REG32_EBX) // nNumberOfBytesToWrite = len
	g.pushR32(REG32_ESI) // lpBuffer = data_ptr
	g.pushR32(REG32_ECX) // hFile = stderr handle

	g.emitCallIAT("WriteFile") // stdcall: cleans 20 bytes of args
	g.addRI32(REG32_ESP, 4)    // clean written_space

	// Write newline: push '\n' onto stack as a 1-byte buffer
	g.emitBytes(0x6a, 0x0a) // push 0x0a
	g.movRR32(REG32_ESI, REG32_ESP) // ESI = &'\n'

	// GetStdHandle(STD_ERROR_HANDLE)
	g.emitMovRegImm32(REG32_EAX, 0xFFFFFFF4)
	g.pushR32(REG32_EAX)
	g.emitCallIAT("GetStdHandle")
	g.movRR32(REG32_ECX, REG32_EAX) // stderr handle

	g.subRI32(REG32_ESP, 4) // written_space
	g.movRR32(REG32_EDX, REG32_ESP)

	g.pushImm32(0)          // lpOverlapped
	g.pushR32(REG32_EDX)    // &written
	g.pushImm32(1)          // nBytes = 1
	g.pushR32(REG32_ESI)    // lpBuffer = &'\n'
	g.pushR32(REG32_ECX)    // hFile

	g.emitCallIAT("WriteFile")
	g.addRI32(REG32_ESP, 8) // clean written_space(4) + '\n' slot(4)

	// Restore callee-saved
	g.popR32(REG32_EBX)
	g.popR32(REG32_ESI)

	// ExitProcess(2)
	g.pushImm32(2)
	g.emitCallIAT("ExitProcess")
}
