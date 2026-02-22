//go:build rtg

package i386

type reg interface {
	regCode() int
}

type temp0 struct{}
type temp1 struct{}

func (t temp0) regCode() int { return 0 } // EAX
func (t temp1) regCode() int { return 1 } // ECX

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
	asmEmit(2, 0x01, 0xc0|((s&7)<<3)|(d&7), 0, 0, 0, 0, 0, 0)
}

func (a *Assembler) Mul(dst reg, imm int) {
	if imm < -2147483648 || imm > 2147483647 {
		panic("i386.Assembler.Mul immediate must fit int32")
	}
	d := regCode(dst)
	v := uint32(int32(imm))
	asmEmit(
		6,
		0x69,
		0xc0|((d&7)<<3)|(d&7),
		int(v),
		int(v>>8),
		int(v>>16),
		int(v>>24),
		0,
		0,
	)
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
	panic("i386.Assembler.Call supports up to 4 arguments")
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

//rtg:internal AsmI386Begin
func asmBegin(name string, params int, rets int)

//rtg:internal AsmI386Load
func asmLoad(dst int, src int)

//rtg:internal AsmI386Emit
func asmEmit(n int, b0 int, b1 int, b2 int, b3 int, b4 int, b5 int, b6 int, b7 int)

//rtg:internal AsmI386Push
func asmPush(src int)

//rtg:internal AsmI386Pop
func asmPop(dst int)

//rtg:internal AsmI386Call0
func asmCall0(dst int, target string)

//rtg:internal AsmI386Call1
func asmCall1(dst int, target string, a0 int)

//rtg:internal AsmI386Call2
func asmCall2(dst int, target string, a0 int, a1 int)

//rtg:internal AsmI386Call3
func asmCall3(dst int, target string, a0 int, a1 int, a2 int)

//rtg:internal AsmI386Call4
func asmCall4(dst int, target string, a0 int, a1 int, a2 int, a3 int)

//rtg:internal AsmI386Ret
func asmRet()

//rtg:internal AsmI386TakeCode
func asmTakeCode() []byte

//rtg:internal AsmI386TakeFixups
func asmTakeFixups() []byte
