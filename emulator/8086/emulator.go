package emu8086

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

const (
	memSize    = 1 << 20
	memMask    = memSize - 1
	defaultPSP = 0x1000
)

const DefaultPSP = defaultPSP

var reg16Names = []string{"ax", "cx", "dx", "bx", "sp", "bp", "si", "di"}

type cpu struct {
	mem [memSize]byte

	ax uint16
	bx uint16
	cx uint16
	dx uint16
	sp uint16
	bp uint16
	si uint16
	di uint16

	cs uint16
	ds uint16
	es uint16
	ss uint16
	ip uint16

	cf bool
	zf bool
	sf bool
	of bool

	trace    bool
	dbgWrite bool
	steps    int
	maxSteps int
	exited   bool
	exitCode int
	writes   int
}

type Options struct {
	Trace    bool
	MaxSteps int
	DbgWrite bool
}

type Result struct {
	ExitCode    int
	Steps       int
	Int21Writes int
}

func RunCOM(bin []byte, args []string, opts Options) (Result, error) {
	if opts.MaxSteps <= 0 {
		opts.MaxSteps = 2_000_000
	}
	if len(bin) > 65536-0x100 {
		return Result{}, fmt.Errorf("COM too large: %d bytes", len(bin))
	}
	WarnIfLooksLike32Bit(bin)

	c := &cpu{
		trace:    opts.Trace,
		maxSteps: opts.MaxSteps,
		dbgWrite: opts.DbgWrite,
	}
	if err := c.loadCOM(defaultPSP, bin, args); err != nil {
		return Result{}, fmt.Errorf("load COM: %w", err)
	}
	if err := c.run(); err != nil {
		return Result{}, err
	}
	return Result{
		ExitCode:    c.exitCode,
		Steps:       c.steps,
		Int21Writes: c.writes,
	}, nil
}

func RunCOMFile(comPath string, args []string, opts Options) (Result, error) {
	bin, err := os.ReadFile(comPath)
	if err != nil {
		return Result{}, fmt.Errorf("read %s: %w", comPath, err)
	}
	return RunCOM(bin, args, opts)
}

func WarnIfLooksLike32Bit(bin []byte) {
	if len(bin) < 20 {
		return
	}
	if bin[0] == 0xbf && bin[3] == 0x00 && bin[4] == 0x00 && bin[5] == 0xe8 {
		fmt.Fprintf(os.Stderr, "comemu: warning: binary looks like 32-bit code stream in a COM image; real DOS starts in 16-bit mode\n")
	}
}

func (c *cpu) loadCOM(psp uint16, bin []byte, args []string) error {
	base := linear(psp, 0)
	if int(base)+65536 > len(c.mem) {
		return errors.New("PSP segment out of memory")
	}

	for i := 0; i < 256; i++ {
		c.mem[int(base)+i] = 0
	}

	load := linear(psp, 0x100)
	copy(c.mem[load:], bin)

	c.cs = psp
	c.ds = psp
	c.es = psp
	c.ss = psp
	c.ip = 0x100
	c.sp = 0xfffe

	cmd := strings.Join(args, " ")
	if len(cmd) > 126 {
		cmd = cmd[:126]
	}
	c.mem[int(base)+0x80] = byte(len(cmd))
	copy(c.mem[int(base)+0x81:int(base)+0x81+len(cmd)], []byte(cmd))
	c.mem[int(base)+0x81+len(cmd)] = 0x0d
	return nil
}

func (c *cpu) run() error {
	for !c.exited {
		if c.steps >= c.maxSteps {
			return fmt.Errorf("step limit reached at %04x:%04x", c.cs, c.ip)
		}
		if err := c.step(); err != nil {
			return err
		}
		c.steps++
	}
	return nil
}

