//go:build !no_backend_darwin_arm64

package main

import (
	"fmt"
	"os"
)

// darwinArm64Imports lists all libSystem.B.dylib functions needed by the backend.
var darwinArm64Imports = []string{
	"_write",
	"_read",
	"_open",
	"_close",
	"_mmap",
	"_exit",
	"_mkdir",
	"_rmdir",
	"_unlink",
	"_stat",
	"_getcwd",
	"_opendir",
	"_readdir",
	"_closedir",
	"_dup2",
	"_fork",
	"_execve",
	"_wait4",
	"_pipe",
	"_chmod",
}

// generateDarwinArm64 compiles an IRModule to a macOS ARM64 Mach-O binary.
func generateDarwinArm64(irmod *IRModule, outputPath string) error {
	g := &CodeGen{
		funcOffsets:   make(map[string]int),
		labelOffsets:  make(map[int]int),
		stringMap:     make(map[string]int),
		globalOffsets: make([]int, len(irmod.Globals)),
		baseAddr:      0x100000000, // standard macOS arm64 VM base
		irmod:         irmod,
		wordSize:      8,
		isArm64:       true,
		gotEntries:    make(map[string]int),
	}

	// Allocate .data space for globals (8 bytes each)
	// Reserve 3 extra globals for argc, argv, envp
	totalGlobals := len(irmod.Globals) + 3
	for i := range irmod.Globals {
		g.globalOffsets[i] = i * 8
	}
	g.data = make([]byte, totalGlobals*8)

	// Pre-register all GOT symbols
	for _, sym := range darwinArm64Imports {
		g.gotSlot(sym)
	}

	// Emit entry point
	g.emitStartArm64(irmod)

	// Compile all functions
	for _, f := range irmod.Funcs {
		g.funcOffsets[f.Name] = len(g.code)
		g.compileFuncArm64(f)
	}

	collectNativeFuncSizes(irmod, g.funcOffsets, len(g.code))

	// Resolve call fixups (skip special targets handled by buildMachO64)
	var unresolved []string
	for _, fix := range g.callFixups {
		if fix.Target == "$rodata_header$" || fix.Target == "$data_addr$" || fix.Target == "$got_addr$" {
			continue
		}
		target, ok := g.funcOffsets[fix.Target]
		if !ok {
			unresolved = append(unresolved, fix.Target)
			continue
		}
		// ARM64 BL fixup: 26-bit signed offset in 4-byte units
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

	// Extract basename for code signature identifier
	binName := outputPath
	lastSlash := -1
	si := 0
	for si < len(outputPath) {
		if outputPath[si] == '/' {
			lastSlash = si
		}
		si++
	}
	if lastSlash >= 0 {
		binName = outputPath[lastSlash+1:]
	}

	// Build Mach-O binary
	macho := g.buildMachO64(irmod, binName)
	err := os.WriteFile(outputPath, macho, 0755)
	if err != nil {
		return fmt.Errorf("write output: %v", err)
	}

	// Fix permissions (workaround for _open mode issue in arm64 backend)
	os.Chmod(outputPath, 0755)

	return nil
}

// emitStartArm64 generates the entry point for macOS ARM64.
// LC_MAIN receives: X0=argc, X1=argv, X2=envp (as a C function call)
func (g *CodeGen) emitStartArm64(irmod *IRModule) {
	// Save LR (we're called as a function by dyld)
	g.emitStp(REG_FP, REG_LR, REG_SP, -16)
	g.emitMovRRArm64(REG_FP, REG_SP)

	// Save argc, argv, envp to globals (at end of data section)
	argcGlobalOff := len(irmod.Globals) * 8
	argvGlobalOff := (len(irmod.Globals) + 1) * 8
	envpGlobalOff := (len(irmod.Globals) + 2) * 8

	// Save X0 (argc) to global
	g.emitAdrpAdd(REG_X3, "$data_addr$", uint64(argcGlobalOff))
	g.emitStr(REG_X0, REG_X3, 0)

	// Save X1 (argv) to global
	g.emitAdrpAdd(REG_X3, "$data_addr$", uint64(argvGlobalOff))
	g.emitStr(REG_X1, REG_X3, 0)

	// Save X2 (envp) to global
	g.emitAdrpAdd(REG_X3, "$data_addr$", uint64(envpGlobalOff))
	g.emitStr(REG_X2, REG_X3, 0)

	// Allocate operand stack: mmap(NULL, 1MB, PROT_READ|PROT_WRITE, MAP_PRIVATE|MAP_ANON, -1, 0)
	// macOS: MAP_ANONYMOUS = 0x1000, MAP_PRIVATE = 0x02 → flags = 0x1002
	g.emitMovZ(REG_X0, 0, 0)                                   // addr = NULL
	g.emitLoadImm64Compact(REG_X1, 1048576)                     // len = 1MB
	g.emitLoadImm64Compact(REG_X2, 3)                           // PROT_READ|PROT_WRITE
	g.emitLoadImm64Compact(REG_X3, 0x1002)                      // MAP_PRIVATE|MAP_ANON
	g.emitLoadImm64Compact(REG_X4, 0xFFFFFFFFFFFFFFFF)          // fd = -1
	g.emitMovZ(REG_X5, 0, 0)                                   // offset = 0
	g.emitCallGOT("_mmap")

	// X28 = mmap result + 1MB (top of operand stack, grows down)
	g.emitLoadImm64Compact(REG_X1, 1048576)
	g.emitAddRR(REG_X28, REG_X0, REG_X1)

	// Call init functions in topological order
	for _, f := range irmod.Funcs {
		if isInitFunc(f.Name) {
			g.emitCallPlaceholderArm64(f.Name)
		}
	}

	// Call main.main
	g.emitCallPlaceholderArm64("main.main")

	// exit(0)
	g.emitMovZ(REG_X0, 0, 0)
	g.emitCallGOT("_exit")

	// Epilogue (won't reach here, but keep it clean)
	g.emitMovRRArm64(REG_SP, REG_FP)
	g.emitLdp(REG_FP, REG_LR, REG_SP, 16)
	g.emitRet()
}

// compileSyscallIntrinsicArm64 dispatches pseudo-syscall numbers to
// libSystem calls via the GOT.
func (g *CodeGen) compileSyscallIntrinsicArm64(paramCount int) {
	// Load syscall number from local 0 (offset 1*8 = [FP-8])
	g.emitLoadLocalArm64(1*8, REG_X0)

	// Dispatch via cmp/b.ne chain.
	// IMPORTANT: g.hasPending must be false at each case boundary to prevent
	// the pending-push optimization from bleeding across branches.

	// SYS_READ (0) → _read(fd, buf, count)
	g.emitCmpImm(REG_X0, 0)
	fixRead := g.emitBCond(COND_NE)
	g.emitLoadLocalArm64(2*8, REG_X0) // fd
	g.emitLoadLocalArm64(3*8, REG_X1) // buf
	g.emitLoadLocalArm64(4*8, REG_X2) // count
	g.emitCallGOT("_read")
	g.emitSyscallReturnArm64()
	skipRead := g.emitB()
	g.patchArm64BCondAt(fixRead, len(g.code))
	g.hasPending = false

	// SYS_WRITE (1) → _write(fd, buf, count)
	g.emitCmpImm(REG_X0, 1)
	fixWrite := g.emitBCond(COND_NE)
	g.emitLoadLocalArm64(2*8, REG_X0)
	g.emitLoadLocalArm64(3*8, REG_X1)
	g.emitLoadLocalArm64(4*8, REG_X2)
	g.emitCallGOT("_write")
	g.emitSyscallReturnArm64()
	skipWrite := g.emitB()
	g.patchArm64BCondAt(fixWrite, len(g.code))
	g.hasPending = false

	// SYS_OPEN (2) → _open(path, flags, mode)
	g.emitCmpImm(REG_X0, 2)
	fixOpen := g.emitBCond(COND_NE)
	g.emitLoadLocalArm64(2*8, REG_X0)
	g.emitLoadLocalArm64(3*8, REG_X1)
	g.emitLoadLocalArm64(4*8, REG_X2)
	g.emitCallGOT("_open")
	g.emitSyscallReturnArm64()
	skipOpen := g.emitB()
	g.patchArm64BCondAt(fixOpen, len(g.code))
	g.hasPending = false

	// SYS_CLOSE (3) → _close(fd)
	g.emitCmpImm(REG_X0, 3)
	fixClose := g.emitBCond(COND_NE)
	g.emitLoadLocalArm64(2*8, REG_X0)
	g.emitCallGOT("_close")
	g.emitSyscallReturnArm64()
	skipClose := g.emitB()
	g.patchArm64BCondAt(fixClose, len(g.code))
	g.hasPending = false

	// SYS_STAT (4) → _stat(path, buf)
	g.emitCmpImm(REG_X0, 4)
	fixStat := g.emitBCond(COND_NE)
	g.emitLoadLocalArm64(2*8, REG_X0)
	g.emitLoadLocalArm64(3*8, REG_X1)
	g.emitCallGOT("_stat")
	g.emitSyscallReturnArm64()
	skipStat := g.emitB()
	g.patchArm64BCondAt(fixStat, len(g.code))
	g.hasPending = false

	// SYS_MKDIR (5) → _mkdir(path, mode)
	g.emitCmpImm(REG_X0, 5)
	fixMkdir := g.emitBCond(COND_NE)
	g.emitLoadLocalArm64(2*8, REG_X0)
	g.emitLoadLocalArm64(3*8, REG_X1)
	g.emitCallGOT("_mkdir")
	g.emitSyscallReturnArm64()
	skipMkdir := g.emitB()
	g.patchArm64BCondAt(fixMkdir, len(g.code))
	g.hasPending = false

	// SYS_RMDIR (6) → _rmdir(path)
	g.emitCmpImm(REG_X0, 6)
	fixRmdir := g.emitBCond(COND_NE)
	g.emitLoadLocalArm64(2*8, REG_X0)
	g.emitCallGOT("_rmdir")
	g.emitSyscallReturnArm64()
	skipRmdir := g.emitB()
	g.patchArm64BCondAt(fixRmdir, len(g.code))
	g.hasPending = false

	// SYS_UNLINK (7) → _unlink(path)
	g.emitCmpImm(REG_X0, 7)
	fixUnlink := g.emitBCond(COND_NE)
	g.emitLoadLocalArm64(2*8, REG_X0)
	g.emitCallGOT("_unlink")
	g.emitSyscallReturnArm64()
	skipUnlink := g.emitB()
	g.patchArm64BCondAt(fixUnlink, len(g.code))
	g.hasPending = false

	// SYS_GETCWD (8) → _getcwd(buf, size)
	g.emitCmpImm(REG_X0, 8)
	fixGetcwd := g.emitBCond(COND_NE)
	g.emitLoadLocalArm64(2*8, REG_X0)
	g.emitLoadLocalArm64(3*8, REG_X1)
	g.emitCallGOT("_getcwd")
	g.emitSyscallReturnPtrArm64()
	skipGetcwd := g.emitB()
	g.patchArm64BCondAt(fixGetcwd, len(g.code))
	g.hasPending = false

	// SYS_EXIT_GROUP (9) → _exit(code)
	g.emitCmpImm(REG_X0, 9)
	fixExit := g.emitBCond(COND_NE)
	g.emitLoadLocalArm64(2*8, REG_X0)
	g.emitCallGOT("_exit")
	// _exit doesn't return, but push dummy results using rawPush
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X0)
	g.rawPush(REG_X0)
	skipExit := g.emitB()
	g.patchArm64BCondAt(fixExit, len(g.code))
	g.hasPending = false

	// SYS_MMAP (10) → _mmap(addr, len, prot, flags, fd, offset)
	g.emitCmpImm(REG_X0, 10)
	fixMmap := g.emitBCond(COND_NE)
	g.emitLoadLocalArm64(2*8, REG_X0) // addr
	g.emitLoadLocalArm64(3*8, REG_X1) // len
	g.emitLoadLocalArm64(4*8, REG_X2) // prot
	g.emitLoadLocalArm64(5*8, REG_X3) // flags
	g.emitLoadLocalArm64(6*8, REG_X4) // fd
	g.emitLoadLocalArm64(7*8, REG_X5) // offset
	g.emitCallGOT("_mmap")
	g.emitSyscallReturnPtrArm64()
	skipMmap := g.emitB()
	g.patchArm64BCondAt(fixMmap, len(g.code))
	g.hasPending = false

	// SYS_OPENDIR (14) → _opendir(path)
	g.emitCmpImm(REG_X0, 14)
	fixOpendir := g.emitBCond(COND_NE)
	g.emitLoadLocalArm64(2*8, REG_X0)
	g.emitCallGOT("_opendir")
	g.emitSyscallReturnPtrArm64()
	skipOpendir := g.emitB()
	g.patchArm64BCondAt(fixOpendir, len(g.code))
	g.hasPending = false

	// SYS_READDIR (15) → _readdir(DIR*)
	g.emitCmpImm(REG_X0, 15)
	fixReaddir := g.emitBCond(COND_NE)
	g.emitLoadLocalArm64(2*8, REG_X0)
	g.emitCallGOT("_readdir")
	// readdir returns ptr or NULL (not an error per se)
	// Use rawPush to avoid pending-push issues
	g.rawPush(REG_X0) // r1 = dirent* or 0
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0) // r2=0
	g.rawPush(REG_X0) // err=0
	skipReaddir := g.emitB()
	g.patchArm64BCondAt(fixReaddir, len(g.code))
	g.hasPending = false

	// SYS_CLOSEDIR (16) → _closedir(DIR*)
	g.emitCmpImm(REG_X0, 16)
	fixClosedir := g.emitBCond(COND_NE)
	g.emitLoadLocalArm64(2*8, REG_X0)
	g.emitCallGOT("_closedir")
	g.emitSyscallReturnArm64()
	skipClosedir := g.emitB()
	g.patchArm64BCondAt(fixClosedir, len(g.code))
	g.hasPending = false

	// SYS_DUP2 (17) → _dup2(oldfd, newfd)
	g.emitCmpImm(REG_X0, 17)
	fixDup2 := g.emitBCond(COND_NE)
	g.emitLoadLocalArm64(2*8, REG_X0) // oldfd
	g.emitLoadLocalArm64(3*8, REG_X1) // newfd
	g.emitCallGOT("_dup2")
	g.emitSyscallReturnArm64()
	skipDup2 := g.emitB()
	g.patchArm64BCondAt(fixDup2, len(g.code))
	g.hasPending = false

	// SYS_FORK (18) → _fork()
	g.emitCmpImm(REG_X0, 18)
	fixFork := g.emitBCond(COND_NE)
	g.emitCallGOT("_fork")
	g.emitSyscallReturnArm64()
	skipFork := g.emitB()
	g.patchArm64BCondAt(fixFork, len(g.code))
	g.hasPending = false

	// SYS_EXECVE (19) → _execve(path, argv, envp)
	g.emitCmpImm(REG_X0, 19)
	fixExecve := g.emitBCond(COND_NE)
	g.emitLoadLocalArm64(2*8, REG_X0) // path
	g.emitLoadLocalArm64(3*8, REG_X1) // argv
	g.emitLoadLocalArm64(4*8, REG_X2) // envp
	g.emitCallGOT("_execve")
	g.emitSyscallReturnArm64()
	skipExecve := g.emitB()
	g.patchArm64BCondAt(fixExecve, len(g.code))
	g.hasPending = false

	// SYS_WAIT4 (20) → _wait4(pid, status, options, rusage)
	g.emitCmpImm(REG_X0, 20)
	fixWait4 := g.emitBCond(COND_NE)
	g.emitLoadLocalArm64(2*8, REG_X0) // pid
	g.emitLoadLocalArm64(3*8, REG_X1) // status
	g.emitLoadLocalArm64(4*8, REG_X2) // options
	g.emitLoadLocalArm64(5*8, REG_X3) // rusage
	g.emitCallGOT("_wait4")
	g.emitSyscallReturnArm64()
	skipWait4 := g.emitB()
	g.patchArm64BCondAt(fixWait4, len(g.code))
	g.hasPending = false

	// SYS_PIPE2 (21) → _pipe(fds) (macOS has pipe, not pipe2; flags arg ignored)
	g.emitCmpImm(REG_X0, 21)
	fixPipe := g.emitBCond(COND_NE)
	g.emitLoadLocalArm64(2*8, REG_X0) // fds array pointer
	g.emitCallGOT("_pipe")
	g.emitSyscallReturnArm64()
	skipPipe := g.emitB()
	g.patchArm64BCondAt(fixPipe, len(g.code))
	g.hasPending = false

	// SYS_CHMOD (22) → _chmod(path, mode)
	g.emitCmpImm(REG_X0, 22)
	fixChmod := g.emitBCond(COND_NE)
	g.emitLoadLocalArm64(2*8, REG_X0) // path
	g.emitLoadLocalArm64(3*8, REG_X1) // mode
	g.emitCallGOT("_chmod")
	g.emitSyscallReturnArm64()
	skipChmod := g.emitB()
	g.patchArm64BCondAt(fixChmod, len(g.code))
	g.hasPending = false

	// SYS_GETARGC (100) → read saved argc global
	g.emitCmpImm(REG_X0, 100)
	fixArgc := g.emitBCond(COND_NE)
	argcOff := len(g.irmod.Globals) * 8
	g.emitAdrpLdr(REG_X0, "$data_addr$", uint64(argcOff))
	g.rawPush(REG_X0) // r1=argc
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0) // r2=0
	g.rawPush(REG_X0) // err=0
	skipArgc := g.emitB()
	g.patchArm64BCondAt(fixArgc, len(g.code))
	g.hasPending = false

	// SYS_GETARGV (101) → read saved argv global
	g.emitCmpImm(REG_X0, 101)
	fixArgv := g.emitBCond(COND_NE)
	argvOff := (len(g.irmod.Globals) + 1) * 8
	g.emitAdrpLdr(REG_X0, "$data_addr$", uint64(argvOff))
	g.rawPush(REG_X0) // r1=argv
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0) // r2=0
	g.rawPush(REG_X0) // err=0
	skipArgv := g.emitB()
	g.patchArm64BCondAt(fixArgv, len(g.code))
	g.hasPending = false

	// SYS_GETENVP (102) → read saved envp global
	g.emitCmpImm(REG_X0, 102)
	fixEnvp := g.emitBCond(COND_NE)
	envpOff := (len(g.irmod.Globals) + 2) * 8
	g.emitAdrpLdr(REG_X0, "$data_addr$", uint64(envpOff))
	g.rawPush(REG_X0) // r1=envp
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0) // r2=0
	g.rawPush(REG_X0) // err=0
	skipEnvp := g.emitB()
	g.patchArm64BCondAt(fixEnvp, len(g.code))
	g.hasPending = false

	// Unsupported syscall: push r1=0, r2=0, err=1
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0) // r1=0
	g.rawPush(REG_X0) // r2=0
	g.emitLoadImm64Compact(REG_X0, 1)
	g.rawPush(REG_X0) // err=1

	// Patch all skip targets to here
	endAddr := len(g.code)
	g.patchArm64BAt(skipRead, endAddr)
	g.patchArm64BAt(skipWrite, endAddr)
	g.patchArm64BAt(skipOpen, endAddr)
	g.patchArm64BAt(skipClose, endAddr)
	g.patchArm64BAt(skipStat, endAddr)
	g.patchArm64BAt(skipMkdir, endAddr)
	g.patchArm64BAt(skipRmdir, endAddr)
	g.patchArm64BAt(skipUnlink, endAddr)
	g.patchArm64BAt(skipGetcwd, endAddr)
	g.patchArm64BAt(skipExit, endAddr)
	g.patchArm64BAt(skipMmap, endAddr)
	g.patchArm64BAt(skipOpendir, endAddr)
	g.patchArm64BAt(skipReaddir, endAddr)
	g.patchArm64BAt(skipClosedir, endAddr)
	g.patchArm64BAt(skipDup2, endAddr)
	g.patchArm64BAt(skipFork, endAddr)
	g.patchArm64BAt(skipExecve, endAddr)
	g.patchArm64BAt(skipWait4, endAddr)
	g.patchArm64BAt(skipPipe, endAddr)
	g.patchArm64BAt(skipChmod, endAddr)
	g.patchArm64BAt(skipArgc, endAddr)
	g.patchArm64BAt(skipArgv, endAddr)
	g.patchArm64BAt(skipEnvp, endAddr)
	g.hasPending = false
}

