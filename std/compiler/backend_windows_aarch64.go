//go:build !no_backend_arm64

package main

import (
	"fmt"
	"os"
)

// winArm64Imports lists all kernel32.dll functions needed by the Windows ARM64 backend.
var winArm64Imports = []string{
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

// generateWinArm64PE compiles an IRModule to a Windows ARM64 PE32+ executable.
func generateWinArm64PE(irmod *IRModule, outputPath string) error {
	g := &CodeGen{
		funcOffsets:   make(map[string]int),
		labelOffsets:  make(map[int]int),
		stringMap:     make(map[string]int),
		globalOffsets: make([]int, len(irmod.Globals)),
		baseAddr:      0x400000,
		irmod:         irmod,
		wordSize:      8,
		isArm64:       true,
	}

	// Allocate .data space for globals (8 bytes each)
	for i := range irmod.Globals {
		g.globalOffsets[i] = i * 8
	}
	g.data = make([]byte, len(irmod.Globals)*8)

	// Emit entry point
	g.emitStartArm64Windows(irmod)

	// Compile all functions
	for _, f := range irmod.Funcs {
		g.funcOffsets[f.Name] = len(g.code)
		g.compileFuncArm64(f)
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
		g.patchArm64BAt(fix.CodeOffset, target)
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
	pe := g.buildPE64(irmod, winArm64Imports)
	err := os.WriteFile(outputPath, pe, 0755)
	if err != nil {
		return fmt.Errorf("write output: %v", err)
	}

	return nil
}

// emitStartArm64Windows generates the Windows ARM64 entry point.
func (g *CodeGen) emitStartArm64Windows(irmod *IRModule) {
	// Save LR (entry is called by Windows loader)
	g.emitStp(REG_FP, REG_LR, REG_SP, -16)
	g.emitMovRRArm64(REG_FP, REG_SP)

	// Allocate 16MB operand stack via VirtualAlloc
	// VirtualAlloc(NULL, 16*1048576, MEM_COMMIT|MEM_RESERVE, PAGE_READWRITE)
	g.emitMovZ(REG_X0, 0, 0)                          // lpAddress = NULL
	g.emitLoadImm64Compact(REG_X1, 16*1048576)         // dwSize = 16MB
	g.emitLoadImm64Compact(REG_X2, 0x3000)             // MEM_COMMIT | MEM_RESERVE
	g.emitLoadImm64Compact(REG_X3, 0x04)               // PAGE_READWRITE
	g.emitCallIATArm64("VirtualAlloc")

	// X28 = result + 16MB (operand stack top, grows down)
	g.emitLoadImm64Compact(REG_X1, 16*1048576)
	g.emitAddRR(REG_X28, REG_X0, REG_X1)

	// Call init functions
	for _, f := range irmod.Funcs {
		if isInitFunc(f.Name) {
			g.emitCallPlaceholderArm64(f.Name)
		}
	}

	// Call main.main
	g.emitCallPlaceholderArm64("main.main")

	// ExitProcess(0)
	g.emitMovZ(REG_X0, 0, 0)
	g.emitCallIATArm64("ExitProcess")

	// Epilogue (won't reach here)
	g.emitMovRRArm64(REG_SP, REG_FP)
	g.emitLdp(REG_FP, REG_LR, REG_SP, 16)
	g.emitRet()
}

// emitCallIATArm64 emits ADRP+LDR X16 (placeholder) then BLR X16 for calling
// a Windows IAT entry. Creates a $iat$funcName callFixup.
func (g *CodeGen) emitCallIATArm64(funcName string) {
	g.flush()
	off := g.emitAdrp(REG_X16)
	// LDR X16, [X16, #0] — placeholder
	inst := uint32(0xF9400000) | (uint32(REG_X16&0x1f) << 5) | uint32(REG_X16&0x1f)
	g.emitArm64(inst)
	g.callFixups = append(g.callFixups, CallFixup{
		CodeOffset: off,
		Target:     "$iat$" + funcName,
	})
	g.emitBlr(REG_X16)
}

// loadFdAsHandleArm64 loads fd from local, converts 0/1/2 to std handles via GetStdHandle.
// Result in X0. Saves/restores X28 across GetStdHandle call using machine stack.
func (g *CodeGen) loadFdAsHandleArm64(localOffset int) {
	g.emitLoadLocalArm64(localOffset, REG_X0) // fd

	// if fd <= 2, call GetStdHandle(-10 - fd)
	g.emitCmpImm(REG_X0, 2)
	fixNotStd := g.emitBCond(COND_HI) // branch if unsigned above

	// fd is 0, 1, or 2: nStdHandle = -10 - fd
	g.emitNeg(REG_X0, REG_X0)
	g.emitLoadImm64Compact(REG_X1, 0xFFFFFFFFFFFFFFF6) // -10
	g.emitAddRR(REG_X0, REG_X0, REG_X1) // X0 = -10 - fd

	// Save X28 on machine stack
	g.emitSubImm(REG_SP, REG_SP, 16)
	g.emitStr(REG_X28, REG_SP, 0)

	g.emitCallIATArm64("GetStdHandle")
	// X0 = handle

	// Restore X28
	g.emitLdr(REG_X28, REG_SP, 0)
	g.emitAddImm(REG_SP, REG_SP, 16)

	fixDone := g.emitB()

	g.patchArm64BCondAt(fixNotStd, len(g.code))
	// fd > 2: use as-is
	g.emitLoadLocalArm64(localOffset, REG_X0)

	g.patchArm64BAt(fixDone, len(g.code))
}

// emitWinApiReturnArm64 checks return value (nonzero=success) and pushes (r1, r2, err) triple.
// On success: r1=successReg, r2=0, err=0
// On failure: r1=0, r2=0, err=GetLastError()
func (g *CodeGen) emitWinApiReturnArm64(successReg int) {
	g.flush()
	g.emitTstRR(REG_X0, REG_X0)
	fixOk := g.emitBCond(COND_NE)

	// Failed: GetLastError
	g.emitSubImm(REG_SP, REG_SP, 16)
	g.emitStr(REG_X28, REG_SP, 0)
	g.emitCallIATArm64("GetLastError")
	g.emitLdr(REG_X28, REG_SP, 0)
	g.emitAddImm(REG_SP, REG_SP, 16)

	g.emitMovRRArm64(REG_X1, REG_X0) // save error
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0) // r1=0
	g.rawPush(REG_X0) // r2=0
	g.rawPush(REG_X1) // err
	fixDone := g.emitB()

	g.patchArm64BCondAt(fixOk, len(g.code))
	// Success
	g.rawPush(successReg)
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0) // r2=0
	g.rawPush(REG_X0) // err=0

	g.patchArm64BAt(fixDone, len(g.code))
	g.hasPending = false
}

