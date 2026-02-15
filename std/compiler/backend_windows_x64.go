//go:build !no_backend_windows_amd64

package main

import (
	"fmt"
	"os"
)

// winAmd64Imports lists all kernel32.dll functions needed by the Windows amd64 backend.
var winAmd64Imports = []string{
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
	"GetCurrentProcessId",
}

// generateWinAmd64PE compiles an IRModule to a Windows PE32+ (x86-64) executable.
func generateWinAmd64PE(irmod *IRModule, outputPath string) error {
	g := &CodeGen{
		funcOffsets:   make(map[string]int),
		labelOffsets:  make(map[int]int),
		stringMap:     make(map[string]int),
		globalOffsets: make([]int, len(irmod.Globals)),
		baseAddr:      0x400000,
		irmod:         irmod,
		wordSize:      8,
	}

	// Allocate .data space for globals (8 bytes each)
	for i := range irmod.Globals {
		g.globalOffsets[i] = i * 8
	}
	g.data = make([]byte, len(irmod.Globals)*8)

	// Emit entry point
	g.emitStart_win64(irmod)

	// Compile all functions
	for _, f := range irmod.Funcs {
		g.funcOffsets[f.Name] = len(g.code)
		g.compileFunc(f)
	}

	collectNativeFuncSizes(irmod, g.funcOffsets, len(g.code))

	// Resolve call fixups (skip $rodata_header$, $data_addr$, $iat$ — handled by buildPE64)
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

	// Build PE32+
	pe := g.buildPE64(irmod, winAmd64Imports)
	err := os.WriteFile(outputPath, pe, 0755)
	if err != nil {
		return fmt.Errorf("write output: %v", err)
	}

	return nil
}

// emitStart_win64 generates the Windows x64 entry point.
func (g *CodeGen) emitStart_win64(irmod *IRModule) {
	// Windows x64 entry point. RSP is 16-byte aligned on entry.
	// We use R15 as the operand stack pointer (callee-saved, preserved by kernel32).

	// Allocate shadow space (32 bytes) + alignment
	g.subRI(REG_RSP, 48) // 32 shadow + 16 for 16-byte alignment

	// VirtualAlloc(NULL, 16MB, MEM_COMMIT|MEM_RESERVE, PAGE_READWRITE)
	// Microsoft x64 ABI: RCX, RDX, R8, R9
	g.xorRR(REG_RCX, REG_RCX)              // lpAddress = NULL
	g.emitMovRegImm64(REG_RDX, 16*1048576) // dwSize = 16MB
	g.emitMovRegImm64(REG_R8, 0x3000)      // MEM_COMMIT | MEM_RESERVE
	g.emitMovRegImm64(REG_R9, 0x04)        // PAGE_READWRITE
	g.emitCallIAT("VirtualAlloc")

	// R15 = RAX + 16MB (operand stack top, grows down)
	g.movRR(REG_R15, REG_RAX)
	g.emitMovRegImm64(REG_RCX, 16*1048576)
	g.addRR(REG_R15, REG_RCX)

	// Call init functions
	for _, f := range irmod.Funcs {
		if isInitFunc(f.Name) {
			g.emitCallPlaceholder(f.Name)
		}
	}

	// Call main.main
	g.emitCallPlaceholder("main.main")

	// ExitProcess(0)
	g.xorRR(REG_RCX, REG_RCX) // uExitCode = 0
	g.emitCallIAT("ExitProcess")
}

// === Intrinsic dispatcher for Windows x64 ===

func (g *CodeGen) compileCallIntrinsicWin64(inst Inst) {
	g.flush()
	switch inst.Name {
	case "SysRead":
		g.compileSyscallRead_win64()
	case "SysWrite":
		g.compileSyscallWrite_win64()
	case "SysOpen":
		g.compileSyscallOpen_win64()
	case "SysClose":
		g.compileSyscallClose_win64()
	case "SysExit":
		g.compileSyscallExit_win64()
	case "SysMmap":
		g.compileSyscallMmap_win64()
	case "SysMkdir":
		g.compileSyscallMkdir_win64()
	case "SysRmdir":
		g.compileSyscallRmdir_win64()
	case "SysUnlink":
		g.compileSyscallUnlink_win64()
	case "SysGetcwd":
		g.compileSyscallGetcwd_win64()
	case "SysGetdents64":
		g.compileSyscallGetdents_win64()
	case "SysStat":
		g.compileSyscallStat_win64()
	case "SysGetCommandLine":
		g.compileSyscallGetCommandLine_win64()
	case "SysGetEnvStrings":
		g.compileSyscallGetEnvStrings_win64()
	case "SysFindFirstFile":
		g.compileSyscallFindFirstFile_win64()
	case "SysFindNextFile":
		g.compileSyscallFindNextFile_win64()
	case "SysFindClose":
		g.compileSyscallFindClose_win64()
	case "SysCreateProcess":
		g.compileSyscallCreateProcess_win64()
	case "SysWaitProcess":
		g.compileSyscallWaitProcess_win64()
	case "SysCreatePipe":
		g.compileSyscallCreatePipe_win64()
	case "SysSetStdHandle":
		g.compileSyscallSetStdHandle_win64()
	case "SysGetpid":
		g.compileSyscallGetpid_win64()
	case "Sliceptr":
		g.compileSliceptrIntrinsic()
	case "Makeslice":
		g.compileMakesliceIntrinsic()
	case "Stringptr":
		g.compileStringptrIntrinsic()
	case "Makestring":
		g.compileMakestringIntrinsic()
	case "Tostring":
		g.compileTostringIntrinsic()
	case "ReadPtr":
		g.compileReadPtrIntrinsic()
	case "WritePtr":
		g.compileWritePtrIntrinsic()
	case "WriteByte":
		g.compileWriteByteIntrinsic()
	default:
		panic("ICE: unknown intrinsic '" + inst.Name + "' in compileCallIntrinsicWin64")
	}
}