func (c *cpu) step() error {
	csip := c.csip()
	op := c.rb(csip)
	if c.trace {
		fmt.Fprintf(os.Stderr, "%04x:%04x op=%02x ax=%04x bx=%04x cx=%04x dx=%04x sp=%04x bp=%04x si=%04x di=%04x\n",
			c.cs, c.ip, op, c.ax, c.bx, c.cx, c.dx, c.sp, c.bp, c.si, c.di)
	}

	switch {
	case op >= 0xb8 && op <= 0xbf:
		reg := int(op - 0xb8)
		imm := c.u16(csip + 1)
		c.setReg16(reg, imm)
		c.ip += 3
		return nil
	case op >= 0xb0 && op <= 0xb7:
		reg := int(op - 0xb0)
		imm := c.rb(csip + 1)
		c.setReg8(reg, imm)
		c.ip += 2
		return nil
	case op >= 0x50 && op <= 0x57:
		reg := int(op - 0x50)
		c.push16(c.reg16(reg))
		c.ip++
		return nil
	case op >= 0x58 && op <= 0x5f:
		reg := int(op - 0x58)
		v := c.pop16()
		c.setReg16(reg, v)
		c.ip++
		return nil
	case op >= 0x72 && op <= 0x75:
		d := int8(c.rb(csip + 1))
		take := false
		switch op {
		case 0x72:
			take = c.cf
		case 0x73:
			take = !c.cf
		case 0x74:
			take = c.zf
		case 0x75:
			take = !c.zf
		}
		c.ip += 2
		if take {
			c.ip = uint16(int32(c.ip) + int32(d))
		}
		return nil
	}

	switch op {
	case 0x67:
		return c.exec67(csip)
	case 0x90:
		c.ip++
		return nil
	case 0xcd:
		vec := c.rb(csip + 1)
		c.ip += 2
		return c.handleInt(vec)
	case 0xe8:
		rel := int16(c.u16(csip + 1))
		ret := c.ip + 3
		c.push16(ret)
		c.ip = uint16(int32(ret) + int32(rel))
		return nil
	case 0xe9:
		rel := int16(c.u16(csip + 1))
		c.ip += 3
		c.ip = uint16(int32(c.ip) + int32(rel))
		return nil
	case 0xeb:
		rel := int8(c.rb(csip + 1))
		c.ip += 2
		c.ip = uint16(int32(c.ip) + int32(rel))
		return nil
	case 0xc3:
		c.ip = c.pop16()
		return nil
	case 0x31:
		return c.exec31(csip)
	case 0x01:
		return c.exec01(csip)
	case 0x29:
		return c.exec29(csip)
	case 0x09:
		return c.exec09(csip)
	case 0x21:
		return c.exec21(csip)
	case 0x39:
		return c.exec39(csip)
	case 0x85:
		return c.exec85(csip)
	case 0x83:
		return c.exec83(csip)
	case 0x81:
		return c.exec81(csip)
	case 0x69:
		return c.exec69(csip)
	case 0x99:
		return c.exec99(csip)
	case 0xf7:
		return c.execF7(csip)
	case 0x0f:
		return c.exec0F(csip)
	case 0x89:
		return c.exec89(csip)
	case 0x8b:
		return c.exec8b(csip)
	case 0x88:
		return c.exec88(csip)
	case 0x8a:
		return c.exec8a(csip)
	case 0x8d:
		return c.exec8d(csip)
	case 0x00:
		// add r/m8, r8 (supports common memory forms to keep execution going)
		return c.exec00(csip)
	default:
		return fmt.Errorf("unsupported opcode %02x at %04x:%04x (bytes %02x %02x %02x %02x)",
			op, c.cs, c.ip, c.rb(csip), c.rb(csip+1), c.rb(csip+2), c.rb(csip+3))
	}
}

func (c *cpu) exec67(pc uint32) error {
	op := c.rb(pc + 1)
	if op == 0x67 {
		// Consume one prefix and continue decoding the next address-size-prefixed instruction.
		c.ip++
		return c.exec67(pc + 1)
	}
	switch op {
	case 0x89:
		return c.exec89Addr32(pc)
	case 0x8b:
		return c.exec8bAddr32(pc)
	case 0x8d:
		return c.exec8dAddr32(pc)
	case 0x88:
		return c.exec88Addr32(pc)
	case 0x8a:
		return c.exec8aAddr32(pc)
	case 0x01:
		return c.exec01Addr32(pc)
	case 0x29:
		return c.exec29Addr32(pc)
	case 0x39:
		return c.exec39Addr32(pc)
	case 0x85:
		return c.exec85Addr32(pc)
	case 0x0f:
		return c.exec0FAddr32(pc)
	default:
		return fmt.Errorf("unsupported 67-prefixed opcode %02x at %04x:%04x", op, c.cs, c.ip)
	}
}