// === Intrinsic dispatcher ===

func (g *CodeGen) compileCallIntrinsicArm64Windows(inst Inst) {
	g.flush()
	switch inst.Name {
	case "SysRead":
		g.compileSyscallRead_winarm64()
	case "SysWrite":
		g.compileSyscallWrite_winarm64()
	case "SysOpen":
		g.compileSyscallOpen_winarm64()
	case "SysClose":
		g.compileSyscallClose_winarm64()
	case "SysExit":
		g.compileSyscallExit_winarm64()
	case "SysMmap":
		g.compileSyscallMmap_winarm64()
	case "SysMkdir":
		g.compileSyscallMkdir_winarm64()
	case "SysRmdir":
		g.compileSyscallRmdir_winarm64()
	case "SysUnlink":
		g.compileSyscallUnlink_winarm64()
	case "SysGetcwd":
		g.compileSyscallGetcwd_winarm64()
	case "SysGetdents64":
		g.compileSyscallGetdents_winarm64()
	case "SysStat":
		g.compileSyscallStat_winarm64()
	case "SysGetCommandLine":
		g.compileSyscallGetCommandLine_winarm64()
	case "SysGetEnvStrings":
		g.compileSyscallGetEnvStrings_winarm64()
	case "SysFindFirstFile":
		g.compileSyscallFindFirstFile_winarm64()
	case "SysFindNextFile":
		g.compileSyscallFindNextFile_winarm64()
	case "SysFindClose":
		g.compileSyscallFindClose_winarm64()
	case "SysCreateProcess":
		g.compileSyscallCreateProcess_winarm64()
	case "SysWaitProcess":
		g.compileSyscallWaitProcess_winarm64()
	case "SysCreatePipe":
		g.compileSyscallCreatePipe_winarm64()
	case "SysSetStdHandle":
		g.compileSyscallSetStdHandle_winarm64()
	case "SysGetpid":
		g.compileSyscallGetpid_winarm64()
	case "Sliceptr":
		g.compileSliceptrIntrinsicArm64()
	case "Makeslice":
		g.compileMakesliceIntrinsicArm64()
	case "Stringptr":
		g.compileStringptrIntrinsicArm64()
	case "Makestring":
		g.compileMakestringIntrinsicArm64()
	case "Tostring":
		g.compileTostringIntrinsicArm64()
	case "ReadPtr":
		g.compileReadPtrIntrinsicArm64()
	case "WritePtr":
		g.compileWritePtrIntrinsicArm64()
	case "WriteByte":
		g.compileWriteByteIntrinsicArm64()
	default:
		panic("ICE: unknown intrinsic '" + inst.Name + "' in compileCallIntrinsicArm64Windows")
	}
}

// === Syscall implementations ===

func (g *CodeGen) compileSyscallMmap_winarm64() {
	// VirtualAlloc(NULL, size, MEM_COMMIT|MEM_RESERVE, PAGE_READWRITE)
	g.emitMovZ(REG_X0, 0, 0)                      // lpAddress = NULL
	g.emitLoadLocalArm64(2*8, REG_X1)              // size
	g.emitLoadImm64Compact(REG_X2, 0x3000)         // MEM_COMMIT | MEM_RESERVE
	g.emitLoadImm64Compact(REG_X3, 0x04)           // PAGE_READWRITE

	g.emitSubImm(REG_SP, REG_SP, 16)
	g.emitStr(REG_X28, REG_SP, 0)
	g.emitCallIATArm64("VirtualAlloc")
	g.emitLdr(REG_X28, REG_SP, 0)
	g.emitAddImm(REG_SP, REG_SP, 16)

	// X0 = address or NULL on failure
	g.flush()
	g.emitTstRR(REG_X0, REG_X0)
	fixOk := g.emitBCond(COND_NE)
	// Failed
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0) // r1=0
	g.rawPush(REG_X0) // r2=0
	g.emitLoadImm64Compact(REG_X0, 1)
	g.rawPush(REG_X0) // err=1
	fixDone := g.emitB()

	g.patchArm64BCondAt(fixOk, len(g.code))
	g.rawPush(REG_X0) // r1=addr
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0) // r2=0
	g.rawPush(REG_X0) // err=0
	g.patchArm64BAt(fixDone, len(g.code))
	g.hasPending = false
}

