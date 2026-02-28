//go:build windows

package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// Windows constants
const (
	DEBUG_PROCESS           = 0x00000001
	DEBUG_ONLY_THIS_PROCESS = 0x00000002
	INFINITE                = 0xFFFFFFFF

	// Debug event codes
	EXCEPTION_DEBUG_EVENT      = 1
	CREATE_THREAD_DEBUG_EVENT  = 2
	CREATE_PROCESS_DEBUG_EVENT = 3
	EXIT_THREAD_DEBUG_EVENT    = 4
	EXIT_PROCESS_DEBUG_EVENT   = 5
	LOAD_DLL_DEBUG_EVENT       = 6
	UNLOAD_DLL_DEBUG_EVENT     = 7
	OUTPUT_DEBUG_STRING_EVENT  = 8

	// Exception codes
	EXCEPTION_ACCESS_VIOLATION   = 0xC0000005
	EXCEPTION_BREAKPOINT         = 0x80000003
	EXCEPTION_SINGLE_STEP        = 0x80000004
	EXCEPTION_ILLEGAL_INSTRUCTION = 0xC000001D

	// Continue status
	DBG_CONTINUE             = 0x00010002
	DBG_EXCEPTION_NOT_HANDLED = 0x80010001

	// Context flags
	CONTEXT_AMD64             = 0x00100000
	CONTEXT_CONTROL           = CONTEXT_AMD64 | 0x0001
	CONTEXT_INTEGER           = CONTEXT_AMD64 | 0x0002
	CONTEXT_FULL              = CONTEXT_CONTROL | CONTEXT_INTEGER | (CONTEXT_AMD64 | 0x0004)
)

// CONTEXT for x64 — we only care about the general-purpose registers.
// Full struct is 1232 bytes; we allocate enough and read fields at known offsets.
type CONTEXT struct {
	data [1232]byte
}

func (c *CONTEXT) ContextFlags() uint32 { return binary.LittleEndian.Uint32(c.data[48:]) }
func (c *CONTEXT) Rax() uint64          { return binary.LittleEndian.Uint64(c.data[120:]) }
func (c *CONTEXT) Rcx() uint64          { return binary.LittleEndian.Uint64(c.data[128:]) }
func (c *CONTEXT) Rdx() uint64          { return binary.LittleEndian.Uint64(c.data[136:]) }
func (c *CONTEXT) Rbx() uint64          { return binary.LittleEndian.Uint64(c.data[144:]) }
func (c *CONTEXT) Rsp() uint64          { return binary.LittleEndian.Uint64(c.data[152:]) }
func (c *CONTEXT) Rbp() uint64          { return binary.LittleEndian.Uint64(c.data[160:]) }
func (c *CONTEXT) Rsi() uint64          { return binary.LittleEndian.Uint64(c.data[168:]) }
func (c *CONTEXT) Rdi() uint64          { return binary.LittleEndian.Uint64(c.data[176:]) }
func (c *CONTEXT) R8() uint64           { return binary.LittleEndian.Uint64(c.data[184:]) }
func (c *CONTEXT) R9() uint64           { return binary.LittleEndian.Uint64(c.data[192:]) }
func (c *CONTEXT) R10() uint64          { return binary.LittleEndian.Uint64(c.data[200:]) }
func (c *CONTEXT) R11() uint64          { return binary.LittleEndian.Uint64(c.data[208:]) }
func (c *CONTEXT) R12() uint64          { return binary.LittleEndian.Uint64(c.data[216:]) }
func (c *CONTEXT) R13() uint64          { return binary.LittleEndian.Uint64(c.data[224:]) }
func (c *CONTEXT) R14() uint64          { return binary.LittleEndian.Uint64(c.data[232:]) }
func (c *CONTEXT) R15() uint64          { return binary.LittleEndian.Uint64(c.data[240:]) }
func (c *CONTEXT) Rip() uint64          { return binary.LittleEndian.Uint64(c.data[248:]) }

// DEBUG_EVENT structure (partial — we read the raw bytes and decode per event type)
type DEBUG_EVENT struct {
	DebugEventCode uint32
	ProcessId      uint32
	ThreadId       uint32
	_pad           uint32
	// Union follows at offset 16 — up to 160 bytes
	U [160]byte
}