// === Windows fd→handle translation (64-bit) ===
// Loads fd from local, if 0/1/2 calls GetStdHandle, else uses as-is.
// Result in RAX. Caller must have shadow space allocated.
func (g *CodeGen) loadFdAsHandle64(localOffset int) {
	g.emitLoadLocal(localOffset, REG_RAX) // fd

	// if fd <= 2, call GetStdHandle(-10 - fd)
	g.cmpRI(REG_RAX, 2)
	fixNotStd := g.jccRel32(0x87) // ja (unsigned above)

	// fd is 0, 1, or 2: nStdHandle = -10 - fd
	g.negR(REG_RAX)
	g.addRI(REG_RAX, -10)    // rax = -10 - fd
	g.movRR(REG_RCX, REG_RAX) // arg1
	g.emitCallIAT("GetStdHandle")
	// RAX = handle
	fixDone := g.jmpRel32()

	g.patchRel32(fixNotStd)
	// fd > 2: use as-is (handle stored directly)
	g.emitLoadLocal(localOffset, REG_RAX)

	g.patchRel32(fixDone)
}

// === Syscall implementations ===

func (g *CodeGen) compileSyscallMmap_win64() {
	// VirtualAlloc(NULL, size, MEM_COMMIT|MEM_RESERVE, PAGE_READWRITE)
	// 4 args in regs, needs 32-byte shadow space
	g.subRI(REG_RSP, 32)

	g.xorRR(REG_RCX, REG_RCX)             // lpAddress = NULL
	g.emitLoadLocal(2*8, REG_RDX)          // size
	g.emitMovRegImm64(REG_R8, 0x3000)     // MEM_COMMIT | MEM_RESERVE
	g.emitMovRegImm64(REG_R9, 0x04)       // PAGE_READWRITE
	g.emitCallIAT("VirtualAlloc")

	g.addRI(REG_RSP, 32)

	// RAX = address or NULL on failure
	g.testRR(REG_RAX, REG_RAX)
	fixOk := g.jccRel32(CC_NE)
	// Failed: r1=0, r2=0, err=1
	g.compileConstI64(0)
	g.compileConstI64(0)
	g.compileConstI64(1)
	fixDone := g.jmpRel32()

	g.patchRel32(fixOk)
	// Success: r1=addr, r2=0, err=0
	g.opPush(REG_RAX)
	g.compileConstI64(0)
	g.compileConstI64(0)
	g.patchRel32(fixDone)
}

func (g *CodeGen) compileSyscallWrite_win64() {
	// WriteFile(hFile, lpBuffer, nNumberOfBytesToWrite, &nwritten, NULL)
	// 5 args: RCX, RDX, R8, R9, [RSP+32]
	// Stack layout: 32 shadow + 8 (5th arg at [rsp+32]) + 8 nwritten at [rsp+40] = 48
	g.subRI(REG_RSP, 48)

	// Get handle for fd
	g.loadFdAsHandle64(1 * 8)
	g.movRR(REG_RCX, REG_RAX) // hFile

	g.emitLoadLocal(2*8, REG_RDX)                                        // lpBuffer
	g.emitLoadLocal(3*8, REG_R8)                                         // nNumberOfBytesToWrite
	g.emitBytes(0x4c, 0x8d, 0x4c, 0x24, 0x28)                           // lea r9, [rsp+40] = &nwritten
	g.emitBytes(0x48, 0xc7, 0x44, 0x24, 0x20, 0x00, 0x00, 0x00, 0x00)  // mov qword [rsp+32], 0 (lpOverlapped)

	g.emitCallIAT("WriteFile")

	// Read nwritten from [rsp+40]
	g.loadMem(REG_RCX, REG_RSP, 40)

	g.addRI(REG_RSP, 48)

	// Check return value (nonzero = success)
	g.testRR(REG_RAX, REG_RAX)
	fixOk := g.jccRel32(CC_NE)
	// Failed: get error
	g.subRI(REG_RSP, 32)
	g.emitCallIAT("GetLastError")
	g.addRI(REG_RSP, 32)
	g.compileConstI64(0)     // r1 = 0
	g.compileConstI64(0)     // r2 = 0
	g.opPush(REG_RAX)       // err
	fixDone := g.jmpRel32()

	g.patchRel32(fixOk)
	// Success: r1=nwritten, r2=0, err=0
	g.opPush(REG_RCX)
	g.compileConstI64(0)
	g.compileConstI64(0)
	g.patchRel32(fixDone)
}

