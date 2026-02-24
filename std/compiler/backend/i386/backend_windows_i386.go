//go:build !no_backend_windows_i386

package i386

import (
	"fmt"
	"os"

	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

// GenerateWinPE compiles an IRModule to a Windows PE32 executable.
func GenerateWinPE(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	g := &CodeGen{
		target:        target,
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

	ir.CollectNativeFuncSizes(irmod, g.funcOffsets, len(g.code))
	if g.needTostringHelper {
		g.emitTostringHelperI386()
	}

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
	pe := g.buildPE32(irmod)
	err := os.WriteFile(outputPath, pe, 0755)
	if err != nil {
		return fmt.Errorf("write output: %v", err)
	}

	return nil
}

// emitStart_win386 generates the Windows entry point.
func (g *CodeGen) emitStart_win386(irmod *ir.IRModule) {
	// Windows entry point: no arguments passed, we use stdcall.
	// EDI = operand stack pointer (callee-saved)
	// EBP = frame pointer (callee-saved)

	// Save callee-saved registers
	g.pushR32(REG32_EBX)
	g.pushR32(REG32_ESI)

	// Allocate 16MB operand stack via VirtualAlloc
	// VirtualAlloc(NULL, 16*1048576, MEM_COMMIT|MEM_RESERVE, PAGE_READWRITE)
	// stdcall: push args right-to-left, callee cleans stack
	g.pushImm32(0x04)         // PAGE_READWRITE
	g.pushImm32(0x3000)       // MEM_COMMIT | MEM_RESERVE
	g.pushImm32(16 * 1048576) // dwSize = 16MB
	g.pushImm32(0)            // lpAddress = NULL
	g.emitCallIAT("VirtualAlloc")
	// EAX = base of allocation

	// EDI = EAX + 16MB (top of operand stack, grows down)
	g.movRR32(REG32_EDI, REG32_EAX)
	g.addRI32(REG32_EDI, int32(16*1048576))

	// Call init functions
	for _, f := range irmod.Funcs {
		if ir.IsInitFunc(f.Name) {
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
	g.emitBytes(0x6a, 0x0a)         // push 0x0a
	g.movRR32(REG32_ESI, REG32_ESP) // ESI = &'\n'

	// GetStdHandle(STD_ERROR_HANDLE)
	g.emitMovRegImm32(REG32_EAX, 0xFFFFFFF4)
	g.pushR32(REG32_EAX)
	g.emitCallIAT("GetStdHandle")
	g.movRR32(REG32_ECX, REG32_EAX) // stderr handle

	g.subRI32(REG32_ESP, 4) // written_space
	g.movRR32(REG32_EDX, REG32_ESP)

	g.pushImm32(0)       // lpOverlapped
	g.pushR32(REG32_EDX) // &written
	g.pushImm32(1)       // nBytes = 1
	g.pushR32(REG32_ESI) // lpBuffer = &'\n'
	g.pushR32(REG32_ECX) // hFile

	g.emitCallIAT("WriteFile")
	g.addRI32(REG32_ESP, 8) // clean written_space(4) + '\n' slot(4)

	// Restore callee-saved
	g.popR32(REG32_EBX)
	g.popR32(REG32_ESI)

	// ExitProcess(2)
	g.pushImm32(2)
	g.emitCallIAT("ExitProcess")
}
