package emu8086

import (
	"errors"
	"fmt"
	"io"
	"math/bits"
	"os"
	"strings"
)

const (
	memSize    = 1 << 20
	memMask    = memSize - 1
	defaultPSP = 0x1000

	modeledFlagsMask = uint16((1 << 0) | (1 << 2) | (1 << 4) | (1 << 6) | (1 << 7) | (1 << 11))
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
	af bool
	zf bool
	sf bool
	of bool
	pf bool

	extraFlags uint16

	trace    bool
	dbgWrite bool
	steps    int
	maxSteps int
	exited   bool
	exitCode int
	writes   int

	segOverrideSet bool
	segOverride    uint16

	dosMode bool

	files          map[uint16]*os.File
	nextFileHandle uint16
	faultErr       error
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

	c := &cpu{
		trace:    opts.Trace,
		maxSteps: opts.MaxSteps,
		dbgWrite: opts.DbgWrite,
		dosMode:  true,
		files:    make(map[uint16]*os.File),
		// 0,1,2 are std handles in DOS.
		nextFileHandle: 5,
	}
	defer c.closeFiles()
	if isMZ(bin) {
		if err := c.loadEXE(defaultPSP, bin, args); err != nil {
			return Result{}, fmt.Errorf("load EXE: %w", err)
		}
	} else {
		if len(bin) > 65536-0x100 {
			return Result{}, fmt.Errorf("COM too large: %d bytes", len(bin))
		}
		WarnIfLooksLike32Bit(bin)
		if err := c.loadCOM(defaultPSP, bin, args); err != nil {
			return Result{}, fmt.Errorf("load COM: %w", err)
		}
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

func isMZ(bin []byte) bool {
	return len(bin) >= 2 && bin[0] == 'M' && bin[1] == 'Z'
}

func rd16(b []byte, off int) (uint16, bool) {
	if off+1 >= len(b) {
		return 0, false
	}
	return uint16(b[off]) | (uint16(b[off+1]) << 8), true
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

func (c *cpu) loadEXE(psp uint16, bin []byte, args []string) error {
	if len(bin) < 28 || !isMZ(bin) {
		return errors.New("invalid MZ header")
	}
	eCblp, ok := rd16(bin, 2)
	if !ok {
		return errors.New("bad header")
	}
	eCp, _ := rd16(bin, 4)
	eCrlc, _ := rd16(bin, 6)
	eCparhdr, _ := rd16(bin, 8)
	eSS, _ := rd16(bin, 14)
	eSP, _ := rd16(bin, 16)
	eIP, _ := rd16(bin, 20)
	eCS, _ := rd16(bin, 22)
	eLfarlc, _ := rd16(bin, 24)

	fileSize := int(eCp) * 512
	if eCp == 0 {
		return errors.New("invalid page count")
	}
	if eCblp != 0 {
		fileSize -= 512
		fileSize += int(eCblp)
	}
	if fileSize > len(bin) {
		fileSize = len(bin)
	}
	headerSize := int(eCparhdr) * 16
	if headerSize < 28 || headerSize > fileSize {
		return errors.New("invalid header size")
	}

	base := linear(psp, 0)
	if int(base)+65536 > len(c.mem) {
		return errors.New("PSP segment out of memory")
	}
	for i := 0; i < 256; i++ {
		c.mem[int(base)+i] = 0
	}

	loadSeg := psp + 0x10
	loadBase := linear(loadSeg, 0)
	image := bin[headerSize:fileSize]
	if int(loadBase)+len(image) > len(c.mem) {
		return errors.New("image out of memory")
	}
	copy(c.mem[loadBase:], image)

	// Apply relocations.
	relocBase := int(eLfarlc)
	for i := 0; i < int(eCrlc); i++ {
		off, ok1 := rd16(bin, relocBase+i*4)
		seg, ok2 := rd16(bin, relocBase+i*4+2)
		if !ok1 || !ok2 {
			return errors.New("bad relocation table")
		}
		addr := linear(loadSeg+seg, off)
		if int(addr)+1 >= len(c.mem) {
			return errors.New("relocation out of memory")
		}
		v := uint16(c.mem[addr]) | (uint16(c.mem[addr+1]) << 8)
		v += loadSeg
		c.mem[addr] = byte(v)
		c.mem[addr+1] = byte(v >> 8)
	}

	c.cs = loadSeg + eCS
	c.ip = eIP
	c.ss = loadSeg + eSS
	c.sp = eSP
	c.ds = psp
	c.es = psp

	cmd := strings.Join(args, " ")
	if len(cmd) > 126 {
		cmd = cmd[:126]
	}
	c.mem[int(base)+0x80] = byte(len(cmd))
	copy(c.mem[int(base)+0x81:int(base)+0x81+len(cmd)], []byte(cmd))
	c.mem[int(base)+0x81+len(cmd)] = 0x0d
	return nil
}

func (c *cpu) closeFiles() {
	for h, f := range c.files {
		_ = f.Close()
		delete(c.files, h)
	}
}

func (c *cpu) allocFileHandle() uint16 {
	h := c.nextFileHandle
	start := h
	for {
		if _, inUse := c.files[h]; !inUse && h > 2 {
			c.nextFileHandle = h + 1
			return h
		}
		h++
		if h == start {
			return 0
		}
	}
}

func (c *cpu) readDOSString(seg, off uint16) (string, bool) {
	buf := make([]byte, 0, 64)
	i := uint16(0)
	for i < 0xfff0 {
		b := c.rb(linear(seg, off+i))
		if b == 0 {
			return string(buf), true
		}
		buf = append(buf, b)
		i++
	}
	return "", false
}

func (c *cpu) run() error {
	for !c.exited {
		if c.faultErr != nil {
			return c.faultErr
		}
		if c.steps >= c.maxSteps {
			return fmt.Errorf("step limit reached at %04x:%04x", c.cs, c.ip)
		}
		if err := c.step(); err != nil {
			return err
		}
		if c.faultErr != nil {
			return c.faultErr
		}
		c.steps++
	}
	return nil
}

func (c *cpu) step() error {
	c.segOverrideSet = false
	repPrefix := byte(0)
	defer func() {
		c.segOverrideSet = false
	}()

	csip := c.csip()
	op := c.rb(csip)

	for {
		switch op {
		case 0x26: // ES:
			c.segOverrideSet = true
			c.segOverride = c.es
		case 0x2e: // CS:
			c.segOverrideSet = true
			c.segOverride = c.cs
		case 0x36: // SS:
			c.segOverrideSet = true
			c.segOverride = c.ss
		case 0x3e: // DS:
			c.segOverrideSet = true
			c.segOverride = c.ds
		case 0xf0: // LOCK (ignored in this single-core emulator)
		case 0xf2, 0xf3: // REPNE/REP
			repPrefix = op
		default:
			goto decoded
		}
		c.ip++
		csip = c.csip()
		op = c.rb(csip)
	}

decoded:
	if c.trace {
		fmt.Fprintf(os.Stderr, "%04x:%04x op=%02x ax=%04x bx=%04x cx=%04x dx=%04x sp=%04x bp=%04x si=%04x di=%04x\n",
			c.cs, c.ip, op, c.ax, c.bx, c.cx, c.dx, c.sp, c.bp, c.si, c.di)
	}
	if op >= 0x60 && op <= 0x7f {
		return c.execJcc8(op&0x0f, csip)
	}

	switch {
	case op >= 0xb8 && op <= 0xbf:
		reg := int(op - 0xb8)
		imm := c.u16(csip + 1)
		c.setReg16(reg, imm)
		c.ip += 3
		return nil
	case op >= 0x40 && op <= 0x47:
		reg := int(op - 0x40)
		a := c.reg16(reg)
		res := a + 1
		c.setReg16(reg, res)
		c.setIncFlags16(a, res)
		c.ip++
		return nil
	case op >= 0x48 && op <= 0x4f:
		reg := int(op - 0x48)
		a := c.reg16(reg)
		res := a - 1
		c.setReg16(reg, res)
		c.setDecFlags16(a, res)
		c.ip++
		return nil
	case op >= 0xb0 && op <= 0xb7:
		reg := int(op - 0xb0)
		imm := c.rb(csip + 1)
		c.setReg8(reg, imm)
		c.ip += 2
		return nil
	case op >= 0x50 && op <= 0x57:
		reg := int(op - 0x50)
		v := c.reg16(reg)
		if reg == 4 { // PUSH SP stores post-decrement SP on 8088
			v = c.sp - 2
		}
		c.push16(v)
		c.ip++
		return nil
	case op >= 0x58 && op <= 0x5f:
		reg := int(op - 0x58)
		v := c.pop16()
		c.setReg16(reg, v)
		c.ip++
		return nil
	}

	switch op {
	case 0x04:
		imm := c.rb(csip + 1)
		a := c.reg8(0)
		res := a + imm
		c.setReg8(0, res)
		c.setAddFlags8(a, imm, res)
		c.ip += 2
		return nil
	case 0x05:
		imm := c.u16(csip + 1)
		a := c.ax
		res := a + imm
		c.ax = res
		c.setAddFlags16(a, imm, res)
		c.ip += 3
		return nil
	case 0x06:
		c.push16(c.es)
		c.ip++
		return nil
	case 0x07:
		c.es = c.pop16()
		c.ip++
		return nil
	case 0x08:
		return c.exec08(csip)
	case 0x0e:
		c.push16(c.cs)
		c.ip++
		return nil
	case 0x0f:
		c.cs = c.pop16()
		c.ip++
		return nil
	case 0x0a:
		return c.exec0A(csip)
	case 0x0b:
		return c.exec0B(csip)
	case 0x0c:
		imm := c.rb(csip + 1)
		res := c.reg8(0) | imm
		c.setReg8(0, res)
		c.setLogicFlags8(res)
		c.ip += 2
		return nil
	case 0x0d:
		imm := c.u16(csip + 1)
		res := c.ax | imm
		c.ax = res
		c.setLogicFlags16(res)
		c.ip += 3
		return nil
	case 0x10:
		return c.exec10(csip)
	case 0x11:
		return c.exec11(csip)
	case 0x12:
		return c.exec12(csip)
	case 0x13:
		return c.exec13(csip)
	case 0x14:
		imm := c.rb(csip + 1)
		res := c.adc8(c.reg8(0), imm)
		c.setReg8(0, res)
		c.ip += 2
		return nil
	case 0x15:
		imm := c.u16(csip + 1)
		c.ax = c.adc16(c.ax, imm)
		c.ip += 3
		return nil
	case 0x16:
		c.push16(c.ss)
		c.ip++
		return nil
	case 0x17:
		c.ss = c.pop16()
		c.ip++
		return nil
	case 0x18:
		return c.exec18(csip)
	case 0x19:
		return c.exec19(csip)
	case 0x1a:
		return c.exec1A(csip)
	case 0x1b:
		return c.exec1B(csip)
	case 0x1c:
		imm := c.rb(csip + 1)
		c.setReg8(0, c.sbb8(c.reg8(0), imm))
		c.ip += 2
		return nil
	case 0x1d:
		imm := c.u16(csip + 1)
		c.ax = c.sbb16(c.ax, imm)
		c.ip += 3
		return nil
	case 0x1e:
		c.push16(c.ds)
		c.ip++
		return nil
	case 0x1f:
		c.ds = c.pop16()
		c.ip++
		return nil
	case 0x24:
		imm := c.rb(csip + 1)
		res := c.reg8(0) & imm
		c.setReg8(0, res)
		c.setLogicFlags8(res)
		c.ip += 2
		return nil
	case 0x25:
		imm := c.u16(csip + 1)
		res := c.ax & imm
		c.ax = res
		c.setLogicFlags16(res)
		c.ip += 3
		return nil
	case 0x27:
		c.execDAA()
		c.ip++
		return nil
	case 0x2f:
		c.execDAS()
		c.ip++
		return nil
	case 0x37:
		c.execAAA()
		c.ip++
		return nil
	case 0x3f:
		c.execAAS()
		c.ip++
		return nil
	case 0x90:
		c.ip++
		return nil
	case 0xcd:
		vec := c.rb(csip + 1)
		c.ip += 2
		return c.handleInt(vec)
	case 0xcc:
		c.ip++
		return c.handleInt(0x03)
	case 0xcf:
		c.ip = c.pop16()
		c.cs = c.pop16()
		c.setFlagsWord(c.pop16())
		return nil
	case 0xce:
		c.ip++
		if c.of {
			return c.handleInt(0x04)
		}
		return nil
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
	case 0xea:
		c.ip = c.u16(csip + 1)
		c.cs = c.u16(csip + 3)
		return nil
	case 0xe0:
		rel := int8(c.rb(csip + 1))
		c.cx--
		c.ip += 2
		if c.cx != 0 && !c.zf {
			c.ip = uint16(int32(c.ip) + int32(rel))
		}
		return nil
	case 0xe1:
		rel := int8(c.rb(csip + 1))
		c.cx--
		c.ip += 2
		if c.cx != 0 && c.zf {
			c.ip = uint16(int32(c.ip) + int32(rel))
		}
		return nil
	case 0xe2:
		rel := int8(c.rb(csip + 1))
		c.cx--
		c.ip += 2
		if c.cx != 0 {
			c.ip = uint16(int32(c.ip) + int32(rel))
		}
		return nil
	case 0xe3:
		rel := int8(c.rb(csip + 1))
		c.ip += 2
		if c.cx == 0 {
			c.ip = uint16(int32(c.ip) + int32(rel))
		}
		return nil
	case 0xe4:
		c.setReg8(0, 0xff)
		c.ip += 2
		return nil
	case 0xe5:
		c.ax = 0xffff
		c.ip += 2
		return nil
	case 0xe6:
		c.ip += 2
		return nil
	case 0xe7:
		c.ip += 2
		return nil
	case 0xec:
		c.setReg8(0, 0xff)
		c.ip++
		return nil
	case 0xed:
		c.ax = 0xffff
		c.ip++
		return nil
	case 0xee:
		c.ip++
		return nil
	case 0xef:
		c.ip++
		return nil
	case 0xf4:
		c.ip++
		return nil
	case 0xf5:
		c.cf = !c.cf
		c.ip++
		return nil
	case 0xf8:
		c.cf = false
		c.ip++
		return nil
	case 0xf9:
		c.cf = true
		c.ip++
		return nil
	case 0xfa:
		c.extraFlags &^= 1 << 9
		c.ip++
		return nil
	case 0xfb:
		c.extraFlags |= 1 << 9
		c.ip++
		return nil
	case 0xfc:
		c.extraFlags &^= 1 << 10
		c.ip++
		return nil
	case 0xfd:
		c.extraFlags |= 1 << 10
		c.ip++
		return nil
	case 0xfe:
		return c.execFE(csip)
	case 0xf6:
		return c.execF6(csip, repPrefix != 0)
	case 0xff:
		return c.execFF(csip)
	case 0xeb:
		rel := int8(c.rb(csip + 1))
		c.ip += 2
		c.ip = uint16(int32(c.ip) + int32(rel))
		return nil
	case 0xc2:
		imm := c.u16(csip + 1)
		c.ip = c.pop16()
		c.sp += imm
		return nil
	case 0xc3:
		c.ip = c.pop16()
		return nil
	case 0xca:
		imm := c.u16(csip + 1)
		c.ip = c.pop16()
		c.cs = c.pop16()
		c.sp += imm
		return nil
	case 0xc9:
		c.sp = c.bp
		c.bp = c.pop16()
		c.ip++
		return nil
	case 0xcb:
		c.ip = c.pop16()
		c.cs = c.pop16()
		return nil
	case 0xc4:
		return c.execC4(csip)
	case 0xc5:
		return c.execC5(csip)
	case 0xc6:
		return c.execC6(csip)
	case 0xc7:
		return c.execC7(csip)
	case 0xd0:
		return c.execD0(csip)
	case 0xd1:
		return c.execD1(csip)
	case 0xd2:
		return c.execD2(csip)
	case 0xd3:
		return c.execD3(csip)
	case 0xd4:
		base := c.rb(csip + 1)
		c.ip += 2
		if base == 0 {
			c.cf = false
			c.of = false
			c.af = false
			c.setSZP8(0)
			return c.handleInt(0x00)
		}
		al := c.reg8(0)
		ah := al / base
		lo := al % base
		c.ax = uint16(ah)<<8 | uint16(lo)
		c.setSZP8(lo)
		return nil
	case 0xd5:
		base := c.rb(csip + 1)
		lo := byte(uint16(c.reg8(4))*uint16(base) + uint16(c.reg8(0)))
		c.ax = uint16(lo)
		c.setSZP8(lo)
		c.ip += 2
		return nil
	case 0xd7:
		seg := c.ds
		if c.segOverrideSet {
			seg = c.segOverride
		}
		off := c.bx + uint16(c.reg8(0))
		c.setReg8(0, c.rb(linear(seg, off)))
		c.ip++
		return nil
	case 0xd8, 0xd9, 0xda, 0xdb, 0xdc, 0xdd, 0xde, 0xdf:
		return c.execESC(csip)
	case 0x31:
		return c.exec31(csip)
	case 0x01:
		return c.exec01(csip)
	case 0x02:
		return c.exec02(csip)
	case 0x03:
		return c.exec03(csip)
	case 0x29:
		return c.exec29(csip)
	case 0x28:
		return c.exec28(csip)
	case 0x2a:
		return c.exec2A(csip)
	case 0x2b:
		return c.exec2B(csip)
	case 0x2c:
		imm := c.rb(csip + 1)
		a := c.reg8(0)
		res := a - imm
		c.setReg8(0, res)
		c.setSubFlags8(a, imm, res)
		c.ip += 2
		return nil
	case 0x2d:
		imm := c.u16(csip + 1)
		a := c.ax
		res := a - imm
		c.ax = res
		c.setSubFlags16(a, imm, res)
		c.ip += 3
		return nil
	case 0x30:
		return c.exec30(csip)
	case 0x09:
		return c.exec09(csip)
	case 0x32:
		return c.exec32(csip)
	case 0x33:
		return c.exec33(csip)
	case 0x34:
		imm := c.rb(csip + 1)
		res := c.reg8(0) ^ imm
		c.setReg8(0, res)
		c.setLogicFlags8(res)
		c.ip += 2
		return nil
	case 0x35:
		imm := c.u16(csip + 1)
		res := c.ax ^ imm
		c.ax = res
		c.setLogicFlags16(res)
		c.ip += 3
		return nil
	case 0x20:
		return c.exec20(csip)
	case 0x21:
		return c.exec21(csip)
	case 0x22:
		return c.exec22(csip)
	case 0x23:
		return c.exec23(csip)
	case 0x38:
		return c.exec38(csip)
	case 0x39:
		return c.exec39(csip)
	case 0x3a:
		return c.exec3A(csip)
	case 0x3b:
		return c.exec3B(csip)
	case 0x3c:
		imm := c.rb(csip + 1)
		a := c.reg8(0)
		res := a - imm
		c.setSubFlags8(a, imm, res)
		c.ip += 2
		return nil
	case 0x3d:
		imm := c.u16(csip + 1)
		a := c.ax
		res := a - imm
		c.setSubFlags16(a, imm, res)
		c.ip += 3
		return nil
	case 0x84:
		return c.exec84(csip)
	case 0x85:
		return c.exec85(csip)
	case 0x86:
		return c.exec86(csip)
	case 0x87:
		return c.exec87(csip)
	case 0x80:
		return c.exec80(csip)
	case 0x82:
		return c.exec80(csip)
	case 0x83:
		return c.exec83(csip)
	case 0x81:
		return c.exec81(csip)
	case 0x91, 0x92, 0x93, 0x94, 0x95, 0x96, 0x97:
		return c.exec90to97(op)
	case 0x98:
		return c.exec98(csip)
	case 0x99:
		return c.exec99(csip)
	case 0x9a:
		return c.exec9A(csip)
	case 0x9c:
		c.push16(c.flagsWord())
		c.ip++
		return nil
	case 0x9d:
		c.setFlagsWord(c.pop16())
		c.ip++
		return nil
	case 0x9e:
		ah := byte(c.ax >> 8)
		c.sf = (ah & 0x80) != 0
		c.zf = (ah & 0x40) != 0
		c.af = (ah & 0x10) != 0
		c.pf = (ah & 0x04) != 0
		c.cf = (ah & 0x01) != 0
		c.ip++
		return nil
	case 0x9f:
		ah := byte(0x02)
		if c.sf {
			ah |= 0x80
		}
		if c.zf {
			ah |= 0x40
		}
		if c.af {
			ah |= 0x10
		}
		if c.pf {
			ah |= 0x04
		}
		if c.cf {
			ah |= 0x01
		}
		c.ax = (c.ax & 0x00ff) | (uint16(ah) << 8)
		c.ip++
		return nil
	case 0xa0:
		off := c.u16(csip + 1)
		seg := c.ds
		if c.segOverrideSet {
			seg = c.segOverride
		}
		c.setReg8(0, c.rb(linear(seg, off)))
		c.ip += 3
		return nil
	case 0xa1:
		off := c.u16(csip + 1)
		seg := c.ds
		if c.segOverrideSet {
			seg = c.segOverride
		}
		c.ax = c.u16SegOff(seg, off)
		c.ip += 3
		return nil
	case 0xa2:
		off := c.u16(csip + 1)
		seg := c.ds
		if c.segOverrideSet {
			seg = c.segOverride
		}
		c.wb(linear(seg, off), c.reg8(0))
		c.ip += 3
		return nil
	case 0xa3:
		off := c.u16(csip + 1)
		seg := c.ds
		if c.segOverrideSet {
			seg = c.segOverride
		}
		c.w16SegOff(seg, off, c.ax)
		c.ip += 3
		return nil
	case 0xa4:
		srcSeg := c.ds
		if c.segOverrideSet {
			srcSeg = c.segOverride
		}
		count := uint16(1)
		if repPrefix != 0 {
			count = c.cx
			c.cx = 0
		}
		for i := uint16(0); i < count; i++ {
			c.wb(linear(c.es, c.di), c.rb(linear(srcSeg, c.si)))
			if c.df() {
				c.si--
				c.di--
			} else {
				c.si++
				c.di++
			}
		}
		c.ip++
		return nil
	case 0xa5:
		srcSeg := c.ds
		if c.segOverrideSet {
			srcSeg = c.segOverride
		}
		count := uint16(1)
		if repPrefix != 0 {
			count = c.cx
			c.cx = 0
		}
		for i := uint16(0); i < count; i++ {
			c.w16SegOff(c.es, c.di, c.u16SegOff(srcSeg, c.si))
			if c.df() {
				c.si -= 2
				c.di -= 2
			} else {
				c.si += 2
				c.di += 2
			}
		}
		c.ip++
		return nil
	case 0xa6:
		srcSeg := c.ds
		if c.segOverrideSet {
			srcSeg = c.segOverride
		}
		step := func() {
			a := c.rb(linear(srcSeg, c.si))
			b := c.rb(linear(c.es, c.di))
			c.setSubFlags8(a, b, a-b)
			if c.df() {
				c.si--
				c.di--
			} else {
				c.si++
				c.di++
			}
		}
		if repPrefix == 0 {
			step()
		} else {
			for c.cx != 0 {
				step()
				c.cx--
				if repPrefix == 0xf3 {
					if !c.zf {
						break
					}
				} else if repPrefix == 0xf2 {
					if c.zf {
						break
					}
				}
			}
		}
		c.ip++
		return nil
	case 0xa7:
		srcSeg := c.ds
		if c.segOverrideSet {
			srcSeg = c.segOverride
		}
		step := func() {
			a := c.u16SegOff(srcSeg, c.si)
			b := c.u16SegOff(c.es, c.di)
			c.setSubFlags16(a, b, a-b)
			if c.df() {
				c.si -= 2
				c.di -= 2
			} else {
				c.si += 2
				c.di += 2
			}
		}
		if repPrefix == 0 {
			step()
		} else {
			for c.cx != 0 {
				step()
				c.cx--
				if repPrefix == 0xf3 {
					if !c.zf {
						break
					}
				} else if repPrefix == 0xf2 {
					if c.zf {
						break
					}
				}
			}
		}
		c.ip++
		return nil
	case 0xa8:
		imm := c.rb(csip + 1)
		c.setLogicFlags8(c.reg8(0) & imm)
		c.ip += 2
		return nil
	case 0xa9:
		imm := c.u16(csip + 1)
		c.setLogicFlags16(c.ax & imm)
		c.ip += 3
		return nil
	case 0xaa:
		count := uint16(1)
		if repPrefix != 0 {
			count = c.cx
			c.cx = 0
		}
		for i := uint16(0); i < count; i++ {
			c.wb(linear(c.es, c.di), c.reg8(0))
			if c.df() {
				c.di--
			} else {
				c.di++
			}
		}
		c.ip++
		return nil
	case 0xab:
		count := uint16(1)
		if repPrefix != 0 {
			count = c.cx
			c.cx = 0
		}
		for i := uint16(0); i < count; i++ {
			c.w16SegOff(c.es, c.di, c.ax)
			if c.df() {
				c.di -= 2
			} else {
				c.di += 2
			}
		}
		c.ip++
		return nil
	case 0xac:
		srcSeg := c.ds
		if c.segOverrideSet {
			srcSeg = c.segOverride
		}
		count := uint16(1)
		if repPrefix != 0 {
			count = c.cx
			c.cx = 0
		}
		for i := uint16(0); i < count; i++ {
			c.setReg8(0, c.rb(linear(srcSeg, c.si)))
			if c.df() {
				c.si--
			} else {
				c.si++
			}
		}
		c.ip++
		return nil
	case 0xad:
		srcSeg := c.ds
		if c.segOverrideSet {
			srcSeg = c.segOverride
		}
		count := uint16(1)
		if repPrefix != 0 {
			count = c.cx
			c.cx = 0
		}
		for i := uint16(0); i < count; i++ {
			c.ax = c.u16SegOff(srcSeg, c.si)
			if c.df() {
				c.si -= 2
			} else {
				c.si += 2
			}
		}
		c.ip++
		return nil
	case 0xae:
		step := func() {
			a := c.reg8(0)
			b := c.rb(linear(c.es, c.di))
			c.setSubFlags8(a, b, a-b)
			if c.df() {
				c.di--
			} else {
				c.di++
			}
		}
		if repPrefix == 0 {
			step()
		} else {
			for c.cx != 0 {
				step()
				c.cx--
				if repPrefix == 0xf3 {
					if !c.zf {
						break
					}
				} else if repPrefix == 0xf2 {
					if c.zf {
						break
					}
				}
			}
		}
		c.ip++
		return nil
	case 0xaf:
		step := func() {
			a := c.ax
			b := c.u16SegOff(c.es, c.di)
			c.setSubFlags16(a, b, a-b)
			if c.df() {
				c.di -= 2
			} else {
				c.di += 2
			}
		}
		if repPrefix == 0 {
			step()
		} else {
			for c.cx != 0 {
				step()
				c.cx--
				if repPrefix == 0xf3 {
					if !c.zf {
						break
					}
				} else if repPrefix == 0xf2 {
					if c.zf {
						break
					}
				}
			}
		}
		c.ip++
		return nil
	case 0xf7:
		return c.execF7(csip, repPrefix != 0)
	case 0x89:
		return c.exec89(csip)
	case 0x8b:
		return c.exec8b(csip)
	case 0x8c:
		return c.exec8c(csip)
	case 0x8e:
		return c.exec8e(csip)
	case 0x8f:
		return c.exec8f(csip)
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

func (c *cpu) handleInt(vec byte) error {
	if vec != 0x21 || !c.dosMode {
		c.push16(c.flagsWord())
		c.push16(c.cs)
		c.push16(c.ip)
		// Hardware interrupts clear TF and IF.
		c.extraFlags &^= (1 << 8) | (1 << 9)
		base := uint32(vec) * 4
		c.ip = c.u16(base)
		c.cs = c.u16(base + 2)
		return nil
	}

	ah := byte(c.ax >> 8)
	switch ah {
	case 0x28:
		// DOS idle interrupt hook; no-op in emulator.
		c.cf = false
		return nil
	case 0x3c:
		// Create/truncate file: DS:DX=ASCIIZ path, CX=attributes (ignored).
		path, ok := c.readDOSString(c.ds, c.dx)
		if !ok {
			c.cf = true
			c.ax = 2
			return nil
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
		if err != nil {
			c.cf = true
			c.ax = 5
			return nil
		}
		h := c.allocFileHandle()
		if h == 0 {
			_ = f.Close()
			c.cf = true
			c.ax = 4 // too many open files
			return nil
		}
		c.files[h] = f
		c.ax = h
		c.cf = false
		return nil
	case 0x3d:
		// Open file: AL=mode(0 rd,1 wr,2 rdwr), DS:DX=ASCIIZ path.
		path, ok := c.readDOSString(c.ds, c.dx)
		if !ok {
			c.cf = true
			c.ax = 2
			return nil
		}
		mode := int(c.ax & 0x00ff)
		flags := os.O_RDONLY
		if mode == 1 {
			flags = os.O_WRONLY
		} else if mode == 2 {
			flags = os.O_RDWR
		}
		f, err := os.OpenFile(path, flags, 0)
		if err != nil {
			c.cf = true
			c.ax = 2
			return nil
		}
		h := c.allocFileHandle()
		if h == 0 {
			_ = f.Close()
			c.cf = true
			c.ax = 4 // too many open files
			return nil
		}
		c.files[h] = f
		c.ax = h
		c.cf = false
		return nil
	case 0x3e:
		// Close file: BX=handle.
		h := c.bx
		f, ok := c.files[h]
		if !ok {
			c.cf = true
			c.ax = 6
			return nil
		}
		_ = f.Close()
		delete(c.files, h)
		c.ax = 0
		c.cf = false
		return nil
	case 0x3f:
		// Read file: BX=handle, CX=count, DS:DX=buffer.
		h := c.bx
		f, ok := c.files[h]
		if !ok {
			c.cf = true
			c.ax = 6
			return nil
		}
		count := int(c.cx)
		if count < 0 {
			count = 0
		}
		buf := make([]byte, count)
		n, err := f.Read(buf)
		if n > 0 {
			dst := linear(c.ds, c.dx)
			for i := 0; i < n; i++ {
				c.wb(dst+uint32(i), buf[i])
			}
		}
		if err != nil && !errors.Is(err, io.EOF) {
			c.cf = true
			c.ax = 5
			return nil
		}
		c.ax = uint16(n)
		c.cf = false
		return nil
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
			c.ax = count
			c.cf = false
		case 2:
			_, _ = os.Stderr.Write(data)
			c.ax = count
			c.cf = false
		default:
			f, ok := c.files[h]
			if !ok {
				c.cf = true
				c.ax = 6
				return nil
			}
			n, err := f.Write(data)
			if err != nil {
				c.cf = true
				c.ax = 5
				return nil
			}
			c.ax = uint16(n)
			c.cf = false
		}
		c.writes++
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
}

func (c *cpu) execDAA() {
	al := c.reg8(0)
	old := al
	oldCF := c.cf
	oldAF := c.af

	if (al&0x0f) > 9 || c.af {
		al += 0x06
		c.af = true
	} else {
		c.af = false
	}

	if oldCF || old > 0x9f || (!oldAF && old > 0x99) {
		al += 0x60
		c.cf = true
	} else {
		c.cf = false
	}

	c.setReg8(0, al)
	c.zf = al == 0
	c.sf = (al & 0x80) != 0
}

func (c *cpu) execDAS() {
	al := c.reg8(0)
	old := al
	oldCF := c.cf
	oldAF := c.af

	if (al&0x0f) > 9 || c.af {
		al -= 0x06
		c.af = true
	} else {
		c.af = false
	}

	if oldCF || old > 0x9f || (!oldAF && old > 0x99) {
		al -= 0x60
		c.cf = true
	} else {
		c.cf = false
	}

	c.setReg8(0, al)
	c.zf = al == 0
	c.sf = (al & 0x80) != 0
}

func (c *cpu) execAAA() {
	al := c.reg8(0)
	ah := c.reg8(4)
	if (al&0x0f) > 9 || c.af {
		al = (al + 6) & 0x0f
		ah++
		c.af = true
		c.cf = true
	} else {
		al &= 0x0f
		c.af = false
		c.cf = false
	}
	c.setReg8(0, al)
	c.setReg8(4, ah)
}

func (c *cpu) execAAS() {
	al := c.reg8(0)
	ah := c.reg8(4)
	if (al&0x0f) > 9 || c.af {
		al = (al - 6) & 0x0f
		ah--
		c.af = true
		c.cf = true
	} else {
		al &= 0x0f
		c.af = false
		c.cf = false
	}
	c.setReg8(0, al)
	c.setReg8(4, ah)
}

func (c *cpu) exec00(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := (modrm >> 3) & 0x7
	rm := modrm & 0x7
	src := c.reg8(int(reg))
	if mod == 0x3 {
		v := c.reg8(int(rm))
		res := v + src
		c.setReg8(int(rm), res)
		c.setAddFlags8(v, src, res)
		c.ip += 2
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, rm, pc+2)
	if !ok {
		return fmt.Errorf("unsupported add r/m8 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	v := c.rb(addr)
	res := v + src
	c.wb(addr, res)
	c.setAddFlags8(v, src, res)
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec31(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := int(modrm & 0x7)
	if mod == 0x3 {
		v := c.reg16(rm) ^ c.reg16(reg)
		c.setReg16(rm, v)
		c.setLogicFlags16(v)
		c.ip += 2
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, byte(rm), pc+2)
	if !ok {
		return fmt.Errorf("unsupported xor r/m16,r16 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	v := c.u16(addr) ^ c.reg16(reg)
	c.w16(addr, v)
	c.setLogicFlags16(v)
	c.ip += uint16(2 + dispLen)
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

func (c *cpu) exec02(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := modrm & 0x7
	if mod == 0x3 {
		a := c.reg8(reg)
		b := c.reg8(int(rm))
		res := a + b
		c.setReg8(reg, res)
		c.setAddFlags8(a, b, res)
		c.ip += 2
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, rm, pc+2)
	if !ok {
		return fmt.Errorf("unsupported add r8,r/m8 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	a := c.reg8(reg)
	b := c.rb(addr)
	res := a + b
	c.setReg8(reg, res)
	c.setAddFlags8(a, b, res)
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec03(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := int(modrm & 0x7)
	if mod == 0x3 {
		a := c.reg16(reg)
		b := c.reg16(rm)
		res := a + b
		c.setReg16(reg, res)
		c.setAddFlags16(a, b, res)
		c.ip += 2
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, byte(rm), pc+2)
	if !ok {
		return fmt.Errorf("unsupported add r16,r/m16 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	a := c.reg16(reg)
	b := c.u16(addr)
	res := a + b
	c.setReg16(reg, res)
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

func (c *cpu) exec28(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := int(modrm & 0x7)
	if mod == 0x3 {
		a := c.reg8(rm)
		b := c.reg8(reg)
		res := a - b
		c.setReg8(rm, res)
		c.setSubFlags8(a, b, res)
		c.ip += 2
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, byte(rm), pc+2)
	if !ok {
		return fmt.Errorf("unsupported sub r/m8,r8 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	a := c.rb(addr)
	b := c.reg8(reg)
	res := a - b
	c.wb(addr, res)
	c.setSubFlags8(a, b, res)
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec2A(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := int(modrm & 0x7)
	if mod == 0x3 {
		a := c.reg8(reg)
		b := c.reg8(rm)
		res := a - b
		c.setReg8(reg, res)
		c.setSubFlags8(a, b, res)
		c.ip += 2
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, byte(rm), pc+2)
	if !ok {
		return fmt.Errorf("unsupported sub r8,r/m8 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	a := c.reg8(reg)
	b := c.rb(addr)
	res := a - b
	c.setReg8(reg, res)
	c.setSubFlags8(a, b, res)
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec2B(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := int(modrm & 0x7)
	if mod == 0x3 {
		a := c.reg16(reg)
		b := c.reg16(rm)
		res := a - b
		c.setReg16(reg, res)
		c.setSubFlags16(a, b, res)
		c.ip += 2
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, byte(rm), pc+2)
	if !ok {
		return fmt.Errorf("unsupported sub r16,r/m16 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	a := c.reg16(reg)
	b := c.u16(addr)
	res := a - b
	c.setReg16(reg, res)
	c.setSubFlags16(a, b, res)
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec09(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := int(modrm & 0x7)
	if mod == 0x3 {
		v := c.reg16(rm) | c.reg16(reg)
		c.setReg16(rm, v)
		c.setLogicFlags16(v)
		c.ip += 2
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, byte(rm), pc+2)
	if !ok {
		return fmt.Errorf("unsupported or r/m16,r16 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	v := c.u16(addr) | c.reg16(reg)
	c.w16(addr, v)
	c.setLogicFlags16(v)
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec08(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := int(modrm & 0x7)
	if mod == 0x3 {
		v := c.reg8(rm) | c.reg8(reg)
		c.setReg8(rm, v)
		c.setLogicFlags8(v)
		c.ip += 2
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, byte(rm), pc+2)
	if !ok {
		return fmt.Errorf("unsupported or r/m8,r8 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	v := c.rb(addr) | c.reg8(reg)
	c.wb(addr, v)
	c.setLogicFlags8(v)
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec0A(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := int(modrm & 0x7)
	if mod == 0x3 {
		v := c.reg8(reg) | c.reg8(rm)
		c.setReg8(reg, v)
		c.setLogicFlags8(v)
		c.ip += 2
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, byte(rm), pc+2)
	if !ok {
		return fmt.Errorf("unsupported or r8,r/m8 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	v := c.reg8(reg) | c.rb(addr)
	c.setReg8(reg, v)
	c.setLogicFlags8(v)
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec0B(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := int(modrm & 0x7)
	if mod == 0x3 {
		v := c.reg16(reg) | c.reg16(rm)
		c.setReg16(reg, v)
		c.setLogicFlags16(v)
		c.ip += 2
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, byte(rm), pc+2)
	if !ok {
		return fmt.Errorf("unsupported or r16,r/m16 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	v := c.reg16(reg) | c.u16(addr)
	c.setReg16(reg, v)
	c.setLogicFlags16(v)
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec10(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := int(modrm & 0x7)
	if mod == 0x3 {
		c.setReg8(rm, c.adc8(c.reg8(rm), c.reg8(reg)))
		c.ip += 2
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, byte(rm), pc+2)
	if !ok {
		return fmt.Errorf("unsupported adc r/m8,r8 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	c.wb(addr, c.adc8(c.rb(addr), c.reg8(reg)))
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec11(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := int(modrm & 0x7)
	if mod == 0x3 {
		c.setReg16(rm, c.adc16(c.reg16(rm), c.reg16(reg)))
		c.ip += 2
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, byte(rm), pc+2)
	if !ok {
		return fmt.Errorf("unsupported adc r/m16,r16 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	c.w16(addr, c.adc16(c.u16(addr), c.reg16(reg)))
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec12(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := int(modrm & 0x7)
	if mod == 0x3 {
		c.setReg8(reg, c.adc8(c.reg8(reg), c.reg8(rm)))
		c.ip += 2
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, byte(rm), pc+2)
	if !ok {
		return fmt.Errorf("unsupported adc r8,r/m8 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	c.setReg8(reg, c.adc8(c.reg8(reg), c.rb(addr)))
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec13(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := int(modrm & 0x7)
	if mod == 0x3 {
		c.setReg16(reg, c.adc16(c.reg16(reg), c.reg16(rm)))
		c.ip += 2
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, byte(rm), pc+2)
	if !ok {
		return fmt.Errorf("unsupported adc r16,r/m16 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	c.setReg16(reg, c.adc16(c.reg16(reg), c.u16(addr)))
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec18(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := int(modrm & 0x7)
	if mod == 0x3 {
		c.setReg8(rm, c.sbb8(c.reg8(rm), c.reg8(reg)))
		c.ip += 2
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, byte(rm), pc+2)
	if !ok {
		return fmt.Errorf("unsupported sbb r/m8,r8 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	c.wb(addr, c.sbb8(c.rb(addr), c.reg8(reg)))
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec19(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := int(modrm & 0x7)
	if mod == 0x3 {
		c.setReg16(rm, c.sbb16(c.reg16(rm), c.reg16(reg)))
		c.ip += 2
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, byte(rm), pc+2)
	if !ok {
		return fmt.Errorf("unsupported sbb r/m16,r16 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	c.w16(addr, c.sbb16(c.u16(addr), c.reg16(reg)))
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec1A(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := int(modrm & 0x7)
	if mod == 0x3 {
		c.setReg8(reg, c.sbb8(c.reg8(reg), c.reg8(rm)))
		c.ip += 2
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, byte(rm), pc+2)
	if !ok {
		return fmt.Errorf("unsupported sbb r8,r/m8 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	c.setReg8(reg, c.sbb8(c.reg8(reg), c.rb(addr)))
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec1B(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := int(modrm & 0x7)
	if mod == 0x3 {
		c.setReg16(reg, c.sbb16(c.reg16(reg), c.reg16(rm)))
		c.ip += 2
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, byte(rm), pc+2)
	if !ok {
		return fmt.Errorf("unsupported sbb r16,r/m16 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	c.setReg16(reg, c.sbb16(c.reg16(reg), c.u16(addr)))
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec21(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := int(modrm & 0x7)
	if mod == 0x3 {
		v := c.reg16(rm) & c.reg16(reg)
		c.setReg16(rm, v)
		c.setLogicFlags16(v)
		c.ip += 2
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, byte(rm), pc+2)
	if !ok {
		return fmt.Errorf("unsupported and r/m16,r16 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	v := c.u16(addr) & c.reg16(reg)
	c.w16(addr, v)
	c.setLogicFlags16(v)
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec20(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := int(modrm & 0x7)
	if mod == 0x3 {
		v := c.reg8(rm) & c.reg8(reg)
		c.setReg8(rm, v)
		c.setLogicFlags8(v)
		c.ip += 2
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, byte(rm), pc+2)
	if !ok {
		return fmt.Errorf("unsupported and r/m8,r8 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	v := c.rb(addr) & c.reg8(reg)
	c.wb(addr, v)
	c.setLogicFlags8(v)
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec22(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := int(modrm & 0x7)
	if mod == 0x3 {
		v := c.reg8(reg) & c.reg8(rm)
		c.setReg8(reg, v)
		c.setLogicFlags8(v)
		c.ip += 2
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, byte(rm), pc+2)
	if !ok {
		return fmt.Errorf("unsupported and r8,r/m8 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	v := c.reg8(reg) & c.rb(addr)
	c.setReg8(reg, v)
	c.setLogicFlags8(v)
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec23(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := int(modrm & 0x7)
	if mod == 0x3 {
		v := c.reg16(reg) & c.reg16(rm)
		c.setReg16(reg, v)
		c.setLogicFlags16(v)
		c.ip += 2
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, byte(rm), pc+2)
	if !ok {
		return fmt.Errorf("unsupported and r16,r/m16 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	v := c.reg16(reg) & c.u16(addr)
	c.setReg16(reg, v)
	c.setLogicFlags16(v)
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec39(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := int(modrm & 0x7)
	if mod == 0x3 {
		a := c.reg16(rm)
		b := c.reg16(reg)
		res := a - b
		c.setSubFlags16(a, b, res)
		c.ip += 2
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, byte(rm), pc+2)
	if !ok {
		return fmt.Errorf("unsupported cmp r/m16,r16 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	a := c.u16(addr)
	b := c.reg16(reg)
	res := a - b
	c.setSubFlags16(a, b, res)
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec30(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := int(modrm & 0x7)
	if mod == 0x3 {
		v := c.reg8(rm) ^ c.reg8(reg)
		c.setReg8(rm, v)
		c.setLogicFlags8(v)
		c.ip += 2
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, byte(rm), pc+2)
	if !ok {
		return fmt.Errorf("unsupported xor r/m8,r8 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	v := c.rb(addr) ^ c.reg8(reg)
	c.wb(addr, v)
	c.setLogicFlags8(v)
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec32(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := int(modrm & 0x7)
	if mod == 0x3 {
		v := c.reg8(reg) ^ c.reg8(rm)
		c.setReg8(reg, v)
		c.setLogicFlags8(v)
		c.ip += 2
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, byte(rm), pc+2)
	if !ok {
		return fmt.Errorf("unsupported xor r8,r/m8 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	v := c.reg8(reg) ^ c.rb(addr)
	c.setReg8(reg, v)
	c.setLogicFlags8(v)
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec33(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := int(modrm & 0x7)
	if mod == 0x3 {
		v := c.reg16(reg) ^ c.reg16(rm)
		c.setReg16(reg, v)
		c.setLogicFlags16(v)
		c.ip += 2
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, byte(rm), pc+2)
	if !ok {
		return fmt.Errorf("unsupported xor r16,r/m16 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	v := c.reg16(reg) ^ c.u16(addr)
	c.setReg16(reg, v)
	c.setLogicFlags16(v)
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec38(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := int(modrm & 0x7)
	if mod == 0x3 {
		a := c.reg8(rm)
		b := c.reg8(reg)
		res := a - b
		c.setSubFlags8(a, b, res)
		c.ip += 2
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, byte(rm), pc+2)
	if !ok {
		return fmt.Errorf("unsupported cmp r/m8,r8 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	a := c.rb(addr)
	b := c.reg8(reg)
	res := a - b
	c.setSubFlags8(a, b, res)
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec3A(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := int(modrm & 0x7)
	if mod == 0x3 {
		a := c.reg8(reg)
		b := c.reg8(rm)
		res := a - b
		c.setSubFlags8(a, b, res)
		c.ip += 2
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, byte(rm), pc+2)
	if !ok {
		return fmt.Errorf("unsupported cmp r8,r/m8 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	a := c.reg8(reg)
	b := c.rb(addr)
	res := a - b
	c.setSubFlags8(a, b, res)
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec3B(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := int(modrm & 0x7)
	if mod == 0x3 {
		a := c.reg16(reg)
		b := c.reg16(rm)
		res := a - b
		c.setSubFlags16(a, b, res)
		c.ip += 2
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, byte(rm), pc+2)
	if !ok {
		return fmt.Errorf("unsupported cmp r16,r/m16 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	a := c.reg16(reg)
	b := c.u16(addr)
	res := a - b
	c.setSubFlags16(a, b, res)
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec84(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := int(modrm & 0x7)
	if mod == 0x3 {
		res := c.reg8(rm) & c.reg8(reg)
		c.setLogicFlags8(res)
		c.ip += 2
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, byte(rm), pc+2)
	if !ok {
		return fmt.Errorf("unsupported test r/m8,r8 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	res := c.rb(addr) & c.reg8(reg)
	c.setLogicFlags8(res)
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec85(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := int(modrm & 0x7)
	if mod == 0x3 {
		res := c.reg16(rm) & c.reg16(reg)
		c.setLogicFlags16(res)
		c.ip += 2
		return nil
	}
	seg, off, ok, dispLen := c.ea16SegOff(mod, byte(rm), pc+2)
	if !ok {
		return fmt.Errorf("unsupported test r/m16,r16 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	res := c.u16SegOff(seg, off) & c.reg16(reg)
	c.setLogicFlags16(res)
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec86(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := int(modrm & 0x7)
	if mod == 0x3 {
		a := c.reg8(rm)
		b := c.reg8(reg)
		c.setReg8(rm, b)
		c.setReg8(reg, a)
		c.ip += 2
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, byte(rm), pc+2)
	if !ok {
		return fmt.Errorf("unsupported xchg r/m8,r8 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	a := c.rb(addr)
	b := c.reg8(reg)
	c.wb(addr, b)
	c.setReg8(reg, a)
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec87(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := int(modrm & 0x7)
	if mod == 0x3 {
		a := c.reg16(rm)
		b := c.reg16(reg)
		c.setReg16(rm, b)
		c.setReg16(reg, a)
		c.ip += 2
		return nil
	}
	seg, off, ok, dispLen := c.ea16SegOff(mod, byte(rm), pc+2)
	if !ok {
		return fmt.Errorf("unsupported xchg r/m16,r16 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	a := c.u16SegOff(seg, off)
	b := c.reg16(reg)
	c.w16SegOff(seg, off, b)
	c.setReg16(reg, a)
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec80(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	op := (modrm >> 3) & 0x7
	rm := int(modrm & 0x7)
	immOff := pc + 2
	if mod == 0x3 {
		v := c.reg8(rm)
		res, write, err := c.group1_8(op, v, c.rb(immOff))
		if err != nil {
			return fmt.Errorf("unsupported 80 /%d at %04x:%04x", op, c.cs, c.ip)
		}
		if write {
			c.setReg8(rm, res)
		}
		c.ip += 3
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, byte(rm), immOff)
	if !ok {
		return fmt.Errorf("unsupported 80 memory form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	immOff += uint32(dispLen)
	v := c.rb(addr)
	res, write, err := c.group1_8(op, v, c.rb(immOff))
	if err != nil {
		return fmt.Errorf("unsupported 80 /%d at %04x:%04x", op, c.cs, c.ip)
	}
	if write {
		c.wb(addr, res)
	}
	c.ip += uint16(3 + dispLen)
	return nil
}

func (c *cpu) exec83(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	op := (modrm >> 3) & 0x7
	rm := int(modrm & 0x7)
	immOff := pc + 2
	imm := uint16(int16(int8(c.rb(immOff))))
	if mod == 0x3 {
		v := c.reg16(rm)
		res, write, err := c.group1_16(op, v, imm)
		if err != nil {
			return fmt.Errorf("unsupported 83 /%d at %04x:%04x", op, c.cs, c.ip)
		}
		if write {
			c.setReg16(rm, res)
		}
		c.ip += 3
		return nil
	}
	seg, off, ok, dispLen := c.ea16SegOff(mod, byte(rm), immOff)
	if !ok {
		return fmt.Errorf("unsupported 83 memory form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	imm = uint16(int16(int8(c.rb(immOff + uint32(dispLen)))))
	v := c.u16SegOff(seg, off)
	res, write, err := c.group1_16(op, v, imm)
	if err != nil {
		return fmt.Errorf("unsupported 83 /%d at %04x:%04x", op, c.cs, c.ip)
	}
	if write {
		c.w16SegOff(seg, off, res)
	}
	c.ip += uint16(3 + dispLen)
	return nil
}

func (c *cpu) exec81(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	op := (modrm >> 3) & 0x7
	rm := int(modrm & 0x7)
	immOff := pc + 2
	if mod == 0x3 {
		v := c.reg16(rm)
		imm := c.u16(immOff)
		res, write, err := c.group1_16(op, v, imm)
		if err != nil {
			return fmt.Errorf("unsupported 81 /%d at %04x:%04x", op, c.cs, c.ip)
		}
		if write {
			c.setReg16(rm, res)
		}
		c.ip += 4
		return nil
	}
	seg, off, ok, dispLen := c.ea16SegOff(mod, byte(rm), immOff)
	if !ok {
		return fmt.Errorf("unsupported 81 memory form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	imm := c.u16(immOff + uint32(dispLen))
	v := c.u16SegOff(seg, off)
	res, write, err := c.group1_16(op, v, imm)
	if err != nil {
		return fmt.Errorf("unsupported 81 /%d at %04x:%04x", op, c.cs, c.ip)
	}
	if write {
		c.w16SegOff(seg, off, res)
	}
	c.ip += uint16(4 + dispLen)
	return nil
}

func (c *cpu) group1_8(op byte, a, b byte) (byte, bool, error) {
	switch op {
	case 0: // add
		res := a + b
		c.setAddFlags8(a, b, res)
		return res, true, nil
	case 1: // or
		res := a | b
		c.setLogicFlags8(res)
		return res, true, nil
	case 2: // adc
		return c.adc8(a, b), true, nil
	case 3: // sbb
		return c.sbb8(a, b), true, nil
	case 4: // and
		res := a & b
		c.setLogicFlags8(res)
		return res, true, nil
	case 5: // sub
		res := a - b
		c.setSubFlags8(a, b, res)
		return res, true, nil
	case 6: // xor
		res := a ^ b
		c.setLogicFlags8(res)
		return res, true, nil
	case 7: // cmp
		res := a - b
		c.setSubFlags8(a, b, res)
		return 0, false, nil
	default:
		return 0, false, fmt.Errorf("unsupported")
	}
}

func (c *cpu) group1_16(op byte, a, b uint16) (uint16, bool, error) {
	switch op {
	case 0: // add
		res := a + b
		c.setAddFlags16(a, b, res)
		return res, true, nil
	case 1: // or
		res := a | b
		c.setLogicFlags16(res)
		return res, true, nil
	case 2: // adc
		return c.adc16(a, b), true, nil
	case 3: // sbb
		return c.sbb16(a, b), true, nil
	case 4: // and
		res := a & b
		c.setLogicFlags16(res)
		return res, true, nil
	case 7: // cmp
		res := a - b
		c.setSubFlags16(a, b, res)
		return 0, false, nil
	case 5: // sub
		res := a - b
		c.setSubFlags16(a, b, res)
		return res, true, nil
	case 6: // xor
		res := a ^ b
		c.setLogicFlags16(res)
		return res, true, nil
	default:
		return 0, false, fmt.Errorf("unsupported")
	}
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

func (c *cpu) exec98(pc uint32) error {
	// cbw: sign-extend AL into AX.
	if (c.ax & 0x0080) != 0 {
		c.ax = (c.ax & 0x00ff) | 0xff00
	} else {
		c.ax &= 0x00ff
	}
	c.ip++
	return nil
}

func (c *cpu) exec9A(pc uint32) error {
	off := c.u16(pc + 1)
	seg := c.u16(pc + 3)
	ret := c.ip + 5
	c.push16(c.cs)
	c.push16(ret)
	c.cs = seg
	c.ip = off
	return nil
}

func (c *cpu) execC4(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := modrm & 0x7
	if mod == 0x3 {
		return fmt.Errorf("unsupported les register form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	segMem, offMem, ok, dispLen := c.ea16SegOff(mod, rm, pc+2)
	if !ok {
		return fmt.Errorf("unsupported les form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	c.setReg16(reg, c.u16SegOff(segMem, offMem))
	c.es = c.u16SegOff(segMem, offMem+2)
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) execC5(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := modrm & 0x7
	if mod == 0x3 {
		return fmt.Errorf("unsupported lds register form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	segMem, offMem, ok, dispLen := c.ea16SegOff(mod, rm, pc+2)
	if !ok {
		return fmt.Errorf("unsupported lds form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	c.setReg16(reg, c.u16SegOff(segMem, offMem))
	c.ds = c.u16SegOff(segMem, offMem+2)
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) execC6(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	rm := int(modrm & 0x7)
	immOff := pc + 2
	if mod == 0x3 {
		c.setReg8(rm, c.rb(immOff))
		c.ip += 3
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, byte(rm), immOff)
	if !ok {
		return fmt.Errorf("unsupported mov r/m8,imm8 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	c.wb(addr, c.rb(immOff+uint32(dispLen)))
	c.ip += uint16(3 + dispLen)
	return nil
}

func (c *cpu) execC7(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	rm := int(modrm & 0x7)
	immOff := pc + 2
	if mod == 0x3 {
		c.setReg16(rm, c.u16(immOff))
		c.ip += 4
		return nil
	}
	segMem, offMem, ok, dispLen := c.ea16SegOff(mod, byte(rm), immOff)
	if !ok {
		return fmt.Errorf("unsupported mov r/m16,imm16 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	c.w16SegOff(segMem, offMem, c.u16(immOff+uint32(dispLen)))
	c.ip += uint16(4 + dispLen)
	return nil
}

func (c *cpu) execF6(pc uint32, repPrefixed bool) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	subop := (modrm >> 3) & 0x7
	rm := int(modrm & 0x7)

	var v byte
	var addr uint32
	var dispLen int
	if mod == 0x3 {
		v = c.reg8(rm)
	} else {
		a, ok, d := c.ea16(mod, byte(rm), pc+2)
		if !ok {
			return fmt.Errorf("unsupported f6 memory form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
		}
		addr = a
		dispLen = d
		v = c.rb(addr)
	}
	insnLen := uint16(2 + dispLen)
	immOff := pc + 2 + uint32(dispLen)

	write8 := func(x byte) {
		if mod == 0x3 {
			c.setReg8(rm, x)
		} else {
			c.wb(addr, x)
		}
	}

	switch subop {
	case 0, 1: // test r/m8, imm8
		imm := c.rb(immOff)
		c.setLogicFlags8(v & imm)
		c.ip += insnLen + 1
		return nil
	case 2: // not
		write8(^v)
		c.ip += insnLen
		return nil
	case 3: // neg
		res := byte(0 - v)
		write8(res)
		c.zf = res == 0
		c.sf = (res & 0x80) != 0
		c.cf = v != 0
		c.of = v == 0x80
		c.ip += insnLen
		return nil
	case 4: // mul
		prod := uint16(c.reg8(0)) * uint16(v)
		c.ax = prod
		c.cf = (prod >> 8) != 0
		c.of = c.cf
		c.ip += insnLen
		return nil
	case 5: // imul
		prod := int16(int8(c.reg8(0))) * int16(int8(v))
		c.ax = uint16(prod)
		hi := byte(uint16(prod) >> 8)
		sign := byte(0x00)
		if (byte(prod) & 0x80) != 0 {
			sign = 0xff
		}
		overflow := hi != sign
		c.cf = overflow
		c.of = overflow
		c.ip += insnLen
		return nil
	case 6: // div
		if v == 0 {
			ah := c.reg8(4)
			c.setSubFlags8(ah, v, ah-v)
			c.ip += insnLen
			return c.handleInt(0x00)
		}
		dividend := uint16(c.ax)
		q := dividend / uint16(v)
		r := dividend % uint16(v)
		if q > 0xff {
			ah := c.reg8(4)
			c.setSubFlags8(ah, v, ah-v)
			c.ip += insnLen
			return c.handleInt(0x00)
		}
		c.setReg8(0, byte(q))
		c.setReg8(4, byte(r))
		c.ip += insnLen
		return nil
	case 7: // idiv
		divisor := int16(int8(v))
		absDiv := byte(v)
		if int8(v) < 0 {
			absDiv = byte(-int8(v))
		}
		absAX := c.ax
		if int16(c.ax) < 0 {
			absAX = uint16(-int16(c.ax))
		}
		cmpA := byte(absAX >> 8)
		if divisor == 0 {
			c.setSubFlags8(cmpA, absDiv, cmpA-absDiv)
			c.cf = false
			c.of = false
			c.ip += insnLen
			return c.handleInt(0x00)
		}
		dividend := int16(c.ax)
		q := dividend / divisor
		r := dividend % divisor
		if q <= -128 || q > 127 {
			c.setSubFlags8(cmpA, absDiv, cmpA-absDiv)
			c.cf = false
			c.of = false
			c.ip += insnLen
			return c.handleInt(0x00)
		}
		q8 := int8(q)
		if repPrefixed {
			q8 = -q8
		}
		c.setReg8(0, byte(q8))
		c.setReg8(4, byte(int8(r)))
		c.ip += insnLen
		return nil
	default:
		return fmt.Errorf("unsupported f6 /%d at %04x:%04x", subop, c.cs, c.ip)
	}
}

func (c *cpu) execFE(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	subop := (modrm >> 3) & 0x7
	rm := int(modrm & 0x7)

	if subop != 0 && subop != 1 {
		return fmt.Errorf("unsupported fe /%d at %04x:%04x", subop, c.cs, c.ip)
	}

	if mod == 0x3 {
		v := c.reg8(rm)
		if subop == 0 {
			res := v + 1
			c.setReg8(rm, res)
			c.setIncFlags8(v, res)
		} else {
			res := v - 1
			c.setReg8(rm, res)
			c.setDecFlags8(v, res)
		}
		c.ip += 2
		return nil
	}

	addr, ok, dispLen := c.ea16(mod, byte(rm), pc+2)
	if !ok {
		return fmt.Errorf("unsupported fe memory form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	v := c.rb(addr)
	if subop == 0 {
		res := v + 1
		c.wb(addr, res)
		c.setIncFlags8(v, res)
	} else {
		res := v - 1
		c.wb(addr, res)
		c.setDecFlags8(v, res)
	}
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) execFF(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	subop := (modrm >> 3) & 0x7
	rm := int(modrm & 0x7)

	dispLen := 0
	getRM16 := func() (uint16, bool) {
		if mod == 0x3 {
			return c.reg16(rm), true
		}
		seg, off, ok, d := c.ea16SegOff(mod, byte(rm), pc+2)
		if !ok {
			return 0, false
		}
		dispLen = d
		return c.u16SegOff(seg, off), true
	}
	setRM16 := func(v uint16) bool {
		if mod == 0x3 {
			c.setReg16(rm, v)
			return true
		}
		seg, off, ok, d := c.ea16SegOff(mod, byte(rm), pc+2)
		if !ok {
			return false
		}
		dispLen = d
		c.w16SegOff(seg, off, v)
		return true
	}
	getFarPtr := func() (uint16, uint16, bool) {
		if mod == 0x3 {
			return 0, 0, false
		}
		seg, off, ok, d := c.ea16SegOff(mod, byte(rm), pc+2)
		if !ok {
			return 0, 0, false
		}
		dispLen = d
		return c.u16SegOff(seg, off), c.u16SegOff(seg, off+2), true
	}

	switch subop {
	case 0: // inc r/m16
		v, ok := getRM16()
		if !ok {
			return fmt.Errorf("unsupported ff /0 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
		}
		res := v + 1
		if !setRM16(res) {
			return fmt.Errorf("unsupported ff /0 writeback modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
		}
		c.setIncFlags16(v, res)
		c.ip += uint16(2 + dispLen)
		return nil
	case 1: // dec r/m16
		v, ok := getRM16()
		if !ok {
			return fmt.Errorf("unsupported ff /1 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
		}
		res := v - 1
		if !setRM16(res) {
			return fmt.Errorf("unsupported ff /1 writeback modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
		}
		c.setDecFlags16(v, res)
		c.ip += uint16(2 + dispLen)
		return nil
	case 2: // call r/m16 (near absolute)
		target, ok := getRM16()
		if !ok {
			return fmt.Errorf("unsupported ff /2 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
		}
		ret := c.ip + uint16(2+dispLen)
		c.push16(ret)
		c.ip = target
		return nil
	case 3: // call m16:16 (far absolute)
		off, seg, ok := getFarPtr()
		if !ok {
			return fmt.Errorf("unsupported ff /3 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
		}
		ret := c.ip + uint16(2+dispLen)
		c.push16(c.cs)
		c.push16(ret)
		c.cs = seg
		c.ip = off
		return nil
	case 4: // jmp r/m16 (near absolute)
		target, ok := getRM16()
		if !ok {
			return fmt.Errorf("unsupported ff /4 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
		}
		c.ip = target
		return nil
	case 5: // jmp m16:16 (far absolute)
		off, seg, ok := getFarPtr()
		if !ok {
			return fmt.Errorf("unsupported ff /5 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
		}
		c.cs = seg
		c.ip = off
		return nil
	case 6, 7: // push r/m16 (8086 also aliases /7 to push)
		v, ok := getRM16()
		if !ok {
			return fmt.Errorf("unsupported ff /%d form modrm=%02x at %04x:%04x", subop, modrm, c.cs, c.ip)
		}
		if mod == 0x3 && rm == 4 {
			// 8086 quirk: PUSH SP pushes post-decrement SP value.
			v = c.sp - 2
		}
		c.push16(v)
		c.ip += uint16(2 + dispLen)
		return nil
	default:
		return fmt.Errorf("unsupported ff /%d at %04x:%04x", subop, c.cs, c.ip)
	}
}

func (c *cpu) execD0(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	op := (modrm >> 3) & 0x7
	rm := int(modrm & 0x7)
	if mod == 0x3 {
		c.setReg8(rm, c.group2_8(op, c.reg8(rm), 1))
		c.ip += 2
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, byte(rm), pc+2)
	if !ok {
		return fmt.Errorf("unsupported d0 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	c.wb(addr, c.group2_8(op, c.rb(addr), 1))
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) execD1(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	op := (modrm >> 3) & 0x7
	rm := int(modrm & 0x7)
	if mod == 0x3 {
		c.setReg16(rm, c.group2_16(op, c.reg16(rm), 1))
		c.ip += 2
		return nil
	}
	seg, off, ok, dispLen := c.ea16SegOff(mod, byte(rm), pc+2)
	if !ok {
		return fmt.Errorf("unsupported d1 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	c.w16SegOff(seg, off, c.group2_16(op, c.u16SegOff(seg, off), 1))
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) execD2(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	op := (modrm >> 3) & 0x7
	rm := int(modrm & 0x7)
	cnt := uint(c.reg8(1))
	if mod == 0x3 {
		c.setReg8(rm, c.group2_8(op, c.reg8(rm), cnt))
		c.ip += 2
		return nil
	}
	addr, ok, dispLen := c.ea16(mod, byte(rm), pc+2)
	if !ok {
		return fmt.Errorf("unsupported d2 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	c.wb(addr, c.group2_8(op, c.rb(addr), cnt))
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) execD3(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	op := (modrm >> 3) & 0x7
	rm := int(modrm & 0x7)
	cnt := uint(c.reg8(1))
	if mod == 0x3 {
		c.setReg16(rm, c.group2_16(op, c.reg16(rm), cnt))
		c.ip += 2
		return nil
	}
	seg, off, ok, dispLen := c.ea16SegOff(mod, byte(rm), pc+2)
	if !ok {
		return fmt.Errorf("unsupported d3 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	c.w16SegOff(seg, off, c.group2_16(op, c.u16SegOff(seg, off), cnt))
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) execESC(pc uint32) error {
	// 8087 ESC opcodes (D8..DF): consume ModR/M and any displacement.
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	rm := modrm & 0x7
	if mod == 0x3 {
		c.ip += 2
		return nil
	}
	_, _, ok, dispLen := c.ea16SegOff(mod, rm, pc+2)
	if !ok {
		return fmt.Errorf("unsupported esc form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) group2_8(op, v byte, count uint) byte {
	if count == 0 {
		return v
	}
	switch op & 0x7 {
	case 0: // rol
		for i := uint(0); i < count; i++ {
			msb := (v >> 7) & 1
			v = (v << 1) | msb
			c.cf = msb != 0
		}
		c.of = ((v >> 7) & 1) != b2u(c.cf)
	case 1: // ror
		for i := uint(0); i < count; i++ {
			lsb := v & 1
			v = (v >> 1) | (lsb << 7)
			c.cf = lsb != 0
		}
		c.of = ((v>>7)&1)^((v>>6)&1) != 0
	case 2: // rcl
		for i := uint(0); i < count; i++ {
			cf := byte(0)
			if c.cf {
				cf = 1
			}
			newCF := (v & 0x80) != 0
			v = (v << 1) | cf
			c.cf = newCF
		}
		c.of = ((v >> 7) & 1) != b2u(c.cf)
	case 3: // rcr
		for i := uint(0); i < count; i++ {
			cf := byte(0)
			if c.cf {
				cf = 1
			}
			newCF := (v & 1) != 0
			v = (v >> 1) | (cf << 7)
			c.cf = newCF
		}
		c.of = ((v>>7)&1)^((v>>6)&1) != 0
	case 4, 6: // shl/sal
		for i := uint(0); i < count; i++ {
			prevMSB := (v & 0x80) != 0
			c.cf = prevMSB
			v <<= 1
			c.of = ((v & 0x80) != 0) != c.cf
		}
		c.zf = v == 0
		c.sf = (v & 0x80) != 0
	case 5: // shr
		for i := uint(0); i < count; i++ {
			prevMSB := (v & 0x80) != 0
			c.cf = (v & 1) != 0
			v >>= 1
			c.of = prevMSB
		}
		c.zf = v == 0
		c.sf = (v & 0x80) != 0
	case 7: // sar
		for i := uint(0); i < count; i++ {
			c.cf = (v & 1) != 0
			v = (v >> 1) | (v & 0x80)
			c.of = false
		}
		c.zf = v == 0
		c.sf = (v & 0x80) != 0
	}
	return v
}

func (c *cpu) group2_16(op byte, v uint16, count uint) uint16 {
	if count == 0 {
		return v
	}
	switch op & 0x7 {
	case 0: // rol
		for i := uint(0); i < count; i++ {
			msb := (v >> 15) & 1
			v = (v << 1) | msb
			c.cf = msb != 0
		}
		c.of = ((v >> 15) & 1) != uint16(b2u(c.cf))
	case 1: // ror
		for i := uint(0); i < count; i++ {
			lsb := v & 1
			v = (v >> 1) | (lsb << 15)
			c.cf = lsb != 0
		}
		c.of = ((v>>15)&1)^((v>>14)&1) != 0
	case 2: // rcl
		for i := uint(0); i < count; i++ {
			cf := uint16(0)
			if c.cf {
				cf = 1
			}
			newCF := (v & 0x8000) != 0
			v = (v << 1) | cf
			c.cf = newCF
		}
		c.of = ((v >> 15) & 1) != uint16(b2u(c.cf))
	case 3: // rcr
		for i := uint(0); i < count; i++ {
			cf := uint16(0)
			if c.cf {
				cf = 1
			}
			newCF := (v & 1) != 0
			v = (v >> 1) | (cf << 15)
			c.cf = newCF
		}
		c.of = ((v>>15)&1)^((v>>14)&1) != 0
	case 4, 6: // shl/sal
		for i := uint(0); i < count; i++ {
			prevMSB := (v & 0x8000) != 0
			c.cf = prevMSB
			v <<= 1
			c.of = ((v & 0x8000) != 0) != c.cf
		}
		c.zf = v == 0
		c.sf = (v & 0x8000) != 0
	case 5: // shr
		for i := uint(0); i < count; i++ {
			prevMSB := (v & 0x8000) != 0
			c.cf = (v & 1) != 0
			v >>= 1
			c.of = prevMSB
		}
		c.zf = v == 0
		c.sf = (v & 0x8000) != 0
	case 7: // sar
		for i := uint(0); i < count; i++ {
			c.cf = (v & 1) != 0
			v = (v >> 1) | (v & 0x8000)
			c.of = false
		}
		c.zf = v == 0
		c.sf = (v & 0x8000) != 0
	}
	return v
}

func b2u(v bool) byte {
	if v {
		return 1
	}
	return 0
}

func (c *cpu) setSZP8(v byte) {
	c.zf = v == 0
	c.sf = (v & 0x80) != 0
	c.pf = parityEven8(v)
}

func (c *cpu) setSZP16(v uint16) {
	c.zf = v == 0
	c.sf = (v & 0x8000) != 0
	c.pf = parityEven8(byte(v))
}

func parityEven8(v byte) bool {
	return bits.OnesCount8(v)%2 == 0
}

func (c *cpu) execF7(pc uint32, repPrefixed bool) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	subop := (modrm >> 3) & 0x7
	rm := int(modrm & 0x7)

	var v uint16
	var seg uint16
	var off uint16
	var dispLen int
	if mod == 0x3 {
		v = c.reg16(rm)
	} else {
		s, o, ok, d := c.ea16SegOff(mod, byte(rm), pc+2)
		if !ok {
			return fmt.Errorf("unsupported f7 memory form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
		}
		seg, off, dispLen = s, o, d
		v = c.u16SegOff(seg, off)
	}
	insnLen := uint16(2 + dispLen)
	immOff := pc + 2 + uint32(dispLen)

	write16 := func(x uint16) {
		if mod == 0x3 {
			c.setReg16(rm, x)
		} else {
			c.w16SegOff(seg, off, x)
		}
	}

	switch subop {
	case 0, 1: // test r/m16, imm16
		imm := c.u16(immOff)
		c.setLogicFlags16(v & imm)
		c.ip += insnLen + 2
		return nil
	case 2: // not
		write16(^v)
		c.ip += insnLen
		return nil
	case 3: // neg r/m16
		res := uint16(0 - int16(v))
		write16(res)
		c.zf = res == 0
		c.sf = (res & 0x8000) != 0
		c.cf = v != 0
		c.of = v == 0x8000
		c.ip += insnLen
		return nil
	case 4: // mul
		prod := uint32(c.ax) * uint32(v)
		c.ax = uint16(prod)
		c.dx = uint16(prod >> 16)
		c.cf = c.dx != 0
		c.of = c.cf
		c.ip += insnLen
		return nil
	case 5: // imul
		prod := int32(int16(c.ax)) * int32(int16(v))
		c.ax = uint16(prod)
		c.dx = uint16(prod >> 16)
		overflow := int32(int16(c.ax)) != prod
		c.cf = overflow
		c.of = overflow
		c.ip += insnLen
		return nil
	case 6: // div
		if v == 0 {
			c.setSubFlags16(c.dx, v, c.dx-v)
			c.ip += insnLen
			return c.handleInt(0x00)
		}
		dividend := uint32(c.dx)<<16 | uint32(c.ax)
		q := dividend / uint32(v)
		r := dividend % uint32(v)
		if q > 0xffff {
			c.setSubFlags16(c.dx, v, c.dx-v)
			c.ip += insnLen
			return c.handleInt(0x00)
		}
		c.ax = uint16(q)
		c.dx = uint16(r)
		c.ip += insnLen
		return nil
	case 7: // idiv r/m16
		divisor := int16(v)
		if divisor == 0 {
			c.setSubFlags16(c.dx, v, c.dx-v)
			c.ip += insnLen
			return c.handleInt(0x00)
		}
		dividend := int32(int16(c.dx))<<16 | int32(c.ax)
		q := dividend / int32(divisor)
		r := dividend % int32(divisor)
		if q <= -32768 || q > 32767 {
			c.setSubFlags16(c.dx, v, c.dx-v)
			c.ip += insnLen
			return c.handleInt(0x00)
		}
		q16 := int16(q)
		if repPrefixed {
			q16 = -q16
		}
		c.ax = uint16(q16)
		c.dx = uint16(int16(r))
		c.ip += insnLen
		return nil
	default:
		return fmt.Errorf("unsupported f7 /%d at %04x:%04x", subop, c.cs, c.ip)
	}
}

func (c *cpu) execJcc8(cc byte, pc uint32) error {
	rel := int8(c.rb(pc + 1))
	c.ip += 2
	if c.evalCC(cc) {
		c.ip = uint16(int32(c.ip) + int32(rel))
	}
	return nil
}

func (c *cpu) evalCC(cc byte) bool {
	switch cc {
	case 0x0: // O
		return c.of
	case 0x1: // NO
		return !c.of
	case 0x2: // B/NAE/C
		return c.cf
	case 0x3: // AE/NB/NC
		return !c.cf
	case 0x4: // E/Z
		return c.zf
	case 0x5: // NE/NZ
		return !c.zf
	case 0x6: // BE/NA
		return c.cf || c.zf
	case 0x7: // A/NBE
		return !c.cf && !c.zf
	case 0x8: // S
		return c.sf
	case 0x9: // NS
		return !c.sf
	case 0xA: // P/PE
		return c.pf
	case 0xB: // NP/PO
		return !c.pf
	case 0xC: // L
		return c.sf != c.of
	case 0xD: // GE
		return c.sf == c.of
	case 0xE: // LE
		return c.zf || (c.sf != c.of)
	case 0xF: // G
		return !c.zf && (c.sf == c.of)
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

func (c *cpu) exec8c(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	seg := int((modrm >> 3) & 0x3)
	rm := modrm & 0x7
	if mod == 0x3 {
		c.setReg16(int(rm), c.segReg(seg))
		c.ip += 2
		return nil
	}
	segMem, offMem, ok, dispLen := c.ea16SegOff(mod, rm, pc+2)
	if !ok {
		return fmt.Errorf("unsupported mov r/m16,sreg modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	c.w16SegOff(segMem, offMem, c.segReg(seg))
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec8e(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	seg := int((modrm >> 3) & 0x3)
	rm := modrm & 0x7
	if mod == 0x3 {
		c.setSegReg(seg, c.reg16(int(rm)))
		c.ip += 2
		return nil
	}
	segMem, offMem, ok, dispLen := c.ea16SegOff(mod, rm, pc+2)
	if !ok {
		return fmt.Errorf("unsupported mov sreg,r/m16 modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	c.setSegReg(seg, c.u16SegOff(segMem, offMem))
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec8f(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	rm := int(modrm & 0x7)
	v := c.pop16()
	if mod == 0x3 {
		c.setReg16(rm, v)
		c.ip += 2
		return nil
	}
	segMem, offMem, ok, dispLen := c.ea16SegOff(mod, byte(rm), pc+2)
	if !ok {
		return fmt.Errorf("unsupported pop r/m16 form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	c.w16SegOff(segMem, offMem, v)
	c.ip += uint16(2 + dispLen)
	return nil
}

func (c *cpu) exec90to97(op byte) error {
	reg := int(op - 0x90)
	a := c.ax
	b := c.reg16(reg)
	c.ax = b
	c.setReg16(reg, a)
	c.ip++
	return nil
}

func (c *cpu) exec8d(pc uint32) error {
	modrm := c.rb(pc + 1)
	mod := (modrm >> 6) & 0x3
	reg := int((modrm >> 3) & 0x7)
	rm := modrm & 0x7
	_, off, ok, dispLen := c.ea16SegOff(mod, rm, pc+2)
	if !ok {
		return fmt.Errorf("unsupported lea form modrm=%02x at %04x:%04x", modrm, c.cs, c.ip)
	}
	c.setReg16(reg, off)
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
	seg, off, ok, dispLen := c.ea16SegOff(mod, rm, dispStart)
	if !ok {
		return 0, false, 0
	}
	return linear(seg, off), true, dispLen
}

func (c *cpu) ea16SegOff(mod, rm byte, dispStart uint32) (uint16, uint16, bool, int) {
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
			seg := c.ds
			if c.segOverrideSet {
				seg = c.segOverride
			}
			return seg, d, true, 2
		}
		base = c.bp
	case 7:
		base = c.bx
	}

	seg := c.ds
	if rm == 2 || rm == 3 || rm == 6 {
		seg = c.ss
	}
	if c.segOverrideSet {
		seg = c.segOverride
	}

	switch mod {
	case 0:
		return seg, base, true, 0
	case 1:
		d := int16(int8(c.rb(dispStart)))
		return seg, uint16(int32(base) + int32(d)), true, 1
	case 2:
		d := int16(c.u16(dispStart))
		return seg, uint16(int32(base) + int32(d)), true, 2
	case 3:
		return 0, 0, false, 0
	default:
		return 0, 0, false, 0
	}
}

func linear(seg, off uint16) uint32 {
	return uint32(seg)*16 + uint32(off)
}

func (c *cpu) csip() uint32 { return linear(c.cs, c.ip) }

func (c *cpu) rb(addr uint32) byte {
	if addr >= memSize {
		if c.faultErr == nil {
			c.faultErr = fmt.Errorf("memory read overflow at %05x (cs:ip=%04x:%04x)", addr, c.cs, c.ip)
		}
		return 0
	}
	return c.mem[addr]
}

func (c *cpu) wb(addr uint32, v byte) {
	if addr >= memSize {
		if c.faultErr == nil {
			c.faultErr = fmt.Errorf("memory write overflow at %05x (cs:ip=%04x:%04x)", addr, c.cs, c.ip)
		}
		return
	}
	c.mem[addr] = v
}

func (c *cpu) u16(addr uint32) uint16 {
	return uint16(c.rb(addr)) | uint16(c.rb(addr+1))<<8
}

func (c *cpu) w16(addr uint32, v uint16) {
	c.wb(addr, byte(v))
	c.wb(addr+1, byte(v>>8))
}

func (c *cpu) u16SegOff(seg, off uint16) uint16 {
	lo := c.rb(linear(seg, off))
	hi := c.rb(linear(seg, off+1))
	return uint16(lo) | uint16(hi)<<8
}

func (c *cpu) w16SegOff(seg, off uint16, v uint16) {
	c.wb(linear(seg, off), byte(v))
	c.wb(linear(seg, off+1), byte(v>>8))
}

func (c *cpu) push16(v uint16) {
	c.sp -= 2
	c.w16SegOff(c.ss, c.sp, v)
}

func (c *cpu) pop16() uint16 {
	v := c.u16SegOff(c.ss, c.sp)
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

func (c *cpu) segReg(i int) uint16 {
	switch i {
	case 0:
		return c.es
	case 1:
		return c.cs
	case 2:
		return c.ss
	case 3:
		return c.ds
	default:
		panic("bad seg reg")
	}
}

func (c *cpu) setSegReg(i int, v uint16) {
	switch i {
	case 0:
		c.es = v
	case 1:
		c.cs = v
	case 2:
		c.ss = v
	case 3:
		c.ds = v
	default:
		panic("bad seg reg")
	}
}

func (c *cpu) flagsWord() uint16 {
	f := c.extraFlags &^ modeledFlagsMask
	if c.cf {
		f |= 1 << 0
	}
	if c.pf {
		f |= 1 << 2
	}
	if c.af {
		f |= 1 << 4
	}
	if c.zf {
		f |= 1 << 6
	}
	if c.sf {
		f |= 1 << 7
	}
	if c.of {
		f |= 1 << 11
	}
	return f
}

func (c *cpu) setFlagsWord(v uint16) {
	c.extraFlags = v &^ modeledFlagsMask
	c.cf = (v & (1 << 0)) != 0
	c.pf = (v & (1 << 2)) != 0
	c.af = (v & (1 << 4)) != 0
	c.zf = (v & (1 << 6)) != 0
	c.sf = (v & (1 << 7)) != 0
	c.of = (v & (1 << 11)) != 0
}

func (c *cpu) df() bool {
	return (c.extraFlags & (1 << 10)) != 0
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
	c.pf = parityEven8(byte(v))
	c.af = false
	c.cf = false
	c.of = false
}

func (c *cpu) setLogicFlags8(v byte) {
	c.zf = v == 0
	c.sf = (v & 0x80) != 0
	c.pf = parityEven8(v)
	c.af = false
	c.cf = false
	c.of = false
}

func (c *cpu) setAddFlags16(a, b, res uint16) {
	c.zf = res == 0
	c.sf = (res & 0x8000) != 0
	c.pf = parityEven8(byte(res))
	c.af = ((a ^ b ^ res) & 0x0010) != 0
	c.cf = res < a
	c.of = ((^(a ^ b)) & (a ^ res) & 0x8000) != 0
}

func (c *cpu) setAddFlags8(a, b, res byte) {
	c.zf = res == 0
	c.sf = (res & 0x80) != 0
	c.pf = parityEven8(res)
	c.af = ((a ^ b ^ res) & 0x10) != 0
	c.cf = res < a
	c.of = ((^(a ^ b)) & (a ^ res) & 0x80) != 0
}

func (c *cpu) adc8(a, b byte) byte {
	cin := uint16(0)
	if c.cf {
		cin = 1
	}
	sum := uint16(a) + uint16(b) + cin
	res := byte(sum)
	signed := int16(int8(a)) + int16(int8(b)) + int16(cin)
	c.zf = res == 0
	c.sf = (res & 0x80) != 0
	c.pf = parityEven8(res)
	c.af = ((a ^ b ^ res) & 0x10) != 0
	c.cf = sum > 0xff
	c.of = signed < -128 || signed > 127
	return res
}

func (c *cpu) adc16(a, b uint16) uint16 {
	cin := uint32(0)
	if c.cf {
		cin = 1
	}
	sum := uint32(a) + uint32(b) + cin
	res := uint16(sum)
	signed := int32(int16(a)) + int32(int16(b)) + int32(cin)
	c.zf = res == 0
	c.sf = (res & 0x8000) != 0
	c.pf = parityEven8(byte(res))
	c.af = ((a ^ b ^ res) & 0x0010) != 0
	c.cf = sum > 0xffff
	c.of = signed < -32768 || signed > 32767
	return res
}

func (c *cpu) sbb8(a, b byte) byte {
	cin := uint16(0)
	if c.cf {
		cin = 1
	}
	sub := uint16(b) + cin
	res16 := uint16(a) - sub
	res := byte(res16)
	signed := int16(int8(a)) - int16(int8(b)) - int16(cin)
	c.zf = res == 0
	c.sf = (res & 0x80) != 0
	c.pf = parityEven8(res)
	c.af = ((a ^ byte(sub) ^ res) & 0x10) != 0
	c.cf = uint16(a) < sub
	c.of = signed < -128 || signed > 127
	return res
}

func (c *cpu) sbb16(a, b uint16) uint16 {
	cin := uint32(0)
	if c.cf {
		cin = 1
	}
	sub := uint32(b) + cin
	res32 := uint32(a) - sub
	res := uint16(res32)
	signed := int32(int16(a)) - int32(int16(b)) - int32(cin)
	c.zf = res == 0
	c.sf = (res & 0x8000) != 0
	c.pf = parityEven8(byte(res))
	c.af = ((a ^ uint16(sub) ^ res) & 0x0010) != 0
	c.cf = uint32(a) < sub
	c.of = signed < -32768 || signed > 32767
	return res
}

func (c *cpu) setSubFlags16(a, b, res uint16) {
	c.zf = res == 0
	c.sf = (res & 0x8000) != 0
	c.pf = parityEven8(byte(res))
	c.af = ((a ^ b ^ res) & 0x0010) != 0
	c.cf = a < b
	c.of = ((a ^ b) & (a ^ res) & 0x8000) != 0
}

func (c *cpu) setSubFlags8(a, b, res byte) {
	c.zf = res == 0
	c.sf = (res & 0x80) != 0
	c.pf = parityEven8(res)
	c.af = ((a ^ b ^ res) & 0x10) != 0
	c.cf = a < b
	c.of = ((a ^ b) & (a ^ res) & 0x80) != 0
}

func (c *cpu) setIncFlags16(a, res uint16) {
	c.zf = res == 0
	c.sf = (res & 0x8000) != 0
	c.pf = parityEven8(byte(res))
	c.af = ((a ^ 1 ^ res) & 0x0010) != 0
	c.of = a == 0x7fff
}

func (c *cpu) setIncFlags8(a, res byte) {
	c.zf = res == 0
	c.sf = (res & 0x80) != 0
	c.pf = parityEven8(res)
	c.af = ((a ^ 1 ^ res) & 0x10) != 0
	c.of = a == 0x7f
}

func (c *cpu) setDecFlags16(a, res uint16) {
	c.zf = res == 0
	c.sf = (res & 0x8000) != 0
	c.pf = parityEven8(byte(res))
	c.af = ((a ^ 1 ^ res) & 0x0010) != 0
	c.of = a == 0x8000
}

func (c *cpu) setDecFlags8(a, res byte) {
	c.zf = res == 0
	c.sf = (res & 0x80) != 0
	c.pf = parityEven8(res)
	c.af = ((a ^ 1 ^ res) & 0x10) != 0
	c.of = a == 0x80
}

func _unusedRegName(i int) string {
	if i >= 0 && i < len(reg16Names) {
		return reg16Names[i]
	}
	return "?"
}