func (g *CodeGen) compileSyscallRead_win64() {
	// ReadFile(hFile, lpBuffer, nNumberOfBytesToRead, &nread, NULL)
	// 5 args: same layout as WriteFile
	g.subRI(REG_RSP, 48)

	g.loadFdAsHandle64(1 * 8)
	g.movRR(REG_RCX, REG_RAX) // hFile

	g.emitLoadLocal(2*8, REG_RDX)       // lpBuffer
	g.emitLoadLocal(3*8, REG_R8)        // nNumberOfBytesToRead
	g.emitBytes(0x4c, 0x8d, 0x4c, 0x24, 0x28) // lea r9, [rsp+40] = &nread
	g.emitBytes(0x48, 0xc7, 0x44, 0x24, 0x20, 0x00, 0x00, 0x00, 0x00) // mov qword [rsp+32], 0 (lpOverlapped)

	g.emitCallIAT("ReadFile")

	g.loadMem(REG_RCX, REG_RSP, 40) // nread

	g.addRI(REG_RSP, 48)

	g.testRR(REG_RAX, REG_RAX)
	fixOk := g.jccRel32(CC_NE)
	g.subRI(REG_RSP, 32)
	g.emitCallIAT("GetLastError")
	g.addRI(REG_RSP, 32)
	g.compileConstI64(0)
	g.compileConstI64(0)
	g.opPush(REG_RAX)
	fixDone := g.jmpRel32()

	g.patchRel32(fixOk)
	g.opPush(REG_RCX)
	g.compileConstI64(0)
	g.compileConstI64(0)
	g.patchRel32(fixDone)
}

func (g *CodeGen) compileSyscallOpen_win64() {
	// CreateFileA(lpFileName, dwDesiredAccess, dwShareMode, lpSecurityAttributes,
	//             dwCreationDisposition, dwFlagsAndAttributes, hTemplateFile)
	// 7 args: 4 in regs + 3 on stack

	g.emitLoadLocal(2*8, REG_RAX) // flags

	// Default: GENERIC_READ, OPEN_EXISTING
	g.emitMovRegImm64(REG_RCX, 0x80000000) // dwDesiredAccess = GENERIC_READ
	g.emitMovRegImm64(REG_RDX, 3)          // dwCreationDisposition = OPEN_EXISTING

	// Check for O_WRONLY|O_CREAT|O_TRUNC (577 = 0x241)
	g.cmpRI(REG_RAX, 577)
	fixNotWrite := g.jccRel32(CC_NE)
	g.emitMovRegImm64(REG_RCX, 0x40000000) // GENERIC_WRITE
	g.emitMovRegImm64(REG_RDX, 2)          // CREATE_ALWAYS
	fixOpenDone := g.jmpRel32()

	g.patchRel32(fixNotWrite)
	// Check for O_RDWR (2)
	g.cmpRI(REG_RAX, 2)
	fixNotRdwr := g.jccRel32(CC_NE)
	g.emitMovRegImm64(REG_RCX, 0xC0000000) // GENERIC_READ | GENERIC_WRITE
	g.emitMovRegImm64(REG_RDX, 3)          // OPEN_EXISTING

	g.patchRel32(fixNotRdwr)
	g.patchRel32(fixOpenDone)

	// Save computed values
	g.pushR(REG_RCX) // dwDesiredAccess
	g.pushR(REG_RDX) // dwCreationDisposition

	// Allocate stack: 32 shadow + 3*8 stack args = 56 -> round to 64
	g.subRI(REG_RSP, 64)

	// Set up args
	g.emitLoadLocal(1*8, REG_RCX)         // lpFileName
	// Restore dwDesiredAccess from saved
	g.loadMem(REG_RDX, REG_RSP, 64+8) // dwDesiredAccess (pushed second-to-last)
	g.emitMovRegImm64(REG_R8, 3)          // dwShareMode = FILE_SHARE_READ | FILE_SHARE_WRITE
	g.xorRR(REG_R9, REG_R9)               // lpSecurityAttributes = NULL
	// Stack args at [rsp+32], [rsp+40], [rsp+48]
	g.loadMem(REG_RAX, REG_RSP, 64) // dwCreationDisposition (pushed last)
	g.storeMem(REG_RSP, 32, REG_RAX) // [rsp+32] = dwCreationDisposition
	g.emitMovRegImm64(REG_RAX, 0x80)
	g.storeMem(REG_RSP, 40, REG_RAX) // [rsp+40] = FILE_ATTRIBUTE_NORMAL
	g.xorRR(REG_RAX, REG_RAX)
	g.storeMem(REG_RSP, 48, REG_RAX) // [rsp+48] = hTemplateFile = NULL

	g.emitCallIAT("CreateFileA")

	g.addRI(REG_RSP, 64)
	g.popR(REG_RDX) // clean saved values
	g.popR(REG_RDX)

	// RAX = handle or INVALID_HANDLE_VALUE (-1)
	g.emitMovRegImm64(REG_RCX, 0xFFFFFFFFFFFFFFFF)
	g.cmpRR(REG_RAX, REG_RCX)
	fixOpenOk := g.jccRel32(CC_NE)
	// Failed
	g.subRI(REG_RSP, 32)
	g.emitCallIAT("GetLastError")
	g.addRI(REG_RSP, 32)
	g.compileConstI64(0)
	g.compileConstI64(0)
	g.opPush(REG_RAX)
	fixOpenEnd := g.jmpRel32()

	g.patchRel32(fixOpenOk)
	// Success: r1=handle, r2=0, err=0
	g.opPush(REG_RAX)
	g.compileConstI64(0)
	g.compileConstI64(0)
	g.patchRel32(fixOpenEnd)
}