func (g *CodeGen) compileSyscallWrite_winarm64() {
	// WriteFile(hFile, lpBuffer, nNumberOfBytesToWrite, &nwritten, NULL)
	// Allocate 16 bytes on SP: 8 for nwritten, 8 for X28 save
	g.emitSubImm(REG_SP, REG_SP, 32)
	g.emitStr(REG_X28, REG_SP, 0) // save X28

	g.loadFdAsHandleArm64(1 * 8) // X0 = handle
	// Save handle on stack
	g.emitStr(REG_X0, REG_SP, 8)

	// Prepare args
	g.emitLdr(REG_X0, REG_SP, 8)       // hFile
	g.emitLoadLocalArm64(2*8, REG_X1)   // lpBuffer
	g.emitLoadLocalArm64(3*8, REG_X2)   // nNumberOfBytesToWrite
	g.emitAddImm(REG_X3, REG_SP, 16)    // &nwritten
	g.emitMovZ(REG_X4, 0, 0)            // lpOverlapped = NULL

	g.emitCallIATArm64("WriteFile")

	// Read nwritten
	g.emitLdr(REG_X2, REG_SP, 16)
	g.emitLdr(REG_X28, REG_SP, 0) // restore X28
	g.emitAddImm(REG_SP, REG_SP, 32)

	g.flush()
	g.emitTstRR(REG_X0, REG_X0)
	fixOk := g.emitBCond(COND_NE)

	// Failed
	g.emitSubImm(REG_SP, REG_SP, 16)
	g.emitStr(REG_X28, REG_SP, 0)
	g.emitCallIATArm64("GetLastError")
	g.emitLdr(REG_X28, REG_SP, 0)
	g.emitAddImm(REG_SP, REG_SP, 16)
	g.emitMovRRArm64(REG_X1, REG_X0)
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0) // r1=0
	g.rawPush(REG_X0) // r2=0
	g.rawPush(REG_X1) // err
	fixDone := g.emitB()

	g.patchArm64BCondAt(fixOk, len(g.code))
	g.rawPush(REG_X2) // r1=nwritten
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0) // r2=0
	g.rawPush(REG_X0) // err=0
	g.patchArm64BAt(fixDone, len(g.code))
	g.hasPending = false
}

func (g *CodeGen) compileSyscallRead_winarm64() {
	// ReadFile(hFile, lpBuffer, nNumberOfBytesToRead, &nread, NULL)
	g.emitSubImm(REG_SP, REG_SP, 32)
	g.emitStr(REG_X28, REG_SP, 0)

	g.loadFdAsHandleArm64(1 * 8)
	g.emitStr(REG_X0, REG_SP, 8)

	g.emitLdr(REG_X0, REG_SP, 8)       // hFile
	g.emitLoadLocalArm64(2*8, REG_X1)   // lpBuffer
	g.emitLoadLocalArm64(3*8, REG_X2)   // nNumberOfBytesToRead
	g.emitAddImm(REG_X3, REG_SP, 16)    // &nread
	g.emitMovZ(REG_X4, 0, 0)            // lpOverlapped

	g.emitCallIATArm64("ReadFile")

	g.emitLdr(REG_X2, REG_SP, 16)
	g.emitLdr(REG_X28, REG_SP, 0)
	g.emitAddImm(REG_SP, REG_SP, 32)

	g.flush()
	g.emitTstRR(REG_X0, REG_X0)
	fixOk := g.emitBCond(COND_NE)

	g.emitSubImm(REG_SP, REG_SP, 16)
	g.emitStr(REG_X28, REG_SP, 0)
	g.emitCallIATArm64("GetLastError")
	g.emitLdr(REG_X28, REG_SP, 0)
	g.emitAddImm(REG_SP, REG_SP, 16)
	g.emitMovRRArm64(REG_X1, REG_X0)
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X1)
	fixDone := g.emitB()

	g.patchArm64BCondAt(fixOk, len(g.code))
	g.rawPush(REG_X2)
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X0)
	g.patchArm64BAt(fixDone, len(g.code))
	g.hasPending = false
}

