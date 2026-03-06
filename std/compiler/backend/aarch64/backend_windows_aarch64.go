//go:build !no_backend_arm64

package aarch64

import (
	"fmt"
	"os"

	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

// GenerateWinPE compiles an IRModule to a Windows ARM64 PE32+ executable.
func GenerateWinPE(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	g := &CodeGen{
		target:        target,
		funcOffsets:   make(map[string]int),
		labelOffsets:  make(map[int]int),
		stringMap:     make(map[string]int),
		globalOffsets: make([]int, len(irmod.Globals)),
		baseAddr:      0x140000000,
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
	g.emitStartArm64Windows(irmod, common.EntryFuncName(target))

	// Compile all functions
	for _, f := range irmod.Funcs {
		g.funcOffsets[f.Name] = len(g.code)
		g.CompileFuncArm64(f)
	}

	ir.CollectNativeFuncSizes(irmod, g.funcOffsets, len(g.code))
	if g.needTostringHelper {
		g.EmitTostringHelperArm64()
	}

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
		g.PatchArm64BAt(fix.CodeOffset, target)
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
	pe := g.buildPE64(irmod)
	err := os.WriteFile(outputPath, pe, 0755)
	if err != nil {
		return fmt.Errorf("write output: %v", err)
	}

	return nil
}

// emitStartArm64Windows generates the Windows ARM64 entry point.
func (g *CodeGen) emitStartArm64Windows(irmod *ir.IRModule, entryFunc string) {
	// Save LR (entry is called by Windows loader)
	g.EmitStp(REG_FP, REG_LR, REG_SP, -16)
	g.EmitMovRRArm64(REG_FP, REG_SP)

	emitDebugMarkerArm64(g, 'A')

	// Allocate 16MB operand stack via VirtualAlloc
	// VirtualAlloc(NULL, 16*1048576, MEM_COMMIT|MEM_RESERVE, PAGE_READWRITE)
	g.EmitMovZ(REG_X0, 0, 0)                   // lpAddress = NULL
	g.EmitLoadImm64Compact(REG_X1, 16*1048576) // dwSize = 16MB
	g.EmitLoadImm64Compact(REG_X2, 0x3000)     // MEM_COMMIT | MEM_RESERVE
	g.EmitLoadImm64Compact(REG_X3, 0x04)       // PAGE_READWRITE
	g.emitCallIATArm64("VirtualAlloc")

	emitDebugMarkerArm64(g, 'B')

	// X28 = result + 16MB (operand stack top, grows down)
	g.EmitLoadImm64Compact(REG_X1, 16*1048576)
	g.EmitAddRR(REG_X28, REG_X0, REG_X1)

	// Call init functions
	for _, f := range irmod.Funcs {
		if ir.IsInitFunc(f.Name) {
			g.EmitCallPlaceholderArm64(f.Name)
		}
	}

	emitDebugMarkerArm64(g, 'C')

	// Call entry function.
	g.EmitCallPlaceholderArm64(entryFunc)

	emitDebugMarkerArm64(g, 'D')

	// ExitProcess(0)
	g.EmitMovZ(REG_X0, 0, 0)
	g.emitCallIATArm64("ExitProcess")

	// Epilogue (won't reach here)
	g.EmitMovRRArm64(REG_SP, REG_FP)
	g.EmitLdp(REG_FP, REG_LR, REG_SP, 16)
	g.EmitRet()
}

// emitCallIATArm64 emits ADRP+LDR X16 (placeholder) then BLR X16 for calling
// a Windows IAT entry. Creates a $iat$funcName callFixup.
func (g *CodeGen) emitCallIATArm64(funcName string) {
	g.emitCallIATArm64InLib(winDefaultImportLibraryArm64, funcName)
}

func (g *CodeGen) emitCallIATArm64InLib(libName string, funcName string) {
	g.Flush()
	// Windows ARM64 ABI requires 32 bytes of home space for callees.
	g.emitSubImm(REG_SP, REG_SP, 32)
	off := g.EmitAdrp(REG_X16)
	// LDR X16, [X16, #0] — placeholder
	inst := uint32(0xF9400000) | (uint32(REG_X16&0x1f) << 5) | uint32(REG_X16&0x1f)
	g.EmitArm64(inst)
	g.callFixups = append(g.callFixups, CallFixup{
		CodeOffset: off,
		Target:     encodeIATFixupTargetArm64(libName, funcName),
	})
	g.EmitBlr(REG_X16)
	g.emitAddImm(REG_SP, REG_SP, 32)
}

func emitDebugMarkerArm64(g *CodeGen, marker byte) {
	if !g.target.CompilerDebug {
		return
	}

	// WriteFile(GetStdHandle(STD_ERROR_HANDLE), &marker, 2, &nwritten, NULL)
	g.emitSubImm(REG_SP, REG_SP, 32)
	val := uint64(marker) | (uint64('\n') << 8)
	g.EmitLoadImm64Compact(REG_X1, val)
	g.EmitStr(REG_X1, REG_SP, 0)

	g.EmitLoadImm64Compact(REG_X0, 0xFFFFFFFFFFFFFFF4) // -12
	g.emitCallIATArm64("GetStdHandle")

	g.EmitMovRRArm64(REG_X1, REG_SP)
	g.EmitLoadImm64Compact(REG_X2, 2)
	g.emitAddImm(REG_X3, REG_SP, 16)
	g.EmitMovZ(REG_X4, 0, 0)
	g.emitCallIATArm64("WriteFile")

	g.emitAddImm(REG_SP, REG_SP, 32)
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
	g.EmitLoadImm64Compact(REG_X1, 0xFFFFFFFFFFFFFFF6) // -10
	g.EmitAddRR(REG_X0, REG_X0, REG_X1)                // X0 = -10 - fd

	// Save X28 on machine stack
	g.emitSubImm(REG_SP, REG_SP, 16)
	g.EmitStr(REG_X28, REG_SP, 0)

	g.emitCallIATArm64("GetStdHandle")
	// X0 = handle

	// Restore X28
	g.emitLdr(REG_X28, REG_SP, 0)
	g.emitAddImm(REG_SP, REG_SP, 16)

	fixDone := g.emitB()

	g.patchArm64BCondAt(fixNotStd, len(g.code))
	// fd > 2: use as-is
	g.emitLoadLocalArm64(localOffset, REG_X0)

	g.PatchArm64BAt(fixDone, len(g.code))
}

// emitWinApiReturnArm64 checks return value (nonzero=success) and pushes (r1, r2, err) triple.
// On success: r1=successReg, r2=0, err=0
// On failure: r1=0, r2=0, err=GetLastError()
func (g *CodeGen) emitWinApiReturnArm64(successReg int) {
	g.Flush()
	g.emitTstRR(REG_X0, REG_X0)
	fixOk := g.emitBCond(COND_NE)

	// Failed: GetLastError
	g.emitSubImm(REG_SP, REG_SP, 16)
	g.EmitStr(REG_X28, REG_SP, 0)
	g.emitCallIATArm64("GetLastError")
	g.emitLdr(REG_X28, REG_SP, 0)
	g.emitAddImm(REG_SP, REG_SP, 16)

	g.EmitMovRRArm64(REG_X1, REG_X0) // save error
	g.EmitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0) // r1=0
	g.rawPush(REG_X0) // r2=0
	g.rawPush(REG_X1) // err
	fixDone := g.emitB()

	g.patchArm64BCondAt(fixOk, len(g.code))
	// Success
	g.rawPush(successReg)
	g.EmitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0) // r2=0
	g.rawPush(REG_X0) // err=0

	g.PatchArm64BAt(fixDone, len(g.code))
	g.ClearOperandCache()
}

