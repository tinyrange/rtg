//go:build rtg

package amd64

type reg interface {
	regCode() int
}

type temp0 struct{}

type temp1 struct{}

func (t temp0) regCode() int { return 0 } // RAX
func (t temp1) regCode() int { return 1 } // RCX

type Assembler struct {
	Temp0 temp0
	Temp1 temp1
}

func regCode(r reg) int {
	if r == nil {
		return 0
	}
	return r.regCode()
}

func (a *Assembler) Load(dst reg, src int) {
	asmLoad(regCode(dst), src)
}

func (a *Assembler) Add(dst reg, src reg) {
	asmAdd(regCode(dst), regCode(src))
}

func (a *Assembler) Mul(dst reg, imm int) {
	asmMul(regCode(dst), imm)
}

func (a *Assembler) Push(src reg) {
	asmPush(regCode(src))
}

func (a *Assembler) Pop(dst reg) {
	asmPop(regCode(dst))
}

func (a *Assembler) Call(dst reg, target string, args ...reg) {
	d := regCode(dst)
	if len(args) == 0 {
		asmCall0(d, target)
		return
	}
	if len(args) == 1 {
		asmCall1(d, target, regCode(args[0]))
		return
	}
	if len(args) == 2 {
		asmCall2(d, target, regCode(args[0]), regCode(args[1]))
		return
	}
	if len(args) == 3 {
		asmCall3(d, target, regCode(args[0]), regCode(args[1]), regCode(args[2]))
		return
	}
	if len(args) == 4 {
		asmCall4(d, target, regCode(args[0]), regCode(args[1]), regCode(args[2]), regCode(args[3]))
		return
	}
	panic("amd64.Assembler.Call supports up to 4 arguments")
}

func (a *Assembler) Ret() {
	asmRet()
}

func __rtg_asm_begin(name string, params int, rets int) {
	asmBegin(name, params, rets)
}

func __rtg_asm_take_code() []byte {
	return asmTakeCode()
}

func __rtg_asm_take_fixups() []byte {
	return asmTakeFixups()
}

//rtg:internal AsmAmd64Begin
func asmBegin(name string, params int, rets int)

//rtg:internal AsmAmd64Load
func asmLoad(dst int, src int)

//rtg:internal AsmAmd64Add
func asmAdd(dst int, src int)

//rtg:internal AsmAmd64Mul
func asmMul(dst int, imm int)

//rtg:internal AsmAmd64Push
func asmPush(src int)

//rtg:internal AsmAmd64Pop
func asmPop(dst int)

//rtg:internal AsmAmd64Call0
func asmCall0(dst int, target string)

//rtg:internal AsmAmd64Call1
func asmCall1(dst int, target string, a0 int)

//rtg:internal AsmAmd64Call2
func asmCall2(dst int, target string, a0 int, a1 int)

//rtg:internal AsmAmd64Call3
func asmCall3(dst int, target string, a0 int, a1 int, a2 int)

//rtg:internal AsmAmd64Call4
func asmCall4(dst int, target string, a0 int, a1 int, a2 int, a3 int)

//rtg:internal AsmAmd64Ret
func asmRet()

//rtg:internal AsmAmd64TakeCode
func asmTakeCode() []byte

//rtg:internal AsmAmd64TakeFixups
func asmTakeFixups() []byte