func (g *CodeGen) compileSyscallClose_win64() {
	// CloseHandle(hObject)
	g.emitLoadLocal(1*8, REG_RAX)

	// Don't close std handles (0, 1, 2)
	g.cmpRI(REG_RAX, 2)
	fixNotStd := g.jccRel32(0x87) // ja
	// For std handles, just succeed
	g.compileConstI64(0)
	g.compileConstI64(0)
	g.compileConstI64(0)
	fixCloseDone := g.jmpRel32()

	g.patchRel32(fixNotStd)
	g.subRI(REG_RSP, 32)
	g.movRR(REG_RCX, REG_RAX)
	g.emitCallIAT("CloseHandle")
	g.addRI(REG_RSP, 32)

	g.testRR(REG_RAX, REG_RAX)
	fixCloseOk := g.jccRel32(CC_NE)
	g.subRI(REG_RSP, 32)
	g.emitCallIAT("GetLastError")
	g.addRI(REG_RSP, 32)
	g.compileConstI64(0)
	g.compileConstI64(0)
	g.opPush(REG_RAX)
	fixCloseEnd := g.jmpRel32()

	g.patchRel32(fixCloseOk)
	g.compileConstI64(0)
	g.compileConstI64(0)
	g.compileConstI64(0)

	g.patchRel32(fixCloseEnd)
	g.patchRel32(fixCloseDone)
}

func (g *CodeGen) compileSyscallExit_win64() {
	// ExitProcess(uExitCode)
	g.subRI(REG_RSP, 32)
	g.emitLoadLocal(1*8, REG_RCX)
	g.emitCallIAT("ExitProcess")
	g.addRI(REG_RSP, 32)

	// ExitProcess doesn't return, but push dummy results
	g.compileConstI64(0)
	g.compileConstI64(0)
	g.compileConstI64(0)
}

func (g *CodeGen) compileSyscallMkdir_win64() {
	// CreateDirectoryA(lpPathName, lpSecurityAttributes)
	g.subRI(REG_RSP, 32)
	g.emitLoadLocal(1*8, REG_RCX)   // lpPathName
	g.xorRR(REG_RDX, REG_RDX)       // lpSecurityAttributes = NULL
	g.emitCallIAT("CreateDirectoryA")
	g.addRI(REG_RSP, 32)

	g.testRR(REG_RAX, REG_RAX)
	fixOk := g.jccRel32(CC_NE)
	// Failed: check if ERROR_ALREADY_EXISTS (183)
	g.subRI(REG_RSP, 32)
	g.emitCallIAT("GetLastError")
	g.addRI(REG_RSP, 32)
	g.cmpRI(REG_RAX, 183)
	fixExists := g.jccRel32(CC_E)
	// Real error
	g.compileConstI64(0)
	g.compileConstI64(0)
	g.opPush(REG_RAX)
	fixDone := g.jmpRel32()

	g.patchRel32(fixExists)
	// Already exists = success
	g.compileConstI64(0)
	g.compileConstI64(0)
	g.compileConstI64(0)
	fixDone2 := g.jmpRel32()

	g.patchRel32(fixOk)
	g.compileConstI64(0)
	g.compileConstI64(0)
	g.compileConstI64(0)

	g.patchRel32(fixDone)
	g.patchRel32(fixDone2)
}