// === Intrinsic dispatcher ===

func (g *CodeGen) compileCallIntrinsicArm64Windows(inst ir.Inst) {
	g.Flush()
	if g.compileLinkStaticIntrinsicArm64Windows(inst) {
		return
	}
	switch inst.Name {
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
	g.EmitStr(REG_X0, REG_SP, 0)
	g.EmitStr(REG_X28, REG_SP, 8)

	// Load data_ptr and len
	g.emitLdr(REG_X2, REG_X0, 0)  // data_ptr -> save for WriteFile
	g.emitLdr(REG_X3, REG_X0, 8)  // len
	g.EmitStr(REG_X2, REG_SP, 16) // save data_ptr
	g.EmitStr(REG_X3, REG_SP, 24) // save len

	// GetStdHandle(STD_ERROR_HANDLE = -12)
	g.EmitLoadImm64Compact(REG_X0, 0xFFFFFFFFFFFFFFF4) // -12
	g.emitCallIATArm64("GetStdHandle")
	// X0 = stderr handle

	// WriteFile(hFile, lpBuffer, nBytes, &nwritten, NULL)
	g.emitSubImm(REG_SP, REG_SP, 16) // space for nwritten
	g.EmitMovRRArm64(REG_X9, REG_X0) // save handle
	g.emitLdr(REG_X1, REG_SP, 32)    // data_ptr (at old SP+16)
	g.emitLdr(REG_X2, REG_SP, 40)    // len (at old SP+24)
	g.EmitMovRRArm64(REG_X0, REG_X9) // hFile
	g.EmitMovRRArm64(REG_X3, REG_SP) // &nwritten
	g.EmitMovZ(REG_X4, 0, 0)         // lpOverlapped
	g.emitCallIATArm64("WriteFile")
	g.emitAddImm(REG_SP, REG_SP, 16) // free nwritten space

	// Write newline
	g.EmitLoadImm64Compact(REG_X0, 0x0A)
	g.emitSubImm(REG_SP, REG_SP, 16)
	g.emitStrb(REG_X0, REG_SP, 0)

	// GetStdHandle(STD_ERROR_HANDLE)
	g.EmitLoadImm64Compact(REG_X0, 0xFFFFFFFFFFFFFFF4)
	g.emitCallIATArm64("GetStdHandle")
	g.EmitMovRRArm64(REG_X9, REG_X0) // save handle

	g.emitSubImm(REG_SP, REG_SP, 16)  // nwritten space
	g.EmitMovRRArm64(REG_X0, REG_X9)  // hFile
	g.emitAddImm(REG_X1, REG_SP, 16)  // lpBuffer = &'\n' (at SP+16)
	g.EmitLoadImm64Compact(REG_X2, 1) // nBytes = 1
	g.EmitMovRRArm64(REG_X3, REG_SP)  // &nwritten
	g.EmitMovZ(REG_X4, 0, 0)          // lpOverlapped
	g.emitCallIATArm64("WriteFile")
	g.emitAddImm(REG_SP, REG_SP, 32) // free nwritten + '\n'

	// Restore X28 and stack
	g.emitLdr(REG_X28, REG_SP, 8)
	g.emitAddImm(REG_SP, REG_SP, 32)

	// ExitProcess(2)
	g.EmitLoadImm64Compact(REG_X0, 2)
	g.emitCallIATArm64("ExitProcess")
}
