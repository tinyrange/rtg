//go:build rtg

package arm64

type reg interface {
	regCode() int
}

type temp0 struct{}
type temp1 struct{}

func (t temp0) regCode() int { return 0 } // X0
func (t temp1) regCode() int { return 1 } // X1

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
	d := regCode(dst)
	s := regCode(src)
	asmEmit32(0x8B000000 | ((s & 0x1f) << 16) | ((d & 0x1f) << 5) | (d & 0x1f))
}

func (a *Assembler) Mul(dst reg, imm int) {
	d := regCode(dst)
	t := regCode(a.Temp1)
	if d == t {
		t = regCode(a.Temp0)
	}
	asmLoad(t, imm)
	asmEmit32(0x9B007C00 | ((t & 0x1f) << 16) | ((d & 0x1f) << 5) | (d & 0x1f))
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
	panic("arm64.Assembler.Call supports up to 4 arguments")
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

//rtg:internal AsmArm64Begin
func asmBegin(name string, params int, rets int)

//rtg:internal AsmArm64Load
func asmLoad(dst int, src int)

//rtg:internal AsmArm64Emit32
func asmEmit32(inst int)

//rtg:internal AsmArm64Push
func asmPush(src int)

//rtg:internal AsmArm64Pop
func asmPop(dst int)

//rtg:internal AsmArm64Call0
func asmCall0(dst int, target string)

//rtg:internal AsmArm64Call1
func asmCall1(dst int, target string, a0 int)

//rtg:internal AsmArm64Call2
func asmCall2(dst int, target string, a0 int, a1 int)

//rtg:internal AsmArm64Call3
func asmCall3(dst int, target string, a0 int, a1 int, a2 int)

//rtg:internal AsmArm64Call4
func asmCall4(dst int, target string, a0 int, a1 int, a2 int, a3 int)

//rtg:internal AsmArm64Ret
func asmRet()

//rtg:internal AsmArm64TakeCode
func asmTakeCode() []byte

//rtg:internal AsmArm64TakeFixups
func asmTakeFixups() []byte