func (g *CodeGen) compileSyscallOpen_winarm64() {
	// CreateFileA(lpFileName, dwDesiredAccess, dwShareMode, lpSecurityAttributes,
	//             dwCreationDisposition, dwFlagsAndAttributes, hTemplateFile)
	g.emitLoadLocalArm64(2*8, REG_X0) // flags

	// Default: GENERIC_READ, OPEN_EXISTING
	g.emitLoadImm64Compact(REG_X3, 0x80000000) // dwDesiredAccess = GENERIC_READ
	g.emitLoadImm64Compact(REG_X4, 3)           // dwCreationDisposition = OPEN_EXISTING

	// Check for O_WRONLY|O_CREAT|O_TRUNC (577 = 0x241)
	g.emitCmpImm(REG_X0, 577)
	fixNotWrite := g.emitBCond(COND_NE)
	g.emitLoadImm64Compact(REG_X3, 0x40000000) // GENERIC_WRITE
	g.emitLoadImm64Compact(REG_X4, 2)           // CREATE_ALWAYS
	fixOpenDone := g.emitB()

	g.patchArm64BCondAt(fixNotWrite, len(g.code))
	// Check for O_RDWR (2)
	g.emitCmpImm(REG_X0, 2)
	fixNotRdwr := g.emitBCond(COND_NE)
	g.emitLoadImm64Compact(REG_X3, 0xC0000000) // GENERIC_READ | GENERIC_WRITE
	g.emitLoadImm64Compact(REG_X4, 3)           // OPEN_EXISTING

	g.patchArm64BCondAt(fixNotRdwr, len(g.code))
	g.patchArm64BAt(fixOpenDone, len(g.code))

	// Save X28 and computed values on stack
	g.emitSubImm(REG_SP, REG_SP, 32)
	g.emitStr(REG_X28, REG_SP, 0)
	g.emitStr(REG_X3, REG_SP, 8)
	g.emitStr(REG_X4, REG_SP, 16)

	// CreateFileA args: X0=lpFileName, X1=dwDesiredAccess, X2=dwShareMode,
	// X3=lpSecurityAttributes, X4=dwCreationDisposition, X5=dwFlagsAndAttributes, X6=hTemplateFile
	g.emitLoadLocalArm64(1*8, REG_X0)           // lpFileName
	g.emitLdr(REG_X1, REG_SP, 8)                // dwDesiredAccess
	g.emitLoadImm64Compact(REG_X2, 3)           // dwShareMode = FILE_SHARE_READ | FILE_SHARE_WRITE
	g.emitMovZ(REG_X3, 0, 0)                    // lpSecurityAttributes = NULL
	g.emitLdr(REG_X4, REG_SP, 16)               // dwCreationDisposition
	g.emitLoadImm64Compact(REG_X5, 0x80)         // dwFlagsAndAttributes = FILE_ATTRIBUTE_NORMAL
	g.emitMovZ(REG_X6, 0, 0)                    // hTemplateFile = NULL

	g.emitCallIATArm64("CreateFileA")

	g.emitLdr(REG_X28, REG_SP, 0)
	g.emitAddImm(REG_SP, REG_SP, 32)

	// X0 = handle or INVALID_HANDLE_VALUE (-1)
	g.flush()
	g.emitLoadImm64Compact(REG_X1, 0xFFFFFFFFFFFFFFFF)
	g.emitCmpRR(REG_X0, REG_X1)
	fixOpenOk := g.emitBCond(COND_NE)

	// Failed
	g.emitSubImm(REG_SP, REG_SP, 16)
	g.emitStr(REG_X28, REG_SP, 0)
	g.emitCallIATArm64("GetLastError")
	g.emitLdr(REG_X28, REG_SP, 0)
	g.emitAddImm(REG_SP, REG_SP, 16)
	g.emitMovRRArm64(REG_X1, REG_X0)
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0) // r1=0
	g.rawPush(REG_X0) // r2=0
	g.rawPush(REG_X1) // err
	fixOpenEnd := g.emitB()

	g.patchArm64BCondAt(fixOpenOk, len(g.code))
	g.rawPush(REG_X0) // r1=handle
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0) // r2=0
	g.rawPush(REG_X0) // err=0
	g.patchArm64BAt(fixOpenEnd, len(g.code))
	g.hasPending = false
}

func (g *CodeGen) compileSyscallClose_winarm64() {
	g.emitLoadLocalArm64(1*8, REG_X0)

	// Don't close std handles (0, 1, 2)
	g.emitCmpImm(REG_X0, 2)
	fixNotStd := g.emitBCond(COND_HI)
	// For std handles, just succeed
	g.flush()
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0) // r1=0
	g.rawPush(REG_X0) // r2=0
	g.rawPush(REG_X0) // err=0
	fixCloseDone := g.emitB()

	g.patchArm64BCondAt(fixNotStd, len(g.code))
	// CloseHandle(handle)
	g.emitSubImm(REG_SP, REG_SP, 16)
	g.emitStr(REG_X28, REG_SP, 0)
	g.emitCallIATArm64("CloseHandle")
	g.emitLdr(REG_X28, REG_SP, 0)
	g.emitAddImm(REG_SP, REG_SP, 16)

	g.flush()
	g.emitTstRR(REG_X0, REG_X0)
	fixCloseOk := g.emitBCond(COND_NE)

	g.emitSubImm(REG_SP, REG_SP, 16)
	g.emitStr(REG_X28, REG_SP, 0)
	g.emitCallIATArm64("GetLastError")
	g.emitLdr(REG_X28, REG_SP, 0)
	g.emitAddImm(REG_SP, REG_SP, 16)
	g.emitMovRRArm64(REG_X1, REG_X0)
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X1)
	fixCloseEnd := g.emitB()

	g.patchArm64BCondAt(fixCloseOk, len(g.code))
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X0)

	g.patchArm64BAt(fixCloseEnd, len(g.code))
	g.patchArm64BAt(fixCloseDone, len(g.code))
	g.hasPending = false
}

func (g *CodeGen) compileSyscallExit_winarm64() {
	g.emitLoadLocalArm64(1*8, REG_X0)
	g.emitSubImm(REG_SP, REG_SP, 16)
	g.emitStr(REG_X28, REG_SP, 0)
	g.emitCallIATArm64("ExitProcess")
	// Does not return, but push dummy results
	g.emitLdr(REG_X28, REG_SP, 0)
	g.emitAddImm(REG_SP, REG_SP, 16)
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X0)
	g.hasPending = false
}