// emitSyscallReturnArm64 handles the standard libSystem return convention.
// Returns -1 on error. We check X0 < 0: if negative, err = -X0, r1 = 0.
// Uses rawPush to avoid pending-push optimization issues across branches.
func (g *CodeGen) emitSyscallReturnArm64() {
	g.flush() // ensure clean state before branching

	// Save return value in X2 (preserved across our code below)
	g.emitMovRRArm64(REG_X2, REG_X0)

	g.emitCmpImm(REG_X2, 0)
	errFixup := g.emitBCond(COND_LT) // branch if negative (error)

	// Success: r1=X2, r2=0, err=0
	g.rawPush(REG_X2)
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0) // r2=0
	g.rawPush(REG_X0) // err=0
	doneFixup := g.emitB()

	// Error: r1=0, r2=0, err=-X2
	g.patchArm64BCondAt(errFixup, len(g.code))
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0) // r1=0
	g.rawPush(REG_X0) // r2=0
	g.emitNeg(REG_X0, REG_X2)
	g.rawPush(REG_X0) // err=-X2

	g.patchArm64BAt(doneFixup, len(g.code))
	g.hasPending = false // clean state after merge
}

// emitSyscallReturnPtrArm64 handles pointer-returning calls (NULL or MAP_FAILED = error).
// Uses rawPush to avoid pending-push optimization issues across branches.
func (g *CodeGen) emitSyscallReturnPtrArm64() {
	g.flush() // ensure clean state before branching

	// Save return value in X2
	g.emitMovRRArm64(REG_X2, REG_X0)

	// Check for NULL (0) or MAP_FAILED (-1, i.e. negative)
	g.emitCmpImm(REG_X2, 0)
	errFixup := g.emitBCond(COND_LE) // branch if <= 0 (NULL or negative)

	// Success: r1=X2, r2=0, err=0
	g.rawPush(REG_X2)
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0) // r2=0
	g.rawPush(REG_X0) // err=0
	doneFixup := g.emitB()

	// Error: r1=0, r2=0, err=1
	g.patchArm64BCondAt(errFixup, len(g.code))
	g.emitMovZ(REG_X0, 0, 0)
	g.rawPush(REG_X0) // r1=0
	g.rawPush(REG_X0) // r2=0
	g.emitLoadImm64Compact(REG_X0, 1)
	g.rawPush(REG_X0) // err=1

	g.patchArm64BAt(doneFixup, len(g.code))
	g.hasPending = false // clean state after merge
}

