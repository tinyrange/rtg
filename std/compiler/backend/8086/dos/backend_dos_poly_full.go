//go:build !no_backend_dos_i386 && !tiny_dos_backend

package dos

import "j5.nz/rtg/std/compiler/backend/becommon"

//rtg:profile
func (g *CodeGen) tostringIntrinsic() {
	g.compileTostringBody()
}

//rtg:profile
func (g *CodeGen) emitTostringHelper() {
	if g.hasTostringHelper {
		return
	}
	g.hasTostringHelper = true
	g.funcOffsets[outlinedTostringHelper] = len(g.code)

	g.pushR16(REG16_BP)
	g.movRR16(REG16_BP, REG16_SP)
	g.subImm16(REG16_SP, 2)
	g.opPop(REG16_AX)
	g.storeLocal(2, REG16_AX)
	g.compileTostringBody()
	g.leave16()
	g.ret16()
}

//rtg:profile
func (g *CodeGen) compileTostringBody() {
	g.loadLocal(2, REG16_BX)
	g.emitLoadRM16(REG16_CX, EA16_BX, 0)
	g.cmpImm16(REG16_CX, 256)
	stringCase := g.jccNearRel16(CC16_AE)
	g.emitLoadRM16(REG16_DX, EA16_BX, 2)

	doneFixups := make([]int, 0)
	g.cmpImm16(REG16_CX, 1)
	next := g.jccNearRel16(CC16_NE)
	g.opPush(REG16_DX)
	g.emitCallPlaceholder("runtime.IntToString")
	doneFixups = append(doneFixups, g.jmpRel16())
	g.patchRel16(next)

	g.cmpImm16(REG16_CX, 2)
	next = g.jccNearRel16(CC16_NE)
	g.opPush(REG16_DX)
	doneFixups = append(doneFixups, g.jmpRel16())
	g.patchRel16(next)

	var entries []becommon.DispatchEntry
	if g.irmod != nil && g.irmod.TypeIDs != nil {
		for typeName, tid := range g.irmod.TypeIDs {
			c := typeName + ".Error"
			if _, ok := g.irmod.MethodTable[c]; ok {
				entries = append(entries, becommon.DispatchEntry{TypeID: tid, FuncName: c})
				continue
			}
			c = typeName + ".String"
			if _, ok := g.irmod.MethodTable[c]; ok {
				entries = append(entries, becommon.DispatchEntry{TypeID: tid, FuncName: c})
			}
		}
	}
	for _, e := range entries {
		g.cmpImm16(REG16_CX, int16(e.TypeID))
		next = g.jccNearRel16(CC16_NE)
		g.opPush(REG16_DX)
		g.emitCallPlaceholder(e.FuncName)
		doneFixups = append(doneFixups, g.jmpRel16())
		g.patchRel16(next)
	}

	g.compileConst(0)
	doneFixups = append(doneFixups, g.jmpRel16())
	g.patchRel16(stringCase)
	g.loadLocal(2, REG16_AX)
	g.opPush(REG16_AX)
	join := len(g.code)
	for _, f := range doneFixups {
		g.patchRel16At(f, join)
	}
}

//rtg:profile
func (g *CodeGen) ifaceBox(typeID int) {
	g.opPop(REG16_AX)
	g.pushR16(REG16_AX)
	g.compileConst(4)
	g.emitCallPlaceholder("runtime.Alloc")
	g.opPop(REG16_BX)
	g.emitMovImm16(REG16_AX, uint16(typeID))
	g.emitStoreRM16(EA16_BX, 0, REG16_AX)
	g.popR16(REG16_AX)
	g.emitStoreRM16(EA16_BX, 2, REG16_AX)
	g.opPush(REG16_BX)
}

//rtg:profile
func (g *CodeGen) ifaceCall(methodName string, argCount int) {
	for i := 0; i < argCount; i++ {
		g.opPop(REG16_AX)
		g.pushR16(REG16_AX)
	}
	g.opPop(REG16_BX)
	g.emitLoadRM16(REG16_CX, EA16_BX, 0)
	g.emitLoadRM16(REG16_DX, EA16_BX, 2)
	g.opPush(REG16_DX)
	for i := argCount - 1; i >= 0; i-- {
		g.popR16(REG16_AX)
		g.opPush(REG16_AX)
	}

	dot := len(methodName) - 1
	for dot >= 0 && methodName[dot] != '.' {
		dot--
	}
	bare := methodName
	if dot >= 0 && dot+1 < len(methodName) {
		bare = methodName[dot+1:]
	}

	var entries []becommon.DispatchEntry
	if g.irmod != nil && g.irmod.TypeIDs != nil {
		for typeName, tid := range g.irmod.TypeIDs {
			c := typeName + "." + bare
			if _, ok := g.irmod.MethodTable[c]; ok {
				entries = append(entries, becommon.DispatchEntry{TypeID: tid, FuncName: c})
			}
		}
	}
	if len(entries) == 0 {
		g.emitByte(0xCC)
		return
	}
	endFixups := make([]int, 0)
	for _, e := range entries {
		g.cmpImm16(REG16_CX, int16(e.TypeID))
		next := g.jccNearRel16(CC16_NE)
		g.emitCallPlaceholder(e.FuncName)
		endFixups = append(endFixups, g.jmpRel16())
		g.patchRel16(next)
	}
	g.emitByte(0xCC)
	end := len(g.code)
	for _, f := range endFixups {
		g.patchRel16At(f, end)
	}
}