func (c *cpu) exec0FAddr32(pc uint32) error {
	op2 := c.rb(pc + 2)
	switch {
	case op2 >= 0x80 && op2 <= 0x8f:
		rel := int16(c.u16(pc + 3))
		c.ip += 5
		if c.evalCC(op2 & 0x0f) {
			c.ip = uint16(int32(c.ip) + int32(rel))
		}
		return nil
	case op2 == 0xb6 || op2 == 0xb7:
		modrm := c.rb(pc + 3)
		mod := (modrm >> 6) & 0x3
		reg := int((modrm >> 3) & 0x7)
		rm := modrm & 0x7
		if mod == 0x3 {
			if op2 == 0xb6 {
				c.setReg16(reg, uint16(c.reg8(int(rm))))
			} else {
				c.setReg16(reg, c.reg16(int(rm)))
			}
			c.ip += 4
			return nil
		}
		addr, dispLen, err := c.ea32(mod, rm, pc+4)
		if err != nil {
			return err
		}
		if op2 == 0xb6 {
			c.setReg16(reg, uint16(c.rb(addr)))
		} else {
			c.setReg16(reg, c.u16(addr))
		}
		c.ip += uint16(4 + dispLen)
		return nil
	default:
		if op2 >= 0x90 && op2 <= 0x9f {
			modrm := c.rb(pc + 3)
			mod := (modrm >> 6) & 0x3
			cc := op2 & 0x0f
			v := byte(0)
			if c.evalCC(cc) {
				v = 1
			}
			if mod == 0x3 {
				c.setReg8(int(modrm&0x7), v)
				c.ip += 4
				return nil
			}
			addr, dispLen, err := c.ea32(mod, modrm&0x7, pc+4)
			if err != nil {
				return err
			}
			c.wb(addr, v)
			c.ip += uint16(4 + dispLen)
			return nil
		}
		return fmt.Errorf("unsupported 67 0f opcode %02x at %04x:%04x", op2, c.cs, c.ip)
	}
}

func (c *cpu) exec89Addr32(pc uint32) error {
	modrm := c.rb(pc + 2)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := modrm & 0x7
	if mod == 0x3 {
		c.setReg16(int(rm), c.reg16(reg))
		c.ip += 3
		return nil
	}
	addr, dispLen, err := c.ea32(mod, rm, pc+3)
	if err != nil {
		return err
	}
	c.w16(addr, c.reg16(reg))
	c.ip += uint16(3 + dispLen)
	return nil
}

func (c *cpu) exec8bAddr32(pc uint32) error {
	modrm := c.rb(pc + 2)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := modrm & 0x7
	if mod == 0x3 {
		c.setReg16(reg, c.reg16(int(rm)))
		c.ip += 3
		return nil
	}
	addr, dispLen, err := c.ea32(mod, rm, pc+3)
	if err != nil {
		return err
	}
	c.setReg16(reg, c.u16(addr))
	c.ip += uint16(3 + dispLen)
	return nil
}

func (c *cpu) exec8dAddr32(pc uint32) error {
	modrm := c.rb(pc + 2)
	mod := (modrm >> 6) & 0x3
	rm := modrm & 0x7
	reg := int((modrm >> 3) & 0x7)
	addr, dispLen, err := c.ea32(mod, rm, pc+3)
	if err != nil {
		return err
	}
	c.setReg16(reg, uint16(addr&0xffff))
	c.ip += uint16(3 + dispLen)
	return nil
}

func (c *cpu) exec88Addr32(pc uint32) error {
	modrm := c.rb(pc + 2)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := modrm & 0x7
	if mod == 0x3 {
		c.setReg8(int(rm), c.reg8(reg))
		c.ip += 3
		return nil
	}
	addr, dispLen, err := c.ea32(mod, rm, pc+3)
	if err != nil {
		return err
	}
	c.wb(addr, c.reg8(reg))
	c.ip += uint16(3 + dispLen)
	return nil
}

func (c *cpu) exec8aAddr32(pc uint32) error {
	modrm := c.rb(pc + 2)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := modrm & 0x7
	if mod == 0x3 {
		c.setReg8(reg, c.reg8(int(rm)))
		c.ip += 3
		return nil
	}
	addr, dispLen, err := c.ea32(mod, rm, pc+3)
	if err != nil {
		return err
	}
	c.setReg8(reg, c.rb(addr))
	c.ip += uint16(3 + dispLen)
	return nil
}

func (c *cpu) exec01Addr32(pc uint32) error {
	modrm := c.rb(pc + 2)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := int(modrm & 0x7)
	if mod == 0x3 {
		a := c.reg16(rm)
		b := c.reg16(reg)
		res := a + b
		c.setReg16(rm, res)
		c.setAddFlags16(a, b, res)
		c.ip += 3
		return nil
	}
	addr, dispLen, err := c.ea32(mod, byte(rm), pc+3)
	if err != nil {
		return err
	}
	a := c.u16(addr)
	b := c.reg16(reg)
	res := a + b
	c.w16(addr, res)
	c.setAddFlags16(a, b, res)
	c.ip += uint16(3 + dispLen)
	return nil
}

