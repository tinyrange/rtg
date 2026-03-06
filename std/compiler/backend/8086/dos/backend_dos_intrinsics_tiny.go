//go:build !no_backend_dos_i386 && tiny_dos_backend

package dos

func (g *CodeGen) callIntrinsic(name string) {
	switch name {
	case "Syscall":
		g.compileSyscallIntrinsic()
	case "Alloc":
		g.loadLocal(2, REG16_AX)
		g.opPush(REG16_AX)
		g.emitCallPlaceholder("runtime.Alloc")
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
	case "ReadPtr":
		g.readPtrIntrinsic()
	case "WritePtr":
		g.writePtrIntrinsic()
	case "WriteByte":
		g.writeByteIntrinsic()
	default:
		panic("ICE: intrinsic disabled in tiny_dos_backend: " + name)
	}
}

func (g *CodeGen) compileSyscallIntrinsic() {
	g.code = append(g.code, tinySyscallIntrinsicTemplate...)
}

const tinySyscallIntrinsicTemplate = "" +
	"\x8B\x46\xFE\x3D\xFC\x00\x74\x1E\x3D\x03\x00\x74" +
	"\x21\x3D\x04\x00\x74\x39\x3D\x05\x00\x74\x51\x3D" +
	"\x06\x00\x74\x72\xB8\x00\x70\x50\x31\xC9\x51\x51" +
	"\xEB\x7F\x8B\x46\xFC\xB4\x4C\xCD\x21\xCC\x8B\x5E" +
	"\xFC\x8B\x56\xFA\x8B\x4E\xF8\xB4\x3F\xCD\x21\x72" +
	"\x07\x50\x31\xC9\x51\x51\xEB\x61\x31\xC9\x51\x51" +
	"\x50\xEB\x5A\x8B\x5E\xFC\x8B\x56\xFA\x8B\x4E\xF8" +
	"\xB4\x40\xCD\x21\x72\x07\x50\x31\xC9\x51\x51\xEB" +
	"\x44\x31\xC9\x51\x51\x50\xEB\x3D\x8B\x56\xFC\x8B" +
	"\x4E\xFA\x85\xC9\x75\x05\xB8\x00\x3D\xEB\x05\x31" +
	"\xC9\xB8\x00\x3C\xCD\x21\x72\x07\x50\x31\xC9\x51" +
	"\x51\xEB\x1E\x31\xC9\x51\x51\x50\xEB\x17\x8B\x5E" +
	"\xFC\xB4\x3E\xCD\x21\x72\x07\x31\xC9\x51\x51\x51" +
	"\xEB\x07\x31\xC9\x51\x51\x50\xEB\x00"