var (
	kernel32                = syscall.NewLazyDLL("kernel32.dll")
	procCreateProcessA      = kernel32.NewProc("CreateProcessA")
	procWaitForDebugEventEx = kernel32.NewProc("WaitForDebugEventEx")
	procWaitForDebugEvent   = kernel32.NewProc("WaitForDebugEvent")
	procContinueDebugEvent  = kernel32.NewProc("ContinueDebugEvent")
	procGetThreadContext    = kernel32.NewProc("GetThreadContext")
	procReadProcessMemory   = kernel32.NewProc("ReadProcessMemory")
	procCloseHandle         = kernel32.NewProc("CloseHandle")
	procTerminateProcess    = kernel32.NewProc("TerminateProcess")
)

func waitForDebugEvent(event *DEBUG_EVENT, ms uint32) bool {
	r, _, _ := procWaitForDebugEvent.Call(
		uintptr(unsafe.Pointer(event)),
		uintptr(ms),
	)
	return r != 0
}

func continueDebugEvent(pid, tid, status uint32) bool {
	r, _, _ := procContinueDebugEvent.Call(
		uintptr(pid), uintptr(tid), uintptr(status),
	)
	return r != 0
}

func getThreadContext(hThread syscall.Handle, ctx *CONTEXT) bool {
	// Set context flags
	binary.LittleEndian.PutUint32(ctx.data[48:], CONTEXT_FULL)
	r, _, _ := procGetThreadContext.Call(
		uintptr(hThread),
		uintptr(unsafe.Pointer(&ctx.data[0])),
	)
	return r != 0
}

func readProcessMemory(hProcess syscall.Handle, addr uint64, buf []byte) int {
	var nRead uintptr
	procReadProcessMemory.Call(
		uintptr(hProcess),
		uintptr(addr),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		uintptr(unsafe.Pointer(&nRead)),
	)
	return int(nRead)
}

type STARTUPINFO struct {
	Cb            uint32
	_             *byte
	_             *byte
	_             *byte
	X             uint32
	Y             uint32
	XSize         uint32
	YSize         uint32
	XCountChars   uint32
	YCountChars   uint32
	FillAttribute uint32
	Flags         uint32
	ShowWindow    uint16
	_             uint16
	_             *byte
	StdInput      syscall.Handle
	StdOutput     syscall.Handle
	StdError      syscall.Handle
}

type PROCESS_INFORMATION struct {
	Process   syscall.Handle
	Thread    syscall.Handle
	ProcessId uint32
	ThreadId  uint32
}

func exceptionCodeName(code uint32) string {
	switch code {
	case EXCEPTION_ACCESS_VIOLATION:
		return "ACCESS_VIOLATION"
	case EXCEPTION_BREAKPOINT:
		return "BREAKPOINT"
	case EXCEPTION_SINGLE_STEP:
		return "SINGLE_STEP"
	case EXCEPTION_ILLEGAL_INSTRUCTION:
		return "ILLEGAL_INSTRUCTION"
	case 0xC0000096:
		return "PRIVILEGED_INSTRUCTION"
	case 0xC000001E:
		return "INVALID_LOCK_SEQUENCE"
	case 0xC0000025:
		return "NONCONTINUABLE_EXCEPTION"
	case 0xC00000FD:
		return "STACK_OVERFLOW"
	default:
		return fmt.Sprintf("0x%08X", code)
	}
}

func dumpBytes(hProcess syscall.Handle, addr uint64, count int) {
	buf := make([]byte, count)
	n := readProcessMemory(hProcess, addr, buf)
	if n == 0 {
		fmt.Printf("  (cannot read memory at 0x%x)\n", addr)
		return
	}
	for i := 0; i < n; i += 16 {
		fmt.Printf("  %016x: ", addr+uint64(i))
		end := i + 16
		if end > n {
			end = n
		}
		for j := i; j < end; j++ {
			fmt.Printf("%02x ", buf[j])
		}
		fmt.Println()
	}
}