func (g *CodeGen) compileSyscallMkdir_winarm64() {
	// CreateDirectoryA(lpPathName, lpSecurityAttributes)
	g.emitLoadLocalArm64(1*8, REG_X0) // lpPathName
	g.emitMovZ(REG_X1, 0, 0)          // lpSecurityAttributes = NULL

	g.emitSubImm(REG_SP, REG_SP, 16)
	g.emitStr(REG_X28, REG_SP, 0)
	g.emitCallIATArm64("CreateDirectoryA")
	g.emitLdr(REG_X28, REG_SP, 0)
	g.emitAddImm(REG_SP, REG_SP, 16)

	g.flush()
	g.emitTstRR(REG_X0, REG_X0)
	fixOk := g.emitBCond(COND_NE)

	// Failed: check ERROR_ALREADY_EXISTS (183)
	g.emitSubImm(REG_SP, REG_SP, 16)
	g.emitStr(REG_X28, REG_SP, 0)
	g.emitCallIATArm64("GetLastError")
	g.emitLdr(REG_X28, REG_SP, 0)
	g.emitAddImm(REG_SP, REG_SP, 16)
	g.emitCmpImm(REG_X0, 183)
	fixExists := g.emitBCond(COND_EQ)

	// Real error
	g.emitMovRRArm64(REG_X1, REG_X0)
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X1)
	fixDone := g.emitB()

	g.patchArm64BCondAt(fixExists, len(g.code))
	// Already exists = success
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X0)
	fixDone2 := g.emitB()

	g.patchArm64BCondAt(fixOk, len(g.code))
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X0)

	g.patchArm64BAt(fixDone, len(g.code))
	g.patchArm64BAt(fixDone2, len(g.code))
	g.hasPending = false
}

func (g *CodeGen) compileSyscallRmdir_winarm64() {
	g.emitLoadLocalArm64(1*8, REG_X0)
	g.emitSubImm(REG_SP, REG_SP, 16)
	g.emitStr(REG_X28, REG_SP, 0)
	g.emitCallIATArm64("RemoveDirectoryA")
	g.emitLdr(REG_X28, REG_SP, 0)
	g.emitAddImm(REG_SP, REG_SP, 16)

	g.emitWinApiReturnSimpleArm64()
}

func (g *CodeGen) compileSyscallUnlink_winarm64() {
	g.emitLoadLocalArm64(1*8, REG_X0)
	g.emitSubImm(REG_SP, REG_SP, 16)
	g.emitStr(REG_X28, REG_SP, 0)
	g.emitCallIATArm64("DeleteFileA")
	g.emitLdr(REG_X28, REG_SP, 0)
	g.emitAddImm(REG_SP, REG_SP, 16)

	g.emitWinApiReturnSimpleArm64()
}

// emitWinApiReturnSimpleArm64 checks X0 (nonzero=ok) and pushes (0,0,0) or (0,0,err).
func (g *CodeGen) emitWinApiReturnSimpleArm64() {
	g.flush()
	g.emitTstRR(REG_X0, REG_X0)
	fixOk := g.emitBCond(COND_NE)

	g.emitSubImm(REG_SP, REG_SP, 16)
	g.emitStr(REG_X28, REG_SP, 0)
	g.emitCallIATArm64("GetLastError")
	g.emitLdr(REG_X28, REG_SP, 0)
	g.emitAddImm(REG_SP, REG_SP, 16)
	g.emitMovRRArm64(REG_X1, REG_X0)
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X1)
	fixDone := g.emitB()

	g.patchArm64BCondAt(fixOk, len(g.code))
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X0)
	g.patchArm64BAt(fixDone, len(g.code))
	g.hasPending = false
}

func (g *CodeGen) compileSyscallGetcwd_winarm64() {
	// GetCurrentDirectoryA(nBufferLength, lpBuffer)
	g.emitLoadLocalArm64(2*8, REG_X0)  // nBufferLength
	g.emitLoadLocalArm64(1*8, REG_X1)  // lpBuffer

	g.emitSubImm(REG_SP, REG_SP, 16)
	g.emitStr(REG_X28, REG_SP, 0)
	g.emitCallIATArm64("GetCurrentDirectoryA")
	g.emitLdr(REG_X28, REG_SP, 0)
	g.emitAddImm(REG_SP, REG_SP, 16)

	// X0 = chars written (not including null), or 0 on error
	g.flush()
	g.emitTstRR(REG_X0, REG_X0)
	fixOk := g.emitBCond(COND_NE)

	g.emitSubImm(REG_SP, REG_SP, 16)
	g.emitStr(REG_X28, REG_SP, 0)
	g.emitCallIATArm64("GetLastError")
	g.emitLdr(REG_X28, REG_SP, 0)
	g.emitAddImm(REG_SP, REG_SP, 16)
	g.emitMovRRArm64(REG_X1, REG_X0)
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X1)
	fixDoneCwd := g.emitB()

	g.patchArm64BCondAt(fixOk, len(g.code))
	// Convert backslashes to forward slashes
	g.emitMovRRArm64(REG_X2, REG_X0) // save length
	g.emitLoadLocalArm64(1*8, REG_X3) // buf ptr
	g.emitMovZ(REG_X4, 0, 0)          // i = 0

	slashLoopStart := len(g.code)
	g.emitCmpRR(REG_X4, REG_X2)
	fixSlashDone := g.emitBCond(COND_GE)
	g.emitLdrb(REG_X5, REG_X3, 0)
	g.emitCmpImm(REG_X5, '\\')
	fixNotSlash := g.emitBCond(COND_NE)
	g.emitLoadImm64Compact(REG_X5, '/')
	g.emitStrb(REG_X5, REG_X3, 0)
	g.patchArm64BCondAt(fixNotSlash, len(g.code))
	g.emitAddImm(REG_X3, REG_X3, 1)
	g.emitAddImm(REG_X4, REG_X4, 1)
	loopBack := g.emitB()
	g.patchArm64BAt(loopBack, slashLoopStart)

	g.patchArm64BCondAt(fixSlashDone, len(g.code))
	// r1=length, r2=0, err=0
	g.rawPush(REG_X2)
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X0)
	g.patchArm64BAt(fixDoneCwd, len(g.code))
	g.hasPending = false
}

