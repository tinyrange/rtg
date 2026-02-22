//go:build !rtg

package amd64

type reg interface {
	regCode() int
}

type temp0 struct{}
type temp1 struct{}

func (t temp0) regCode() int { return 0 }
func (t temp1) regCode() int { return 1 }

type Assembler struct {
	Temp0 temp0
	Temp1 temp1
}

func (a *Assembler) Load(dst reg, src int)                    {}
func (a *Assembler) Add(dst reg, src reg)                     {}
func (a *Assembler) Mul(dst reg, imm int)                     {}
func (a *Assembler) Push(src reg)                             {}
func (a *Assembler) Pop(dst reg)                              {}
func (a *Assembler) Call(dst reg, target string, args ...reg) {}
func (a *Assembler) Ret()                                     {}

func __rtg_asm_begin(name string, params int, rets int) {}
func __rtg_asm_take_code() []byte                       { return nil }
func __rtg_asm_take_fixups() []byte                     { return nil }