func (g *CodeGen) compileSyscallRmdir_win64() {
	// RemoveDirectoryA(lpPathName)
	g.subRI(REG_RSP, 32)
	g.emitLoadLocal(1*8, REG_RCX)
	g.emitCallIAT("RemoveDirectoryA")
	g.addRI(REG_RSP, 32)

	g.emitWinApiReturn64()
}

func (g *CodeGen) compileSyscallUnlink_win64() {
	// DeleteFileA(lpFileName)
	g.subRI(REG_RSP, 32)
	g.emitLoadLocal(1*8, REG_RCX)
	g.emitCallIAT("DeleteFileA")
	g.addRI(REG_RSP, 32)

	g.emitWinApiReturn64()
}

// emitWinApiReturn64 checks RAX (nonzero=ok) and pushes (0,0,0) or (0,0,err).
func (g *CodeGen) emitWinApiReturn64() {
	g.testRR(REG_RAX, REG_RAX)
	fixOk := g.jccRel32(CC_NE)
	g.subRI(REG_RSP, 32)
	g.emitCallIAT("GetLastError")
	g.addRI(REG_RSP, 32)
	g.compileConstI64(0)
	g.compileConstI64(0)
	g.opPush(REG_RAX)
	fixDone := g.jmpRel32()

	g.patchRel32(fixOk)
	g.compileConstI64(0)
	g.compileConstI64(0)
	g.compileConstI64(0)
	g.patchRel32(fixDone)
}

func (g *CodeGen) compileSyscallGetcwd_win64() {
	// GetCurrentDirectoryA(nBufferLength, lpBuffer)
	g.subRI(REG_RSP, 32)
	g.emitLoadLocal(2*8, REG_RCX)    // nBufferLength
	g.emitLoadLocal(1*8, REG_RDX)    // lpBuffer
	g.emitCallIAT("GetCurrentDirectoryA")
	g.addRI(REG_RSP, 32)

	// RAX = number of chars written (not including null), or 0 on error
	g.testRR(REG_RAX, REG_RAX)
	fixOk := g.jccRel32(CC_NE)
	g.subRI(REG_RSP, 32)
	g.emitCallIAT("GetLastError")
	g.addRI(REG_RSP, 32)
	g.compileConstI64(0)
	g.compileConstI64(0)
	g.opPush(REG_RAX)
	fixDone := g.jmpRel32()

	g.patchRel32(fixOk)
	// Convert backslashes to forward slashes in-place
	g.movRR(REG_RCX, REG_RAX)        // save length
	g.emitLoadLocal(1*8, REG_RDX)    // buf ptr
	g.opPush(REG_RCX)                // save length on operand stack

	// Loop: replace '\' with '/'
	g.xorRR(REG_RSI, REG_RSI) // i = 0
	slashLoopStart := len(g.code)
	g.cmpRR(REG_RSI, REG_RCX)
	fixSlashDone := g.jccRel32(CC_GE)
	g.loadMemByte(REG_RAX, REG_RDX, 0)
	g.cmpRI(REG_RAX, '\\')
	fixNotSlash := g.jccRel32(CC_NE)
	g.emitByte(0xb8)
	g.emitU32('/') // mov eax, '/'
	g.storeMemByte(REG_RDX, 0, REG_RAX)
	g.patchRel32(fixNotSlash)
	g.addRI(REG_RDX, 1)
	g.addRI(REG_RSI, 1)
	loopBack := g.jmpRel32()
	g.patchRel32At(loopBack, slashLoopStart)

	g.patchRel32(fixSlashDone)

	// r1 = length (already on opstack), r2 = 0, err = 0
	g.compileConstI64(0)
	g.compileConstI64(0)
	g.patchRel32(fixDone)
}

func (g *CodeGen) compileSyscallGetdents_win64() {
	// Windows doesn't have getdents64. Return error.
	g.compileConstI64(0)
	g.compileConstI64(0)
	g.compileConstI64(1) // ENOSYS
}

func (g *CodeGen) compileSyscallStat_win64() {
	// GetFileAttributesExA(lpFileName, fInfoLevelId, lpFileInformation)
	// Allocate WIN32_FILE_ATTRIBUTE_DATA (36 bytes) on stack + shadow
	// 32 shadow + 48 for struct + pad = 80 -> round to 80 (already 16-aligned)
	g.subRI(REG_RSP, 80)

	g.emitLoadLocal(1*8, REG_RCX)               // lpFileName
	g.xorRR(REG_RDX, REG_RDX)                    // fInfoLevelId = GetFileExInfoStandard
	g.emitBytes(0x4c, 0x8d, 0x44, 0x24, 0x20)   // lea r8, [rsp+32] = lpFileInformation
	g.emitCallIAT("GetFileAttributesExA")

	g.addRI(REG_RSP, 80)

	g.emitWinApiReturn64()
}

