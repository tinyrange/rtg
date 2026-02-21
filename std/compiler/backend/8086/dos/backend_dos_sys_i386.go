//go:build !no_backend_dos_i386

package dos

// compileSyscallIntrinsic_dos386 lowers runtime.Syscall for the dos/8086 target.
//
// This is a compatibility shim over the Linux-like syscall numbers used by
// runtime_dos_16.go:
//
//	4   -> write(fd, buf, count) via INT 21h AH=40h
//	252 -> exit(code) via INT 21h AH=4Ch
//	192 -> mmap(...) stub returning a fixed near heap base
func (g *CodeGen) compileSyscallIntrinsic_dos386(paramCount int) {
	_ = paramCount
	w := g.slotBytes_i386()

	g.emitLoadLocal32(1*w, REG32_EAX) // syscall num
	g.cmpRI32(REG32_EAX, 4)
	fixWrite := g.jccRel32(CC32_E)
	g.cmpRI32(REG32_EAX, 252)
	fixExit := g.jccRel32(CC32_E)
	g.cmpRI32(REG32_EAX, 192)
	fixMmap := g.jccRel32(CC32_E)

	// Default: ENOSYS
	g.compileConstI32(0)
	g.compileConstI32(0)
	g.compileConstI32(38)
	fixDone := g.jmpRel32()

	// write(fd, buf, count)
	g.patchRel32(fixWrite)
	g.pushR32(REG32_EBP)
	g.pushR32(REG32_EDI)
	g.emitLoadLocal32(2*w, REG32_EBX) // fd
	g.emitLoadLocal32(3*w, REG32_EDX) // buf
	g.emitLoadLocal32(4*w, REG32_ECX) // count
	g.emitBytes(0xb4, 0x40)           // mov ah, 0x40
	g.emitBytes(0xcd, 0x21)           // int 0x21
	fixWriteErr := g.jccRel32(0x82)   // jc
	// success: AX = bytes written
	g.movRR32(REG32_ECX, REG32_EAX) // save r1
	g.popR32(REG32_EDI)
	g.popR32(REG32_EBP)
	g.opPush(REG32_ECX)
	g.compileConstI32(0)
	g.compileConstI32(0)
	fixWriteDone := g.jmpRel32()
	// error: AX = DOS error code
	g.patchRel32(fixWriteErr)
	g.movRR32(REG32_ECX, REG32_EAX) // err
	g.popR32(REG32_EDI)
	g.popR32(REG32_EBP)
	g.compileConstI32(0)
	g.compileConstI32(0)
	g.opPush(REG32_ECX)
	g.patchRel32(fixWriteDone)
	fixWriteJoin := g.jmpRel32()

	// exit(code)
	g.patchRel32(fixExit)
	g.emitLoadLocal32(2*w, REG32_EAX) // a0 (exit code)
	g.emitBytes(0xb4, 0x4c)           // mov ah, 0x4c
	g.emitBytes(0xcd, 0x21)           // int 0x21 (does not return)
	g.int3()

	// mmap stub
	g.patchRel32(fixMmap)
	g.compileConstI32(0x7000)
	g.compileConstI32(0)
	g.compileConstI32(0)
	fixMmapJoin := g.jmpRel32()

	g.patchRel32(fixWriteJoin)
	g.patchRel32(fixMmapJoin)
	g.patchRel32(fixDone)
}