func dumpContext(ctx *CONTEXT) {
	fmt.Printf("  RAX=%016x  RBX=%016x  RCX=%016x  RDX=%016x\n", ctx.Rax(), ctx.Rbx(), ctx.Rcx(), ctx.Rdx())
	fmt.Printf("  RSI=%016x  RDI=%016x  RBP=%016x  RSP=%016x\n", ctx.Rsi(), ctx.Rdi(), ctx.Rbp(), ctx.Rsp())
	fmt.Printf("  R8 =%016x  R9 =%016x  R10=%016x  R11=%016x\n", ctx.R8(), ctx.R9(), ctx.R10(), ctx.R11())
	fmt.Printf("  R12=%016x  R13=%016x  R14=%016x  R15=%016x\n", ctx.R12(), ctx.R13(), ctx.R14(), ctx.R15())
	fmt.Printf("  RIP=%016x\n", ctx.Rip())
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: win32debug <exe> [args...]\n")
		os.Exit(1)
	}

	exePath := os.Args[1]
	cmdLine := exePath
	for i := 2; i < len(os.Args); i++ {
		cmdLine += " " + os.Args[i]
	}

	// Convert to C strings
	exePathC, _ := syscall.BytePtrFromString(exePath)
	cmdLineC, _ := syscall.BytePtrFromString(cmdLine)

	var si STARTUPINFO
	si.Cb = uint32(unsafe.Sizeof(si))
	var pi PROCESS_INFORMATION

	r, _, err := procCreateProcessA.Call(
		uintptr(unsafe.Pointer(exePathC)),
		uintptr(unsafe.Pointer(cmdLineC)),
		0, 0, 0,
		DEBUG_PROCESS|DEBUG_ONLY_THIS_PROCESS,
		0, 0,
		uintptr(unsafe.Pointer(&si)),
		uintptr(unsafe.Pointer(&pi)),
	)
	if r == 0 {
		fmt.Fprintf(os.Stderr, "CreateProcess failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Process %d started (thread %d)\n", pi.ProcessId, pi.ThreadId)

	// Track thread handles for GetThreadContext
	threads := make(map[uint32]syscall.Handle)
	threads[pi.ThreadId] = pi.Thread
	hProcess := pi.Process

	var imageBase uint64
	firstBreakpoint := true

	var event DEBUG_EVENT
	for {
		if !waitForDebugEvent(&event, INFINITE) {
			fmt.Fprintf(os.Stderr, "WaitForDebugEvent failed\n")
			break
		}

		continueStatus := uint32(DBG_CONTINUE)

		switch event.DebugEventCode {
		case CREATE_PROCESS_DEBUG_EVENT:
			// Offsets into CREATE_PROCESS_DEBUG_INFO:
			// 0: hFile, 8: hProcess, 16: hThread, 24: lpBaseOfImage, ...
			imageBase = binary.LittleEndian.Uint64(event.U[24:])
			fmt.Printf("Process created, image base: 0x%x\n", imageBase)
			// Close hFile if non-zero
			hFile := syscall.Handle(binary.LittleEndian.Uint64(event.U[0:]))
			if hFile != 0 && hFile != syscall.InvalidHandle {
				procCloseHandle.Call(uintptr(hFile))
			}

		case CREATE_THREAD_DEBUG_EVENT:
			hThread := syscall.Handle(binary.LittleEndian.Uint64(event.U[0:]))
			threads[event.ThreadId] = hThread

		case EXIT_THREAD_DEBUG_EVENT:
			delete(threads, event.ThreadId)

		case EXIT_PROCESS_DEBUG_EVENT:
			exitCode := binary.LittleEndian.Uint32(event.U[0:])
			if exitCode == 0 {
				fmt.Printf("Process exited normally (code 0)\n")
			} else {
				fmt.Printf("Process exited with code 0x%x (%d)\n", exitCode, exitCode)
			}
			os.Exit(0)

		case EXCEPTION_DEBUG_EVENT:
			// EXCEPTION_RECORD at event.U[0:]
			// 0: ExceptionCode (u32), 4: ExceptionFlags (u32), 8: ExceptionRecord (ptr),
			// 16: ExceptionAddress (ptr), 24: NumberParameters (u32), 32: ExceptionInformation[0..14] (ptr each)
			excCode := binary.LittleEndian.Uint32(event.U[0:])
			excAddr := binary.LittleEndian.Uint64(event.U[16:])
			firstChance := binary.LittleEndian.Uint32(event.U[0x90:]) // after EXCEPTION_RECORD in DEBUG_EVENT (at fixed known offset? Actually, the u union layout differs)

			_ = firstChance

			if excCode == EXCEPTION_BREAKPOINT && firstBreakpoint {
				firstBreakpoint = false
				fmt.Printf("Initial breakpoint hit at 0x%x\n", excAddr)
				break // continue normally
			}

			fmt.Printf("\n=== EXCEPTION: %s at 0x%x ===\n", exceptionCodeName(excCode), excAddr)
			if imageBase != 0 {
				fmt.Printf("  Code offset from image base: 0x%x\n", excAddr-imageBase)
			}

			if excCode == EXCEPTION_ACCESS_VIOLATION {
				nParams := binary.LittleEndian.Uint32(event.U[24:])
				if nParams >= 2 {
					accessType := binary.LittleEndian.Uint64(event.U[32:])
					accessAddr := binary.LittleEndian.Uint64(event.U[40:])
					typeStr := "read"
					if accessType == 1 {
						typeStr = "write"
					} else if accessType == 8 {
						typeStr = "execute"
					}
					fmt.Printf("  %s at address 0x%x\n", typeStr, accessAddr)
				}
			}

			// Dump thread context
			hThread, ok := threads[event.ThreadId]
			if ok {
				var ctx CONTEXT
				if getThreadContext(hThread, &ctx) {
					dumpContext(&ctx)
					// Dump code bytes at RIP
					fmt.Printf("  Code at RIP:\n")
					dumpBytes(hProcess, ctx.Rip(), 32)
					// Dump stack
					fmt.Printf("  Stack at RSP:\n")
					dumpBytes(hProcess, ctx.Rsp(), 64)
				}
			}

			// Don't continue — let the process crash
			continueStatus = DBG_EXCEPTION_NOT_HANDLED

		case LOAD_DLL_DEBUG_EVENT:
			dllBase := binary.LittleEndian.Uint64(event.U[8:])
			hFile := syscall.Handle(binary.LittleEndian.Uint64(event.U[0:]))
			// Try to read the DLL name
			namePtr := binary.LittleEndian.Uint64(event.U[24:])
			dllName := ""
			if namePtr != 0 {
				// namePtr is a pointer in the target process to a pointer to the name string
				var nameAddr [8]byte
				if readProcessMemory(hProcess, namePtr, nameAddr[:]) == 8 {
					actualNameAddr := binary.LittleEndian.Uint64(nameAddr[:])
					if actualNameAddr != 0 {
						isUnicode := binary.LittleEndian.Uint32(event.U[32:])
						nameBuf := make([]byte, 512)
						n := readProcessMemory(hProcess, actualNameAddr, nameBuf)
						if n > 0 {
							if isUnicode != 0 {
								// UTF-16LE to ASCII (simple)
								for i := 0; i+1 < n; i += 2 {
									ch := binary.LittleEndian.Uint16(nameBuf[i:])
									if ch == 0 {
										break
									}
									if ch < 128 {
										dllName += string(rune(ch))
									} else {
										dllName += "?"
									}
								}
							} else {
								for i := 0; i < n; i++ {
									if nameBuf[i] == 0 {
										break
									}
									dllName += string(rune(nameBuf[i]))
								}
							}
						}
					}
				}
			}
			if dllName != "" {
				fmt.Printf("DLL loaded: %s at 0x%x\n", dllName, dllBase)
			}
			if hFile != 0 && hFile != syscall.InvalidHandle {
				procCloseHandle.Call(uintptr(hFile))
			}

		case UNLOAD_DLL_DEBUG_EVENT:
			// ignore

		case OUTPUT_DEBUG_STRING_EVENT:
			// ignore
		}

		continueDebugEvent(event.ProcessId, event.ThreadId, continueStatus)
	}

	procTerminateProcess.Call(uintptr(hProcess), 1)
	procCloseHandle.Call(uintptr(hProcess))
	procCloseHandle.Call(uintptr(pi.Thread))
}