func (g *CodeGen) compileSyscallGetCommandLine_win64() {
	// GetCommandLineA() returns a pointer to the command line string
	g.subRI(REG_RSP, 32)
	g.emitCallIAT("GetCommandLineA")
	g.addRI(REG_RSP, 32)
	// r1 = ptr to command line, r2 = 0, err = 0
	g.opPush(REG_RAX)
	g.compileConstI64(0)
	g.compileConstI64(0)
}

func (g *CodeGen) compileSyscallGetEnvStrings_win64() {
	// GetEnvironmentStringsA() returns a pointer to the environment block
	g.subRI(REG_RSP, 32)
	g.emitCallIAT("GetEnvironmentStringsA")
	g.addRI(REG_RSP, 32)
	g.opPush(REG_RAX)
	g.compileConstI64(0)
	g.compileConstI64(0)
}

func (g *CodeGen) compileSyscallGetpid_win64() {
	// GetCurrentProcessId() returns DWORD (the process ID)
	g.subRI(REG_RSP, 32)
	g.emitCallIAT("GetCurrentProcessId")
	g.addRI(REG_RSP, 32)
	g.opPush(REG_RAX)
	g.compileConstI64(0)
	g.compileConstI64(0)
}

func (g *CodeGen) compileSyscallFindFirstFile_win64() {
	// FindFirstFileA(lpFileName, lpFindFileData)
	g.subRI(REG_RSP, 32)
	g.emitLoadLocal(1*8, REG_RCX) // lpFileName
	g.emitLoadLocal(2*8, REG_RDX) // lpFindFileData
	g.emitCallIAT("FindFirstFileA")
	g.addRI(REG_RSP, 32)

	// RAX = handle or INVALID_HANDLE_VALUE (-1)
	g.emitMovRegImm64(REG_RCX, 0xFFFFFFFFFFFFFFFF)
	g.cmpRR(REG_RAX, REG_RCX)
	fixOk := g.jccRel32(CC_NE)
	g.subRI(REG_RSP, 32)
	g.emitCallIAT("GetLastError")
	g.addRI(REG_RSP, 32)
	g.compileConstI64(0)
	g.compileConstI64(0)
	g.opPush(REG_RAX)
	fixDone := g.jmpRel32()

	g.patchRel32(fixOk)
	g.opPush(REG_RAX) // r1 = handle
	g.compileConstI64(0)
	g.compileConstI64(0)
	g.patchRel32(fixDone)
}

func (g *CodeGen) compileSyscallFindNextFile_win64() {
	// FindNextFileA(hFindFile, lpFindFileData)
	g.subRI(REG_RSP, 32)
	g.emitLoadLocal(1*8, REG_RCX)
	g.emitLoadLocal(2*8, REG_RDX)
	g.emitCallIAT("FindNextFileA")
	g.addRI(REG_RSP, 32)

	g.testRR(REG_RAX, REG_RAX)
	fixOk := g.jccRel32(CC_NE)
	// FindNextFileA returned FALSE - no more files
	g.compileConstI64(0)
	g.compileConstI64(0)
	g.compileConstI64(18) // ERROR_NO_MORE_FILES
	fixDone := g.jmpRel32()

	g.patchRel32(fixOk)
	g.compileConstI64(1) // r1 = 1 (success)
	g.compileConstI64(0)
	g.compileConstI64(0)
	g.patchRel32(fixDone)
}

func (g *CodeGen) compileSyscallFindClose_win64() {
	// FindClose(hFindFile)
	g.subRI(REG_RSP, 32)
	g.emitLoadLocal(1*8, REG_RCX)
	g.emitCallIAT("FindClose")
	g.addRI(REG_RSP, 32)

	g.compileConstI64(0)
	g.compileConstI64(0)
	g.compileConstI64(0)
}