func (g *CodeGen) compileSyscallGetdents_winarm64() {
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X0)
	g.emitLoadImm64Compact(REG_X0, 1) // ENOSYS
	g.rawPush(REG_X0)
	g.hasPending = false
}

func (g *CodeGen) compileSyscallStat_winarm64() {
	// GetFileAttributesExA(lpFileName, fInfoLevelId, lpFileInformation)
	// Allocate 48 bytes on stack: 36 for WIN32_FILE_ATTRIBUTE_DATA + pad + X28 save
	g.emitSubImm(REG_SP, REG_SP, 64)
	g.emitStr(REG_X28, REG_SP, 0)

	g.emitLoadLocalArm64(1*8, REG_X0)           // lpFileName
	g.emitMovZ(REG_X1, 0, 0)                    // fInfoLevelId = GetFileExInfoStandard
	g.emitAddImm(REG_X2, REG_SP, 16)            // lpFileInformation

	g.emitCallIATArm64("GetFileAttributesExA")

	g.emitLdr(REG_X28, REG_SP, 0)
	g.emitAddImm(REG_SP, REG_SP, 64)

	g.emitWinApiReturnSimpleArm64()
}

func (g *CodeGen) compileSyscallGetCommandLine_winarm64() {
	g.emitSubImm(REG_SP, REG_SP, 16)
	g.emitStr(REG_X28, REG_SP, 0)
	g.emitCallIATArm64("GetCommandLineA")
	g.emitLdr(REG_X28, REG_SP, 0)
	g.emitAddImm(REG_SP, REG_SP, 16)
	g.rawPush(REG_X0)
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X0)
	g.hasPending = false
}

func (g *CodeGen) compileSyscallGetEnvStrings_winarm64() {
	g.emitSubImm(REG_SP, REG_SP, 16)
	g.emitStr(REG_X28, REG_SP, 0)
	g.emitCallIATArm64("GetEnvironmentStringsA")
	g.emitLdr(REG_X28, REG_SP, 0)
	g.emitAddImm(REG_SP, REG_SP, 16)
	g.rawPush(REG_X0)
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X0)
	g.hasPending = false
}

func (g *CodeGen) compileSyscallGetpid_winarm64() {
	g.emitSubImm(REG_SP, REG_SP, 16)
	g.emitStr(REG_X28, REG_SP, 0)
	g.emitCallIATArm64("GetCurrentProcessId")
	g.emitLdr(REG_X28, REG_SP, 0)
	g.emitAddImm(REG_SP, REG_SP, 16)
	g.rawPush(REG_X0)
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X0)
	g.hasPending = false
}

func (g *CodeGen) compileSyscallFindFirstFile_winarm64() {
	// FindFirstFileA(lpFileName, lpFindFileData)
	g.emitLoadLocalArm64(1*8, REG_X0) // lpFileName
	g.emitLoadLocalArm64(2*8, REG_X1) // lpFindFileData

	g.emitSubImm(REG_SP, REG_SP, 16)
	g.emitStr(REG_X28, REG_SP, 0)
	g.emitCallIATArm64("FindFirstFileA")
	g.emitLdr(REG_X28, REG_SP, 0)
	g.emitAddImm(REG_SP, REG_SP, 16)

	// X0 = handle or INVALID_HANDLE_VALUE (-1)
	g.flush()
	g.emitLoadImm64Compact(REG_X1, 0xFFFFFFFFFFFFFFFF)
	g.emitCmpRR(REG_X0, REG_X1)
	fixOk := g.emitBCond(COND_NE)

	g.emitSubImm(REG_SP, REG_SP, 16)
	g.emitStr(REG_X28, REG_SP, 0)
	g.emitCallIATArm64("GetLastError")
	g.emitLdr(REG_X28, REG_SP, 0)
	g.emitAddImm(REG_SP, REG_SP, 16)
	g.emitMovRRArm64(REG_X1, REG_X0)
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X1)
	fixDone := g.emitB()

	g.patchArm64BCondAt(fixOk, len(g.code))
	g.rawPush(REG_X0) // r1=handle
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X0)
	g.patchArm64BAt(fixDone, len(g.code))
	g.hasPending = false
}