// compilePanicArm64 handles panic on macOS ARM64.
func (g *CodeGen) compilePanicArm64() {
	// Pop value from operand stack
	g.opPop(REG_X0)

	// Tostring heuristic: if [X0] < 256, it's an interface box
	g.emitLdr(REG_X1, REG_X0, 0)
	g.emitCmpImm(REG_X1, 256)
	stringFixup := g.emitBCond(COND_CS) // branch if unsigned >= 256

	// Interface box: extract value (string ptr at [X0+8])
	g.emitLdr(REG_X0, REG_X0, 8)

	g.patchArm64BCondAt(stringFixup, len(g.code))

	// X0 = string header ptr {data_ptr, len}
	// Save on hardware stack (callee-saved)
	g.emitSubImm(REG_SP, REG_SP, 16)
	g.emitStr(REG_X0, REG_SP, 0)

	// Load data_ptr and len
	g.emitLdr(REG_X1, REG_X0, 0) // data_ptr
	g.emitLdr(REG_X2, REG_X0, 8) // len

	// _write(2, data_ptr, len)
	g.emitLoadImm64Compact(REG_X0, 2) // fd = stderr
	g.emitCallGOT("_write")

	// Write newline: allocate 1 byte on stack
	g.emitLoadImm64Compact(REG_X0, 0x0A) // '\n'
	g.emitSubImm(REG_SP, REG_SP, 16)
	g.emitStrb(REG_X0, REG_SP, 0)
	g.emitLoadImm64Compact(REG_X0, 2)    // fd = stderr
	g.emitMovRRArm64(REG_X1, REG_SP)     // buf = SP
	g.emitLoadImm64Compact(REG_X2, 1)    // len = 1
	g.emitCallGOT("_write")
	g.emitAddImm(REG_SP, REG_SP, 16)

	// Restore stack
	g.emitAddImm(REG_SP, REG_SP, 16)

	// _exit(2)
	g.emitLoadImm64Compact(REG_X0, 2)
	g.emitCallGOT("_exit")
}