func (g *CodeGen) compileSyscallCreateProcess_win64() {
	// CreateProcessA(lpApplicationName, lpCommandLine, lpProcessAttributes,
	//                lpThreadAttributes, bInheritHandles, dwCreationFlags,
	//                lpEnvironment, lpCurrentDirectory, lpStartupInfo, lpProcessInformation)
	// 10 args: 4 in regs + 6 on stack

	// Allocate: 32 shadow + 6*8 stack args = 80 -> round to 80 (16-aligned)
	g.subRI(REG_RSP, 80)

	g.emitLoadLocal(1*8, REG_RCX)   // lpApplicationName
	g.emitLoadLocal(2*8, REG_RDX)   // lpCommandLine
	g.xorRR(REG_R8, REG_R8)         // lpProcessAttributes = NULL
	g.xorRR(REG_R9, REG_R9)         // lpThreadAttributes = NULL

	// Stack args at [rsp+32..rsp+72]
	g.emitByte(0xb8)
	g.emitU32(1) // mov eax, 1
	g.storeMem(REG_RSP, 32, REG_RAX) // bInheritHandles = TRUE
	g.xorRR(REG_RAX, REG_RAX)
	g.storeMem(REG_RSP, 40, REG_RAX) // dwCreationFlags = 0
	g.emitLoadLocal(5*8, REG_RAX)
	g.storeMem(REG_RSP, 48, REG_RAX) // lpEnvironment
	g.xorRR(REG_RAX, REG_RAX)
	g.storeMem(REG_RSP, 56, REG_RAX) // lpCurrentDirectory = NULL
	g.emitLoadLocal(3*8, REG_RAX)
	g.storeMem(REG_RSP, 64, REG_RAX) // lpStartupInfo
	g.emitLoadLocal(4*8, REG_RAX)
	g.storeMem(REG_RSP, 72, REG_RAX) // lpProcessInformation

	g.emitCallIAT("CreateProcessA")

	g.addRI(REG_RSP, 80)

	g.testRR(REG_RAX, REG_RAX)
	fixOk := g.jccRel32(CC_NE)
	g.subRI(REG_RSP, 32)
	g.emitCallIAT("GetLastError")
	g.addRI(REG_RSP, 32)
	g.compileConstI64(0)
	g.compileConstI64(0)
	g.opPush(REG_RAX)
	fixDone := g.jmpRel32()

	g.patchRel32(fixOk)
	g.compileConstI64(1) // success
	g.compileConstI64(0)
	g.compileConstI64(0)
	g.patchRel32(fixDone)
}

func (g *CodeGen) compileSyscallWaitProcess_win64() {
	// WaitForSingleObject(hHandle, INFINITE) then GetExitCodeProcess(hHandle, &exitCode)

	// WaitForSingleObject(hProcess, INFINITE=0xFFFFFFFF)
	g.subRI(REG_RSP, 32)
	g.emitLoadLocal(1*8, REG_RCX) // hProcess
	g.emitMovRegImm64(REG_RDX, 0xFFFFFFFF) // INFINITE
	g.emitCallIAT("WaitForSingleObject")

	// GetExitCodeProcess(hProcess, &exitCode)
	g.emitLoadLocal(1*8, REG_RCX)  // hProcess
	g.emitLoadLocal(2*8, REG_RDX)  // exitCodeBuf
	g.emitCallIAT("GetExitCodeProcess")
	g.addRI(REG_RSP, 32)

	// Read exit code from buffer
	g.emitLoadLocal(2*8, REG_RAX)
	g.loadMem(REG_RAX, REG_RAX, 0)

	g.opPush(REG_RAX) // r1 = exit code
	g.compileConstI64(0)
	g.compileConstI64(0)
}

func (g *CodeGen) compileSyscallCreatePipe_win64() {
	// CreatePipe(&hReadPipe, &hWritePipe, lpPipeAttributes, nSize)
	// We need a SECURITY_ATTRIBUTES struct: {nLength=4, pad=4, lpSecurityDescriptor=8, bInheritHandle=4, pad=4} = 24 bytes
	// Allocate: 32 shadow + 24 SECURITY_ATTRIBUTES + 8 pad = 64
	g.subRI(REG_RSP, 64)

	// Build SECURITY_ATTRIBUTES at RSP+32
	g.emitByte(0xb8)
	g.emitU32(24)
	g.storeMem(REG_RSP, 32, REG_RAX) // nLength = 24 (only low 4 bytes matter)
	g.xorRR(REG_RAX, REG_RAX)
	g.storeMem(REG_RSP, 40, REG_RAX) // lpSecurityDescriptor = NULL
	g.emitByte(0xb8)
	g.emitU32(1)
	g.storeMem(REG_RSP, 48, REG_RAX) // bInheritHandle = TRUE

	g.emitLoadLocal(1*8, REG_RCX)            // &hReadPipe
	g.emitLoadLocal(2*8, REG_RDX)            // &hWritePipe
	g.emitBytes(0x4c, 0x8d, 0x44, 0x24, 0x20) // lea r8, [rsp+32] = lpPipeAttributes
	g.xorRR(REG_R9, REG_R9)                   // nSize = 0 (default)

	g.emitCallIAT("CreatePipe")

	g.addRI(REG_RSP, 64)

	g.testRR(REG_RAX, REG_RAX)
	fixOk := g.jccRel32(CC_NE)
	g.subRI(REG_RSP, 32)
	g.emitCallIAT("GetLastError")
	g.addRI(REG_RSP, 32)
	g.compileConstI64(0)
	g.compileConstI64(0)
	g.opPush(REG_RAX)
	fixDone := g.jmpRel32()

	g.patchRel32(fixOk)
	g.compileConstI64(1)
	g.compileConstI64(0)
	g.compileConstI64(0)
	g.patchRel32(fixDone)
}