func (g *CodeGen) compileSyscallFindNextFile_winarm64() {
	// FindNextFileA(hFindFile, lpFindFileData)
	g.emitLoadLocalArm64(1*8, REG_X0)
	g.emitLoadLocalArm64(2*8, REG_X1)

	g.emitSubImm(REG_SP, REG_SP, 16)
	g.emitStr(REG_X28, REG_SP, 0)
	g.emitCallIATArm64("FindNextFileA")
	g.emitLdr(REG_X28, REG_SP, 0)
	g.emitAddImm(REG_SP, REG_SP, 16)

	g.flush()
	g.emitTstRR(REG_X0, REG_X0)
	fixOk := g.emitBCond(COND_NE)
	// No more files
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X0)
	g.emitLoadImm64Compact(REG_X0, 18) // ERROR_NO_MORE_FILES
	g.rawPush(REG_X0)
	fixDone := g.emitB()

	g.patchArm64BCondAt(fixOk, len(g.code))
	g.emitLoadImm64Compact(REG_X0, 1)
	g.rawPush(REG_X0) // r1=1 (success)
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X0)
	g.patchArm64BAt(fixDone, len(g.code))
	g.hasPending = false
}

func (g *CodeGen) compileSyscallFindClose_winarm64() {
	g.emitLoadLocalArm64(1*8, REG_X0)
	g.emitSubImm(REG_SP, REG_SP, 16)
	g.emitStr(REG_X28, REG_SP, 0)
	g.emitCallIATArm64("FindClose")
	g.emitLdr(REG_X28, REG_SP, 0)
	g.emitAddImm(REG_SP, REG_SP, 16)
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X0)
	g.hasPending = false
}

func (g *CodeGen) compileSyscallCreateProcess_winarm64() {
	// CreateProcessA(lpApplicationName, lpCommandLine, lpProcessAttributes,
	//                lpThreadAttributes, bInheritHandles, dwCreationFlags,
	//                lpEnvironment, lpCurrentDirectory, lpStartupInfo, lpProcessInformation)
	// 10 args: 8 in regs + 2 on stack
	g.emitSubImm(REG_SP, REG_SP, 32)
	g.emitStr(REG_X28, REG_SP, 0)

	g.emitLoadLocalArm64(1*8, REG_X0)  // lpApplicationName
	g.emitLoadLocalArm64(2*8, REG_X1)  // lpCommandLine
	g.emitMovZ(REG_X2, 0, 0)           // lpProcessAttributes = NULL
	g.emitMovZ(REG_X3, 0, 0)           // lpThreadAttributes = NULL
	g.emitLoadImm64Compact(REG_X4, 1)  // bInheritHandles = TRUE
	g.emitMovZ(REG_X5, 0, 0)           // dwCreationFlags = 0
	g.emitLoadLocalArm64(5*8, REG_X6)  // lpEnvironment
	g.emitMovZ(REG_X7, 0, 0)           // lpCurrentDirectory = NULL

	// Args 9 and 10 go on stack (after the shadow space / spill area)
	g.emitLoadLocalArm64(3*8, REG_X9)  // lpStartupInfo
	g.emitStr(REG_X9, REG_SP, 16)
	g.emitLoadLocalArm64(4*8, REG_X9)  // lpProcessInformation
	g.emitStr(REG_X9, REG_SP, 24)

	g.emitCallIATArm64("CreateProcessA")

	g.emitLdr(REG_X28, REG_SP, 0)
	g.emitAddImm(REG_SP, REG_SP, 32)

	g.emitWinApiReturnArm64(REG_X0)
}

func (g *CodeGen) compileSyscallWaitProcess_winarm64() {
	// WaitForSingleObject(hHandle, INFINITE) then GetExitCodeProcess(hHandle, &exitCode)
	g.emitSubImm(REG_SP, REG_SP, 32)
	g.emitStr(REG_X28, REG_SP, 0)

	g.emitLoadLocalArm64(1*8, REG_X0)               // hProcess
	g.emitLoadImm64Compact(REG_X1, 0xFFFFFFFF)       // INFINITE
	g.emitCallIATArm64("WaitForSingleObject")

	// GetExitCodeProcess(hProcess, &exitCode)
	g.emitLoadLocalArm64(1*8, REG_X0)
	g.emitLoadLocalArm64(2*8, REG_X1) // exitCodeBuf
	g.emitCallIATArm64("GetExitCodeProcess")

	// Read exit code from buffer
	g.emitLoadLocalArm64(2*8, REG_X0)
	g.emitLdr(REG_X0, REG_X0, 0)

	g.emitLdr(REG_X28, REG_SP, 0)
	g.emitAddImm(REG_SP, REG_SP, 32)

	g.rawPush(REG_X0) // r1=exit code
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0) // r2=0
	g.rawPush(REG_X0) // err=0
	g.hasPending = false
}