func (c *cpu) exec29Addr32(pc uint32) error {
	modrm := c.rb(pc + 2)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := int(modrm & 0x7)
	if mod == 0x3 {
		a := c.reg16(rm)
		b := c.reg16(reg)
		res := a - b
		c.setReg16(rm, res)
		c.setSubFlags16(a, b, res)
		c.ip += 3
		return nil
	}
	addr, dispLen, err := c.ea32(mod, byte(rm), pc+3)
	if err != nil {
		return err
	}
	a := c.u16(addr)
	b := c.reg16(reg)
	res := a - b
	c.w16(addr, res)
	c.setSubFlags16(a, b, res)
	c.ip += uint16(3 + dispLen)
	return nil
}

func (c *cpu) exec39Addr32(pc uint32) error {
	modrm := c.rb(pc + 2)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := int(modrm & 0x7)
	if mod == 0x3 {
		a := c.reg16(rm)
		b := c.reg16(reg)
		res := a - b
		c.setSubFlags16(a, b, res)
		c.ip += 3
		return nil
	}
	addr, dispLen, err := c.ea32(mod, byte(rm), pc+3)
	if err != nil {
		return err
	}
	a := c.u16(addr)
	b := c.reg16(reg)
	res := a - b
	c.setSubFlags16(a, b, res)
	c.ip += uint16(3 + dispLen)
	return nil
}

func (c *cpu) exec85Addr32(pc uint32) error {
	modrm := c.rb(pc + 2)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := int(modrm & 0x7)
	if mod == 0x3 {
		res := c.reg16(rm) & c.reg16(reg)
		c.zf = res == 0
		c.sf = (res & 0x8000) != 0
		c.cf = false
		c.of = false
		c.ip += 3
		return nil
	}
	addr, dispLen, err := c.ea32(mod, byte(rm), pc+3)
	if err != nil {
		return err
	}
	res := c.u16(addr) & c.reg16(reg)
	c.zf = res == 0
	c.sf = (res & 0x8000) != 0
	c.cf = false
	c.of = false
	c.ip += uint16(3 + dispLen)
	return nil
}

func (c *cpu) ea32(mod, rm byte, dispStart uint32) (uint32, int, error) {
	consumed := 0
	var base uint32
	if rm == 4 {
		sib := c.rb(dispStart)
		consumed++
		baseReg := sib & 0x7
		if mod == 0 && baseReg == 5 {
			base = uint32(c.u16(dispStart + 1))
			consumed += 2
		} else {
			base = uint32(c.reg16(int(baseReg)))
		}
	} else if mod == 0 && rm == 5 {
		base = uint32(c.u16(dispStart))
		consumed += 2
	} else {
		base = uint32(c.reg16(int(rm)))
	}
	var disp int32
	switch mod {
	case 1:
		disp = int32(int8(c.rb(dispStart + uint32(consumed))))
		consumed++
	case 2:
		disp = int32(int16(c.u16(dispStart + uint32(consumed))))
		consumed += 2
	}
	off := uint32(int32(base) + disp)
	return linear(c.ds, uint16(off&0xffff)), consumed, nil
}

func (c *cpu) handleInt(vec byte) error {
	switch vec {
	case 0x21:
		ah := byte(c.ax >> 8)
		switch ah {
		case 0x4c:
			c.exitCode = int(byte(c.ax))
			c.exited = true
			return nil
		case 0x40:
			h := c.bx
			count := c.cx
			ptr := linear(c.ds, c.dx)
			data := make([]byte, int(count))
			i := 0
			for i < int(count) {
				data[i] = c.rb(ptr + uint32(i))
				i++
			}
			if c.trace {
				fmt.Fprintf(os.Stderr, "int21/40h handle=%d count=%d ds:dx=%04x:%04x\\n", h, count, c.ds, c.dx)
			}
			if c.dbgWrite {
				n := int(count)
				if n > 48 {
					n = 48
				}
				fmt.Fprintf(os.Stderr, "int21/40h bp=%04x h=%d count=%d dx=%04x bytes=", c.bp, h, count, c.dx)
				i := 0
				for i < n {
					fmt.Fprintf(os.Stderr, "%02x", data[i])
					if i+1 < n {
						fmt.Fprintf(os.Stderr, " ")
					}
					i++
				}
				if int(count) > n {
					fmt.Fprintf(os.Stderr, " ...")
				}
				fmt.Fprintf(os.Stderr, " locals=")
				j := 1
				for j <= 8 {
					off := uint16(uint16(j) * 2)
					v := c.u16(linear(c.ss, c.bp-off))
					fmt.Fprintf(os.Stderr, "%d:%04x", j, v)
					if j < 8 {
						fmt.Fprintf(os.Stderr, ",")
					}
					j++
				}
				fmt.Fprintf(os.Stderr, "\n")
			}
			switch h {
			case 1:
				_, _ = os.Stdout.Write(data)
			case 2:
				_, _ = os.Stderr.Write(data)
			default:
				// ignore unknown handle; report success for now
			}
			c.writes++
			c.ax = count
			c.cf = false
			return nil
		case 0x09:
			ptr := linear(c.ds, c.dx)
			max := uint32(memSize)
			i := uint32(0)
			for i < max {
				b := c.rb(ptr + i)
				if b == '$' {
					break
				}
				_, _ = os.Stdout.Write([]byte{b})
				i++
			}
			c.cf = false
			return nil
		default:
			return fmt.Errorf("unsupported int21h AH=%02x at %04x:%04x", ah, c.cs, c.ip)
		}
	default:
		return fmt.Errorf("unsupported interrupt 0x%02x at %04x:%04x", vec, c.cs, c.ip)
	}
}