func (g *CodeGen) compileSyscallSetStdHandle_win64() {
	// SetStdHandle(nStdHandle, hHandle)
	g.subRI(REG_RSP, 32)
	g.emitLoadLocal(1*8, REG_RCX)
	g.emitLoadLocal(2*8, REG_RDX)
	g.emitCallIAT("SetStdHandle")
	g.addRI(REG_RSP, 32)

	g.compileConstI64(0)
	g.compileConstI64(0)
	g.compileConstI64(0)
}

// compilePanicWin64 handles panic on Windows x64.
func (g *CodeGen) compilePanicWin64() {
	// Pop value from operand stack
	g.opPop(REG_RAX)

	// Tostring heuristic: if first qword < 256, it's an interface box
	g.emitBytes(0x48, 0x8b, 0x08) // mov rcx, [rax]
	g.emitBytes(0x48, 0x81, 0xf9) // cmp rcx, 256
	g.emitU32(256)
	g.emitBytes(0x73, 0x04) // jae +4 (skip next instruction)
	// Interface box: extract value field (the string ptr)
	g.emitBytes(0x48, 0x8b, 0x40, 0x08) // mov rax, [rax+8]

	// RAX = string header ptr {data_ptr, len}
	// Save string info to RBX/R12 (callee-saved, safe across Win64 API calls)
	g.pushR(REG_RBX)
	g.pushR(REG_R12)
	g.loadMem(REG_RBX, REG_RAX, 0)  // RBX = data_ptr
	g.loadMem(REG_R12, REG_RAX, 8)  // R12 = len

	// GetStdHandle(STD_ERROR_HANDLE = -12)
	g.subRI(REG_RSP, 32)
	g.emitMovRegImm64(REG_RCX, 0xFFFFFFFFFFFFFFF4) // -12
	g.emitCallIAT("GetStdHandle")
	// RAX = stderr handle

	// WriteFile(hFile, lpBuffer, nBytes, &nwritten, NULL)
	// Reuse stack: 32 shadow + 8 for 5th arg + 8 for nwritten = 48
	g.addRI(REG_RSP, 32)
	g.subRI(REG_RSP, 48)
	g.movRR(REG_RCX, REG_RAX)       // hFile = stderr
	g.movRR(REG_RDX, REG_RBX)       // lpBuffer = data_ptr
	g.movRR(REG_R8, REG_R12)        // nBytes = len
	g.emitBytes(0x4c, 0x8d, 0x4c, 0x24, 0x28) // lea r9, [rsp+40] = &nwritten
	g.emitBytes(0x48, 0xc7, 0x44, 0x24, 0x20, 0x00, 0x00, 0x00, 0x00) // mov qword [rsp+32], 0 (lpOverlapped)
	g.emitCallIAT("WriteFile")
	g.addRI(REG_RSP, 48)

	// Write newline: push '\n' onto stack as a 1-byte buffer
	g.emitBytes(0x6a, 0x0a) // push 0x0a
	g.movRR(REG_RBX, REG_RSP) // RBX = &'\n'

	// GetStdHandle(STD_ERROR_HANDLE)
	g.subRI(REG_RSP, 32)
	g.emitMovRegImm64(REG_RCX, 0xFFFFFFFFFFFFFFF4)
	g.emitCallIAT("GetStdHandle")
	g.addRI(REG_RSP, 32)

	// WriteFile newline
	g.subRI(REG_RSP, 48)
	g.movRR(REG_RCX, REG_RAX)       // hFile
	g.movRR(REG_RDX, REG_RBX)       // lpBuffer = &'\n'
	g.emitMovRegImm64(REG_R8, 1)    // nBytes = 1
	g.emitBytes(0x4c, 0x8d, 0x4c, 0x24, 0x28) // lea r9, [rsp+40]
	g.emitBytes(0x48, 0xc7, 0x44, 0x24, 0x20, 0x00, 0x00, 0x00, 0x00) // mov qword [rsp+32], 0
	g.emitCallIAT("WriteFile")
	g.addRI(REG_RSP, 48)

	g.addRI(REG_RSP, 8) // pop '\n' slot

	// Restore callee-saved
	g.popR(REG_R12)
	g.popR(REG_RBX)

	// ExitProcess(2)
	g.subRI(REG_RSP, 32)
	g.emitMovRegImm64(REG_RCX, 2)
	g.emitCallIAT("ExitProcess")
	g.addRI(REG_RSP, 32)
}