func (g *CodeGen) compileSyscallCreatePipe_winarm64() {
	// CreatePipe(&hReadPipe, &hWritePipe, lpPipeAttributes, nSize)
	// SECURITY_ATTRIBUTES: 24 bytes on 64-bit
	//  {nLength=4 + pad=4 + lpSecurityDescriptor=8 + bInheritHandle=4 + pad=4}
	g.emitSubImm(REG_SP, REG_SP, 48) // 24 for SECURITY_ATTRIBUTES + 8 for X28 + pad
	g.emitStr(REG_X28, REG_SP, 0)

	// Build SECURITY_ATTRIBUTES at SP+16
	g.emitLoadImm64Compact(REG_X0, 24)
	g.emitStr(REG_X0, REG_SP, 16)     // nLength = 24 (stored as 8 bytes, but only low 4 matter)
	g.emitMovZ(REG_X0, 0, 0)
	g.emitStr(REG_X0, REG_SP, 24)     // lpSecurityDescriptor = NULL
	g.emitLoadImm64Compact(REG_X0, 1)
	g.emitStr(REG_X0, REG_SP, 32)     // bInheritHandle = TRUE (stored as 8 bytes, low 4 matter)

	g.emitLoadLocalArm64(1*8, REG_X0) // &hReadPipe
	g.emitLoadLocalArm64(2*8, REG_X1) // &hWritePipe
	g.emitAddImm(REG_X2, REG_SP, 16)  // lpPipeAttributes
	g.emitMovZ(REG_X3, 0, 0)          // nSize = 0 (default)

	g.emitCallIATArm64("CreatePipe")

	g.emitLdr(REG_X28, REG_SP, 0)
	g.emitAddImm(REG_SP, REG_SP, 48)

	g.emitWinApiReturnArm64(REG_X0)
}

func (g *CodeGen) compileSyscallSetStdHandle_winarm64() {
	// SetStdHandle(nStdHandle, hHandle)
	g.emitLoadLocalArm64(1*8, REG_X0)
	g.emitLoadLocalArm64(2*8, REG_X1)

	g.emitSubImm(REG_SP, REG_SP, 16)
	g.emitStr(REG_X28, REG_SP, 0)
	g.emitCallIATArm64("SetStdHandle")
	g.emitLdr(REG_X28, REG_SP, 0)
	g.emitAddImm(REG_SP, REG_SP, 16)

	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X0)
	g.hasPending = false
}

// compilePanicArm64Windows handles panic on Windows ARM64.
func (g *CodeGen) compilePanicArm64Windows() {
	// Pop value from operand stack
	g.opPop(REG_X0)

	// Tostring heuristic: if [X0] < 256, it's an interface box
	g.emitLdr(REG_X1, REG_X0, 0)
	g.emitCmpImm(REG_X1, 256)
	stringFixup := g.emitBCond(COND_CS) // branch if unsigned >= 256

	// Interface box: extract value
	g.emitLdr(REG_X0, REG_X0, 8)

	g.patchArm64BCondAt(stringFixup, len(g.code))

	// X0 = string header ptr {data_ptr, len}
	// Save on hardware stack
	g.emitSubImm(REG_SP, REG_SP, 32)
	g.emitStr(REG_X0, REG_SP, 0)
	g.emitStr(REG_X28, REG_SP, 8)

	// Load data_ptr and len
	g.emitLdr(REG_X2, REG_X0, 0)  // data_ptr -> save for WriteFile
	g.emitLdr(REG_X3, REG_X0, 8)  // len
	g.emitStr(REG_X2, REG_SP, 16) // save data_ptr
	g.emitStr(REG_X3, REG_SP, 24) // save len

	// GetStdHandle(STD_ERROR_HANDLE = -12)
	g.emitLoadImm64Compact(REG_X0, 0xFFFFFFFFFFFFFFF4) // -12
	g.emitCallIATArm64("GetStdHandle")
	// X0 = stderr handle

	// WriteFile(hFile, lpBuffer, nBytes, &nwritten, NULL)
	g.emitSubImm(REG_SP, REG_SP, 16) // space for nwritten
	g.emitMovRRArm64(REG_X9, REG_X0) // save handle
	g.emitLdr(REG_X1, REG_SP, 32)    // data_ptr (at old SP+16)
	g.emitLdr(REG_X2, REG_SP, 40)    // len (at old SP+24)
	g.emitMovRRArm64(REG_X0, REG_X9) // hFile
	g.emitMovRRArm64(REG_X3, REG_SP) // &nwritten
	g.emitMovZ(REG_X4, 0, 0)         // lpOverlapped
	g.emitCallIATArm64("WriteFile")
	g.emitAddImm(REG_SP, REG_SP, 16) // free nwritten space

	// Write newline
	g.emitLoadImm64Compact(REG_X0, 0x0A)
	g.emitSubImm(REG_SP, REG_SP, 16)
	g.emitStrb(REG_X0, REG_SP, 0)

	// GetStdHandle(STD_ERROR_HANDLE)
	g.emitLoadImm64Compact(REG_X0, 0xFFFFFFFFFFFFFFF4)
	g.emitCallIATArm64("GetStdHandle")
	g.emitMovRRArm64(REG_X9, REG_X0) // save handle

	g.emitSubImm(REG_SP, REG_SP, 16) // nwritten space
	g.emitMovRRArm64(REG_X0, REG_X9) // hFile
	g.emitAddImm(REG_X1, REG_SP, 16) // lpBuffer = &'\n' (at SP+16)
	g.emitLoadImm64Compact(REG_X2, 1) // nBytes = 1
	g.emitMovRRArm64(REG_X3, REG_SP)  // &nwritten
	g.emitMovZ(REG_X4, 0, 0)          // lpOverlapped
	g.emitCallIATArm64("WriteFile")
	g.emitAddImm(REG_SP, REG_SP, 32) // free nwritten + '\n'

	// Restore X28 and stack
	g.emitLdr(REG_X28, REG_SP, 8)
	g.emitAddImm(REG_SP, REG_SP, 32)

	// ExitProcess(2)
	g.emitLoadImm64Compact(REG_X0, 2)
	g.emitCallIATArm64("ExitProcess")
}
