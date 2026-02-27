//go:build !no_backend_dos_i386 && !tiny_dos_backend

package dos

//rtg:profile
func (g *CodeGen) callIntrinsic(name string) {
	switch name {
	case "Syscall":
		g.compileSyscallIntrinsic()
	case "Sliceptr":
		g.loadLocal(2, REG16_BX)
		g.emitLoadRM16(REG16_AX, EA16_BX, 0)
		g.opPush(REG16_AX)
	case "Makeslice":
		g.makeSliceIntrinsic()
	case "Stringptr":
		g.loadLocal(2, REG16_BX)
		g.emitLoadRM16(REG16_AX, EA16_BX, 0)
		g.opPush(REG16_AX)
	case "Makestring":
		g.makeStringIntrinsic()
	case "Tostring":
		g.tostringIntrinsic()
	case "ReadPtr":
		g.readPtrIntrinsic()
	case "WritePtr":
		g.writePtrIntrinsic()
	case "WriteByte":
		g.writeByteIntrinsic()
	default:
		panic("ICE: unknown intrinsic in 8086 backend: " + name)
	}
}

//rtg:profile
func (g *CodeGen) compileSyscallIntrinsic() {
	g.loadLocal(2, REG16_AX)
	g.cmpImm16(REG16_AX, 3)
	fixRead := g.jccNearRel16(CC16_E)
	g.cmpImm16(REG16_AX, 4)
	fixWrite := g.jccNearRel16(CC16_E)
	g.cmpImm16(REG16_AX, 5)
	fixOpen := g.jccNearRel16(CC16_E)
	g.cmpImm16(REG16_AX, 6)
	fixClose := g.jccNearRel16(CC16_E)
	g.cmpImm16(REG16_AX, 252)
	fixExit := g.jccNearRel16(CC16_E)
	g.cmpImm16(REG16_AX, 192)
	fixMmap := g.jccNearRel16(CC16_E)

	g.compileConst(0)
	g.compileConst(0)
	g.compileConst(38)
	done := g.jmpRel16()

	g.patchRel16(fixRead)
	g.loadLocal(4, REG16_BX)
	g.loadLocal(6, REG16_DX)
	g.loadLocal(8, REG16_CX)
	g.emitBytes(0xB4, 0x3F)
	g.emitBytes(0xCD, 0x21)
	fixReadErr := g.jccNearRel16(CC16_C)
	g.opPush(REG16_AX)
	g.compileConst(0)
	g.compileConst(0)
	readJoin := g.jmpRel16()
	g.patchRel16(fixReadErr)
	g.compileConst(0)
	g.compileConst(0)
	g.opPush(REG16_AX)
	readDone := g.jmpRel16()

	g.patchRel16(fixWrite)
	g.loadLocal(4, REG16_BX)
	g.loadLocal(6, REG16_DX)
	g.loadLocal(8, REG16_CX)
	g.emitBytes(0xB4, 0x40)
	g.emitBytes(0xCD, 0x21)
	fixWriteErr := g.jccNearRel16(CC16_C)
	g.opPush(REG16_AX)
	g.compileConst(0)
	g.compileConst(0)
	writeJoin := g.jmpRel16()
	g.patchRel16(fixWriteErr)
	g.compileConst(0)
	g.compileConst(0)
	g.opPush(REG16_AX)
	writeDone := g.jmpRel16()

	g.patchRel16(fixOpen)
	g.loadLocal(4, REG16_DX)
	g.loadLocal(6, REG16_CX)
	g.testRR16(REG16_CX, REG16_CX)
	fixCreate := g.jccNearRel16(CC16_NE)
	g.emitMovImm16(REG16_AX, 0x3D00)
	openDo := g.jmpRel16()
	g.patchRel16(fixCreate)
	g.xorRR16(REG16_CX, REG16_CX)
	g.emitMovImm16(REG16_AX, 0x3C00)
	g.patchRel16(openDo)
	g.emitBytes(0xCD, 0x21)
	fixOpenErr := g.jccNearRel16(CC16_C)
	g.opPush(REG16_AX)
	g.compileConst(0)
	g.compileConst(0)
	openDone := g.jmpRel16()
	g.patchRel16(fixOpenErr)
	g.compileConst(0)
	g.compileConst(0)
	g.opPush(REG16_AX)
	g.patchRel16(openDone)
	openJoin := g.jmpRel16()

	g.patchRel16(fixClose)
	g.loadLocal(4, REG16_BX)
	g.emitBytes(0xB4, 0x3E)
	g.emitBytes(0xCD, 0x21)
	fixCloseErr := g.jccNearRel16(CC16_C)
	g.compileConst(0)
	g.compileConst(0)
	g.compileConst(0)
	closeDone := g.jmpRel16()
	g.patchRel16(fixCloseErr)
	g.compileConst(0)
	g.compileConst(0)
	g.opPush(REG16_AX)
	g.patchRel16(closeDone)
	closeJoin := g.jmpRel16()

	g.patchRel16(fixExit)
	g.loadLocal(4, REG16_AX)
	g.emitBytes(0xB4, 0x4C)
	g.emitBytes(0xCD, 0x21)
	// If the DOS exit interrupt ever returns unexpectedly, report success
	// syscall return values instead of trapping.
	g.compileConst(0)
	g.compileConst(0)
	g.compileConst(0)
	exitJoin := g.jmpRel16()

	g.patchRel16(fixMmap)
	g.compileConst(0x7000)
	g.compileConst(0)
	g.compileConst(0)
	mmapJoin := g.jmpRel16()

	g.patchRel16(readJoin)
	g.patchRel16(readDone)
	g.patchRel16(writeJoin)
	g.patchRel16(writeDone)
	g.patchRel16(openJoin)
	g.patchRel16(closeJoin)
	g.patchRel16(exitJoin)
	g.patchRel16(mmapJoin)
	g.patchRel16(done)
}