func (c *cpu) exec00(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := (modrm >> 3) & 0x7
	rm := modrm & 0x7
	src := c.reg8(int(reg))
	addr, ok, dispLen := c.ea16(mod, rm, pc+2)
	if !ok {
		return fmt.Errorf("unsupported add r/m8 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	v := c.rb(addr)
	c.wb(addr, v+src)
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec31(pc uint32) error {
	modrm := c.rb(pc + 1)
	if (modrm >> 6) != 0x3 {
		return fmt.Errorf("unsupported xor memory form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	dst := int(modrm & 0x7)
	src := int((modrm >> 3) & 0x7)
	v := c.reg16(dst) ^ c.reg16(src)
	c.setReg16(dst, v)
	c.setLogicFlags16(v)
	c.ip += 2
	return nil
}

func (c *cpu) exec01(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := int(modrm & 0x7)
	if mod == 0x3 {
		a := c.reg16(rm)
		b := c.reg16(reg)
		res := a + b
		c.setReg16(rm, res)
		c.setAddFlags16(a, b, res)
		c.ip += 2
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, byte(rm), pc+2)
	if !ok {
		return fmt.Errorf("unsupported add r/m16 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	a := c.u16(addr)
	b := c.reg16(reg)
	res := a + b
	c.w16(addr, res)
	c.setAddFlags16(a, b, res)
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec29(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := int(modrm & 0x7)
	if mod == 0x3 {
		a := c.reg16(rm)
		b := c.reg16(reg)
		res := a - b
		c.setReg16(rm, res)
		c.setSubFlags16(a, b, res)
		c.ip += 2
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, byte(rm), pc+2)
	if !ok {
		return fmt.Errorf("unsupported sub r/m16 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	a := c.u16(addr)
	b := c.reg16(reg)
	res := a - b
	c.w16(addr, res)
	c.setSubFlags16(a, b, res)
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec09(pc uint32) error {
	modrm := c.rb(pc + 1)
	if (modrm >> 6) != 0x3 {
		return fmt.Errorf("unsupported or memory form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	dst := int(modrm & 0x7)
	src := int((modrm >> 3) & 0x7)
	v := c.reg16(dst) | c.reg16(src)
	c.setReg16(dst, v)
	c.setLogicFlags16(v)
	c.ip += 2
	return nil
}

func (c *cpu) exec21(pc uint32) error {
	modrm := c.rb(pc + 1)
	if (modrm >> 6) != 0x3 {
		return fmt.Errorf("unsupported and memory form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	dst := int(modrm & 0x7)
	src := int((modrm >> 3) & 0x7)
	v := c.reg16(dst) & c.reg16(src)
	c.setReg16(dst, v)
	c.setLogicFlags16(v)
	c.ip += 2
	return nil
}

func (c *cpu) exec39(pc uint32) error {
	modrm := c.rb(pc + 1)
	if (modrm >> 6) != 0x3 {
		return fmt.Errorf("unsupported cmp memory form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	a := c.reg16(int(modrm & 0x7))
	b := c.reg16(int((modrm >> 3) & 0x7))
	res := a - b
	c.setSubFlags16(a, b, res)
	c.ip += 2
	return nil
}

func (c *cpu) exec85(pc uint32) error {
	modrm := c.rb(pc + 1)
	if (modrm >> 6) != 0x3 {
		return fmt.Errorf("unsupported test memory form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	a := c.reg16(int(modrm & 0x7))
	b := c.reg16(int((modrm >> 3) & 0x7))
	res := a & b
	c.zf = res == 0
	c.sf = (res & 0x8000) != 0
	c.cf = false
	c.of = false
	c.ip += 2
	return nil
}

func (c *cpu) exec83(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	op := (modrm >> 3) & 0x7
	rm := int(modrm & 0x7)
	if mod != 0x3 {
		return fmt.Errorf("unsupported 83 memory form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	imm := uint16(int16(int8(c.rb(pc + 2))))
	v := c.reg16(rm)
	switch op {
	case 0: // add
		res := v + imm
		c.setReg16(rm, res)
		c.setAddFlags16(v, imm, res)
	case 5: // sub
		res := v - imm
		c.setReg16(rm, res)
		c.setSubFlags16(v, imm, res)
	case 6: // xor
		res := v ^ imm
		c.setReg16(rm, res)
		c.setLogicFlags16(res)
	case 7: // cmp
		res := v - imm
		c.setSubFlags16(v, imm, res)
	default:
		return fmt.Errorf("unsupported 83 /%d at %04x:%04x", op, c.cs, c.ip)
	}
	c.ip += 3
	return nil
}

func (c *cpu) exec81(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	op := (modrm >> 3) & 0x7
	rm := int(modrm & 0x7)
	if mod != 0x3 {
		return fmt.Errorf("unsupported 81 memory form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	imm := c.u16(pc + 2)
	v := c.reg16(rm)
	switch op {
	case 0: // add
		res := v + imm
		c.setReg16(rm, res)
		c.setAddFlags16(v, imm, res)
	case 5: // sub
		res := v - imm
		c.setReg16(rm, res)
		c.setSubFlags16(v, imm, res)
	case 6: // xor
		res := v ^ imm
		c.setReg16(rm, res)
		c.setLogicFlags16(res)
	case 7: // cmp
		res := v - imm
		c.setSubFlags16(v, imm, res)
	default:
		return fmt.Errorf("unsupported 81 /%d at %04x:%04x", op, c.cs, c.ip)
	}
	c.ip += 4
	return nil
}

func (c *cpu) exec69(pc uint32) error {
	// imul r16, r/m16, imm16
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	dst := int((modrm >> 3) & 0x7)
	rm := int(modrm & 0x7)
	immOff := pc + 2
	var src uint16
	var dispLen int
	if mod == 0x3 {
		src = c.reg16(rm)
	} else {
		addr, ok, d := c.ea16(mod, byte(rm), pc+2)
		if !ok {
			return fmt.Errorf("unsupported imul r/m16 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
		}
		src = c.u16(addr)
		dispLen = d
		immOff += uint32(dispLen)
	}
	imm := int16(c.u16(immOff))
	prod := int32(int16(src)) * int32(imm)
	c.setReg16(dst, uint16(prod))
	c.zf = uint16(prod) == 0
	c.sf = (uint16(prod) & 0x8000) != 0
	c.cf = false
	c.of = false
	c.ip += uint16(4 + dispLen)
	return nil
}

func (c *cpu) exec99(pc uint32) error {
	// cwd: sign-extend AX into DX:AX.
	if (c.ax & 0x8000) != 0 {
		c.dx = 0xffff
	} else {
		c.dx = 0
	}
	c.ip++
	return nil
}

func (c *cpu) execF7(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	subop := (modrm >> 3) & 0x7
	rm := int(modrm & 0x7)
	if mod != 0x3 {
		return fmt.Errorf("unsupported f7 memory form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	switch subop {
	case 3: // neg r/m16
		v := c.reg16(rm)
		res := uint16(0 - int16(v))
		c.setReg16(rm, res)
		c.zf = res == 0
		c.sf = (res & 0x8000) != 0
		c.cf = v != 0
		c.of = v == 0x8000
		c.ip += 2
		return nil
	case 7: // idiv r/m16
		divisor := int16(c.reg16(rm))
		if divisor == 0 {
			return fmt.Errorf("divide by zero at %04x:%04x", c.cs, c.ip)
		}
		dividend := int32(int16(c.dx))<<16 | int32(c.ax)
		q := dividend / int32(divisor)
		r := dividend % int32(divisor)
		if q < -32768 || q > 32767 {
			return fmt.Errorf("idiv overflow at %04x:%04x", c.cs, c.ip)
		}
		c.ax = uint16(int16(q))
		c.dx = uint16(int16(r))
		c.ip += 2
		return nil
	default:
		return fmt.Errorf("unsupported f7 /%d at %04x:%04x", subop, c.cs, c.ip)
	}
}

func (c *cpu) exec0F(pc uint32) error {
	op2 := c.rb(pc + 1)
	switch op2 {
	case 0x80, 0x81, 0x82, 0x83, 0x84, 0x85, 0x86, 0x87, 0x88, 0x89, 0x8a, 0x8b, 0x8c, 0x8d, 0x8e, 0x8f:
		rel := int16(c.u16(pc + 2))
		c.ip += 4
		if c.evalCC(op2 & 0x0f) {
			c.ip = uint16(int32(c.ip) + int32(rel))
		}
		return nil
	case 0xaf: // imul r16, r/m16
		modrm := c.rb(pc + 2)
		if (modrm >> 6) != 0x3 {
			return fmt.Errorf("unsupported imul memory form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
		}
		dst := int((modrm >> 3) & 0x7)
		src := int(modrm & 0x7)
		a := int16(c.reg16(dst))
		b := int16(c.reg16(src))
		c.setReg16(dst, uint16(a*b))
		c.ip += 3
		return nil
	case 0xb6: // movzx r16, r/m8
		modrm := c.rb(pc + 2)
		if (modrm >> 6) != 0x3 {
			return fmt.Errorf("unsupported movzx memory form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
		}
		dst := int((modrm >> 3) & 0x7)
		src := int(modrm & 0x7)
		c.setReg16(dst, uint16(c.reg8(src)))
		c.ip += 3
		return nil
	case 0xb7: // movzx r16, r/m16 (effectively mov)
		modrm := c.rb(pc + 2)
		if (modrm >> 6) != 0x3 {
			return fmt.Errorf("unsupported movzxw memory form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
		}
		dst := int((modrm >> 3) & 0x7)
		src := int(modrm & 0x7)
		c.setReg16(dst, c.reg16(src))
		c.ip += 3
		return nil
	default:
		if op2 >= 0x90 && op2 <= 0x9f { // setcc r/m8
			modrm := c.rb(pc + 2)
			if (modrm >> 6) != 0x3 {
				return fmt.Errorf("unsupported setcc memory form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
			}
			cc := op2 & 0x0f
			v := byte(0)
			if c.evalCC(cc) {
				v = 1
			}
			c.setReg8(int(modrm&0x7), v)
			c.ip += 3
			return nil
		}
		return fmt.Errorf("unsupported 0f opcode %02x at %04x:%04x", op2, c.cs, c.ip)
	}
}

func (c *cpu) evalCC(cc byte) bool {
	switch cc {
	case 0x2: // B/NAE/C
		return c.cf
	case 0x3: // AE/NB/NC
		return !c.cf
	case 0x4: // E/Z
		return c.zf
	case 0x5: // NE/NZ
		return !c.zf
	case 0x8: // S
		return c.sf
	case 0x9: // NS
		return !c.sf
	case 0xC: // L
		return c.sf != false && !c.zf
	case 0xD: // GE
		return !c.sf
	case 0xE: // LE
		return c.zf || c.sf
	case 0xF: // G
		return !c.zf && !c.sf
	default:
		return false
	}
}

func (c *cpu) exec89(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := uint16((modrm >> 3) & 0x7)
	rm := modrm & 0x7
	if mod == 0x3 {
		c.setReg16(int(rm), c.reg16(int(reg)))
		c.ip += 2
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, rm, pc+2)
	if !ok {
		return fmt.Errorf("unsupported mov r/m16,r16 modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	c.w16(addr, c.reg16(int(reg)))
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec8b(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := modrm & 0x7
	if mod == 0x3 {
		c.setReg16(reg, c.reg16(int(rm)))
		c.ip += 2
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, rm, pc+2)
	if !ok {
		return fmt.Errorf("unsupported mov r16,r/m16 modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	c.setReg16(reg, c.u16(addr))
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec8d(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := modrm & 0x7
	addr, ok, dispLen := c.ea16(mod, rm, pc+2)
	if !ok {
		return fmt.Errorf("unsupported lea form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	c.setReg16(reg, uint16(addr&0xffff))
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec88(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := modrm & 0x7
	if mod == 0x3 {
		c.setReg8(int(rm), c.reg8(reg))
		c.ip += 2
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, rm, pc+2)
	if !ok {
		return fmt.Errorf("unsupported mov r/m8,r8 modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	c.wb(addr, c.reg8(reg))
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec8a(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := modrm & 0x7
	if mod == 0x3 {
		c.setReg8(reg, c.reg8(int(rm)))
		c.ip += 2
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, rm, pc+2)
	if !ok {
		return fmt.Errorf("unsupported mov r8,r/m8 modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	c.setReg8(reg, c.rb(addr))
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) ea16(mod, rm byte, dispStart uint32) (uint32, bool, int) {
	var base uint16
	switch rm {
	case 0:
		base = c.bx + c.si
	case 1:
		base = c.bx + c.di
	case 2:
		base = c.bp + c.si
	case 3:
		base = c.bp + c.di
	case 4:
		base = c.si
	case 5:
		base = c.di
	case 6:
		if mod == 0 {
			d := c.u16(dispStart)
			return linear(c.ds, d), true, 2
		}
		base = c.bp
	case 7:
		base = c.bx
	}

	seg := c.ds
	if rm == 2 || rm == 3 || rm == 6 {
		seg = c.ss
	}

	switch mod {
	case 0:
		return linear(seg, base), true, 0
	case 1:
		d := int16(int8(c.rb(dispStart)))
		return linear(seg, uint16(int32(base)+int32(d))), true, 1
	case 2:
		d := int16(c.u16(dispStart))
		return linear(seg, uint16(int32(base)+int32(d))), true, 2
	case 3:
		return 0, false, 0
	default:
		return 0, false, 0
	}
}

func linear(seg, off uint16) uint32 {
	return (uint32(seg)*16 + uint32(off)) & memMask
}

func (c *cpu) csip() uint32 { return linear(c.cs, c.ip) }

func (c *cpu) rb(addr uint32) byte {
	return c.mem[addr&memMask]
}

func (c *cpu) wb(addr uint32, v byte) {
	c.mem[addr&memMask] = v
}

func (c *cpu) u16(addr uint32) uint16 {
	return uint16(c.rb(addr)) | uint16(c.rb(addr+1))<<8
}

func (c *cpu) w16(addr uint32, v uint16) {
	c.wb(addr, byte(v))
	c.wb(addr+1, byte(v>>8))
}

func (c *cpu) push16(v uint16) {
	c.sp -= 2
	c.w16(linear(c.ss, c.sp), v)
}

func (c *cpu) pop16() uint16 {
	v := c.u16(linear(c.ss, c.sp))
	c.sp += 2
	return v
}

func (c *cpu) reg16(i int) uint16 {
	switch i {
	case 0:
		return c.ax
	case 1:
		return c.cx
	case 2:
		return c.dx
	case 3:
		return c.bx
	case 4:
		return c.sp
	case 5:
		return c.bp
	case 6:
		return c.si
	case 7:
		return c.di
	default:
		panic("bad reg16")
	}
}

func (c *cpu) setReg16(i int, v uint16) {
	switch i {
	case 0:
		c.ax = v
	case 1:
		c.cx = v
	case 2:
		c.dx = v
	case 3:
		c.bx = v
	case 4:
		c.sp = v
	case 5:
		c.bp = v
	case 6:
		c.si = v
	case 7:
		c.di = v
	default:
		panic("bad reg16")
	}
}

func (c *cpu) reg8(i int) byte {
	switch i {
	case 0:
		return byte(c.ax)
	case 1:
		return byte(c.cx)
	case 2:
		return byte(c.dx)
	case 3:
		return byte(c.bx)
	case 4:
		return byte(c.ax >> 8)
	case 5:
		return byte(c.cx >> 8)
	case 6:
		return byte(c.dx >> 8)
	case 7:
		return byte(c.bx >> 8)
	default:
		panic("bad reg8")
	}
}

func (c *cpu) setReg8(i int, v byte) {
	switch i {
	case 0:
		c.ax = (c.ax & 0xff00) | uint16(v)
	case 1:
		c.cx = (c.cx & 0xff00) | uint16(v)
	case 2:
		c.dx = (c.dx & 0xff00) | uint16(v)
	case 3:
		c.bx = (c.bx & 0xff00) | uint16(v)
	case 4:
		c.ax = (c.ax & 0x00ff) | uint16(v)<<8
	case 5:
		c.cx = (c.cx & 0x00ff) | uint16(v)<<8
	case 6:
		c.dx = (c.dx & 0x00ff) | uint16(v)<<8
	case 7:
		c.bx = (c.bx & 0x00ff) | uint16(v)<<8
	default:
		panic("bad reg8")
	}
}

func (c *cpu) setLogicFlags16(v uint16) {
	c.zf = v == 0
	c.sf = (v & 0x8000) != 0
	c.cf = false
	c.of = false
}

func (c *cpu) setAddFlags16(a, b, res uint16) {
	c.zf = res == 0
	c.sf = (res & 0x8000) != 0
	c.cf = res < a
	c.of = ((^(a ^ b)) & (a ^ res) & 0x8000) != 0
}

func (c *cpu) setSubFlags16(a, b, res uint16) {
	c.zf = res == 0
	c.sf = (res & 0x8000) != 0
	c.cf = a < b
	c.of = ((a ^ b) & (a ^ res) & 0x8000) != 0
}

func _unusedRegName(i int) string {
	if i >= 0 && i < len(reg16Names) {
		return reg16Names[i]
	}
	return "?"
}
