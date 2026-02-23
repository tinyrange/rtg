//go:build windows

package console

import "runtime"

const (
	InvalidHandleValue uintptr = ^uintptr(0)
	StdInputHandle     uintptr = ^uintptr(9)
	StdOutputHandle    uintptr = ^uintptr(10)
	StdErrorHandle     uintptr = ^uintptr(11)
)

const (
	EnableProcessedInput        uint32 = 0x0001
	EnableLineInput             uint32 = 0x0002
	EnableEchoInput             uint32 = 0x0004
	EnableWindowInput           uint32 = 0x0008
	EnableMouseInput            uint32 = 0x0010
	EnableInsertMode            uint32 = 0x0020
	EnableQuickEditMode         uint32 = 0x0040
	EnableExtendedFlags         uint32 = 0x0080
	EnableAutoPosition          uint32 = 0x0100
	EnableVirtualTerminalInput  uint32 = 0x0200
	EnableProcessedOutput       uint32 = 0x0001
	EnableWrapAtEolOutput       uint32 = 0x0002
	EnableVirtualTerminalOutput uint32 = 0x0004
	DisableNewlineAutoReturn    uint32 = 0x0008
	EnableLvbGridWorldwide      uint32 = 0x0010
)

const (
	ForegroundBlue      uint16 = 0x0001
	ForegroundGreen     uint16 = 0x0002
	ForegroundRed       uint16 = 0x0004
	ForegroundIntensity uint16 = 0x0008
	BackgroundBlue      uint16 = 0x0010
	BackgroundGreen     uint16 = 0x0020
	BackgroundRed       uint16 = 0x0040
	BackgroundIntensity uint16 = 0x0080
)

const (
	cpUTF8 uint32 = 65001
)

const (
	sizeCoord            = 4
	sizeSmallRect        = 8
	sizeScreenBufferInfo = 22
	sizeCursorInfo       = 8
)

type Errno int32

type Coord struct {
	X int16
	Y int16
}

type SmallRect struct {
	Left   int16
	Top    int16
	Right  int16
	Bottom int16
}

type ScreenBufferInfo struct {
	Size              Coord
	CursorPosition    Coord
	Attributes        uint16
	Window            SmallRect
	MaximumWindowSize Coord
}

type CursorInfo struct {
	Size    uint32
	Visible bool
}

type Console struct {
	Handle uintptr
}

//rtg:zerocall
func (s ScreenBufferInfo) AttributesValue() uint16 {
	return s.Attributes
}

//rtg:zerocall
func (s ScreenBufferInfo) WindowLeft() int16 {
	return s.Window.Left
}

//rtg:zerocall
func (s ScreenBufferInfo) WindowTop() int16 {
	return s.Window.Top
}

//rtg:zerocall
func (s ScreenBufferInfo) WindowRight() int16 {
	return s.Window.Right
}

//rtg:zerocall
func (c CursorInfo) SizeValue() uint32 {
	return c.Size
}

func (e Errno) Error() string {
	return "windows console error"
}

func lastError() Errno {
	v, _, _ := winGetLastError()
	if v == 0 {
		return 1
	}
	return Errno(v)
}

func LastError() Errno {
	return lastError()
}

//rtg:zerocall
func byteAt(addr uintptr) byte {
	return byte(runtime.ReadPtr(addr) & 0xFF)
}

//rtg:zerocall
func readU16(addr uintptr) uint16 {
	b0 := uint16(byteAt(addr))
	b1 := uint16(byteAt(addr + 1))
	return b0 | (b1 << 8)
}

//rtg:zerocall
func readU32(addr uintptr) uint32 {
	b0 := uint32(byteAt(addr))
	b1 := uint32(byteAt(addr + 1))
	b2 := uint32(byteAt(addr + 2))
	b3 := uint32(byteAt(addr + 3))
	return b0 | (b1 << 8) | (b2 << 16) | (b3 << 24)
}

//rtg:zerocall
func writeU16(addr uintptr, v uint16) {
	runtime.WriteByte(addr, byte(v))
	runtime.WriteByte(addr+1, byte(v>>8))
}

//rtg:zerocall
func writeU32(addr uintptr, v uint32) {
	runtime.WriteByte(addr, byte(v))
	runtime.WriteByte(addr+1, byte(v>>8))
	runtime.WriteByte(addr+2, byte(v>>16))
	runtime.WriteByte(addr+3, byte(v>>24))
}

//rtg:zerocall
func writeCoord(addr uintptr, c Coord) {
	writeU16(addr, uint16(c.X))
	writeU16(addr+2, uint16(c.Y))
}

//rtg:zerocall
func writeSmallRect(addr uintptr, r SmallRect) {
	writeU16(addr, uint16(r.Left))
	writeU16(addr+2, uint16(r.Top))
	writeU16(addr+4, uint16(r.Right))
	writeU16(addr+6, uint16(r.Bottom))
}

func readCoord(addr uintptr) Coord {
	return Coord{
		X: int16(readU16(addr)),
		Y: int16(readU16(addr + 2)),
	}
}

func readSmallRect(addr uintptr) SmallRect {
	return SmallRect{
		Left:   int16(readU16(addr)),
		Top:    int16(readU16(addr + 2)),
		Right:  int16(readU16(addr + 4)),
		Bottom: int16(readU16(addr + 6)),
	}
}

//rtg:zerocall
func packCoord(c Coord) uintptr {
	x := uintptr(uint16(c.X))
	y := uintptr(uint16(c.Y))
	return x | (y << 16)
}

//rtg:zerocall
func boolPtr(v bool) uintptr {
	if v {
		return 1
	}
	return 0
}

func toCString(s string) []byte {
	b := make([]byte, len(s)+1)
	i := 0
	for i < len(s) {
		b[i] = s[i]
		i++
	}
	return b
}

func OpenStd(stdHandle uintptr) (Console, Errno) {
	h, _, _ := winGetStdHandle(stdHandle)
	if h == 0 || h == InvalidHandleValue {
		return Console{}, lastError()
	}
	return Console{Handle: h}, 0
}

func Stdin() (Console, Errno) {
	return OpenStd(StdInputHandle)
}

func Stdout() (Console, Errno) {
	return OpenStd(StdOutputHandle)
}

func Stderr() (Console, Errno) {
	return OpenStd(StdErrorHandle)
}

func (c Console) GetMode() (uint32, Errno) {
	p := runtime.Alloc(4)
	runtime.Memzero(p, 4)
	ok, _, _ := winGetConsoleMode(c.Handle, p)
	if ok == 0 {
		return 0, lastError()
	}
	return readU32(p), 0
}

func (c Console) SetMode(mode uint32) Errno {
	ok, _, _ := winSetConsoleMode(c.Handle, uintptr(mode))
	if ok == 0 {
		return lastError()
	}
	return 0
}

func (c Console) WriteString(s string) (int, Errno) {
	if len(s) == 0 {
		return 0, 0
	}
	pWritten := runtime.Alloc(4)
	runtime.Memzero(pWritten, 4)
	ok, _, _ := winWriteConsoleA(c.Handle, runtime.Stringptr(s), uintptr(len(s)), pWritten, 0)
	if ok == 0 {
		return 0, lastError()
	}
	return int(readU32(pWritten)), 0
}

func (c Console) ReadString(maxChars int) (string, Errno) {
	if maxChars <= 0 {
		return "", 0
	}
	buf := runtime.Alloc(maxChars)
	runtime.Memzero(buf, maxChars)
	pRead := runtime.Alloc(4)
	runtime.Memzero(pRead, 4)
	ok, _, _ := winReadConsoleA(c.Handle, buf, uintptr(maxChars), pRead, 0)
	if ok == 0 {
		return "", lastError()
	}
	n := int(readU32(pRead))
	if n < 0 {
		n = 0
	}
	if n > maxChars {
		n = maxChars
	}
	return runtime.Makestring(buf, n), 0
}

func (c Console) GetScreenBufferInfo() (ScreenBufferInfo, Errno) {
	var out ScreenBufferInfo
	p := runtime.Alloc(sizeScreenBufferInfo)
	runtime.Memzero(p, sizeScreenBufferInfo)
	ok, _, _ := winGetConsoleScreenBufferInfo(c.Handle, p)
	if ok == 0 {
		return out, lastError()
	}
	out.Size = readCoord(p)
	out.CursorPosition = readCoord(p + 4)
	out.Attributes = readU16(p + 8)
	out.Window = readSmallRect(p + 10)
	out.MaximumWindowSize = readCoord(p + 18)
	return out, 0
}

func (c Console) SetCursorPosition(pos Coord) Errno {
	ok, _, _ := winSetConsoleCursorPosition(c.Handle, packCoord(pos))
	if ok == 0 {
		return lastError()
	}
	return 0
}

func (c Console) FillOutputCharacter(ch byte, length uint32, pos Coord) (uint32, Errno) {
	pWritten := runtime.Alloc(4)
	runtime.Memzero(pWritten, 4)
	ok, _, _ := winFillConsoleOutputCharacterA(c.Handle, uintptr(ch), uintptr(length), packCoord(pos), pWritten)
	if ok == 0 {
		return 0, lastError()
	}
	return readU32(pWritten), 0
}

func (c Console) FillOutputAttribute(attr uint16, length uint32, pos Coord) (uint32, Errno) {
	pWritten := runtime.Alloc(4)
	runtime.Memzero(pWritten, 4)
	ok, _, _ := winFillConsoleOutputAttribute(c.Handle, uintptr(attr), uintptr(length), packCoord(pos), pWritten)
	if ok == 0 {
		return 0, lastError()
	}
	return readU32(pWritten), 0
}

func (c Console) SetTextAttribute(attr uint16) Errno {
	ok, _, _ := winSetConsoleTextAttribute(c.Handle, uintptr(attr))
	if ok == 0 {
		return lastError()
	}
	return 0
}

func (c Console) GetCursorInfo() (CursorInfo, Errno) {
	var out CursorInfo
	p := runtime.Alloc(sizeCursorInfo)
	runtime.Memzero(p, sizeCursorInfo)
	ok, _, _ := winGetConsoleCursorInfo(c.Handle, p)
	if ok == 0 {
		return out, lastError()
	}
	out.Size = readU32(p)
	out.Visible = readU32(p+4) != 0
	return out, 0
}

func (c Console) SetCursorInfo(info CursorInfo) Errno {
	p := runtime.Alloc(sizeCursorInfo)
	runtime.Memzero(p, sizeCursorInfo)
	writeU32(p, info.Size)
	writeU32(p+4, uint32(boolPtr(info.Visible)))
	ok, _, _ := winSetConsoleCursorInfo(c.Handle, p)
	if ok == 0 {
		return lastError()
	}
	return 0
}

func (c Console) InputEventCount() (uint32, Errno) {
	p := runtime.Alloc(4)
	runtime.Memzero(p, 4)
	ok, _, _ := winGetNumberOfConsoleInputEvents(c.Handle, p)
	if ok == 0 {
		return 0, lastError()
	}
	return readU32(p), 0
}

func (c Console) FlushInputBuffer() Errno {
	ok, _, _ := winFlushConsoleInputBuffer(c.Handle)
	if ok == 0 {
		return lastError()
	}
	return 0
}

func (c Console) SetScreenBufferSize(size Coord) Errno {
	ok, _, _ := winSetConsoleScreenBufferSize(c.Handle, packCoord(size))
	if ok == 0 {
		return lastError()
	}
	return 0
}

func (c Console) SetWindow(rect SmallRect, absolute bool) Errno {
	p := runtime.Alloc(sizeSmallRect)
	runtime.Memzero(p, sizeSmallRect)
	writeSmallRect(p, rect)
	ok, _, _ := winSetConsoleWindowInfo(c.Handle, boolPtr(absolute), p)
	if ok == 0 {
		return lastError()
	}
	return 0
}

func SetTitle(title string) Errno {
	buf := toCString(title)
	ok, _, _ := winSetConsoleTitleA(runtime.Sliceptr(buf))
	if ok == 0 {
		return lastError()
	}
	return 0
}

func GetTitle(maxChars int) (string, Errno) {
	if maxChars <= 0 {
		maxChars = 1
	}
	buf := runtime.Alloc(maxChars + 1)
	runtime.Memzero(buf, maxChars+1)
	n, _, _ := winGetConsoleTitleA(buf, uintptr(maxChars+1))
	if n == 0 {
		return "", lastError()
	}
	return runtime.Makestring(buf, int(n)), 0
}

func SetInputCodePage(codePage uint32) Errno {
	ok, _, _ := winSetConsoleCP(uintptr(codePage))
	if ok == 0 {
		return lastError()
	}
	return 0
}

func SetOutputCodePage(codePage uint32) Errno {
	ok, _, _ := winSetConsoleOutputCP(uintptr(codePage))
	if ok == 0 {
		return lastError()
	}
	return 0
}

func EnableUTF8Console(out Console, in Console) Errno {
	err := SetInputCodePage(cpUTF8)
	if err != 0 {
		return err
	}
	err = SetOutputCodePage(cpUTF8)
	if err != 0 {
		return err
	}
	om, err := out.GetMode()
	if err == 0 {
		_ = out.SetMode(om | EnableProcessedOutput | EnableWrapAtEolOutput | EnableVirtualTerminalOutput)
	}
	im, err := in.GetMode()
	if err == 0 {
		_ = in.SetMode(im | EnableProcessedInput | EnableLineInput | EnableEchoInput)
	}
	return 0
}

//rtg:linkstatic kernel32.dll,GetLastError,rawptr
func winGetLastError() (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,GetStdHandle,rawptr
func winGetStdHandle(stdHandle uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,GetConsoleMode,rawptr
func winGetConsoleMode(handle, modePtr uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,SetConsoleMode,rawptr
func winSetConsoleMode(handle, mode uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,WriteConsoleA,rawptr
func winWriteConsoleA(handle, buffer, charsToWrite, charsWrittenPtr, reserved uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,ReadConsoleA,rawptr
func winReadConsoleA(handle, buffer, charsToRead, charsReadPtr, control uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,GetConsoleScreenBufferInfo,rawptr
func winGetConsoleScreenBufferInfo(handle, infoPtr uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,SetConsoleCursorPosition,rawptr
func winSetConsoleCursorPosition(handle, coord uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,FillConsoleOutputCharacterA,rawptr
func winFillConsoleOutputCharacterA(handle, ch, length, coord, writtenPtr uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,FillConsoleOutputAttribute,rawptr
func winFillConsoleOutputAttribute(handle, attr, length, coord, writtenPtr uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,SetConsoleTextAttribute,rawptr
func winSetConsoleTextAttribute(handle, attr uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,SetConsoleTitleA,rawptr
func winSetConsoleTitleA(titlePtr uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,GetConsoleTitleA,rawptr
func winGetConsoleTitleA(titlePtr, size uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,GetConsoleCursorInfo,rawptr
func winGetConsoleCursorInfo(handle, infoPtr uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,SetConsoleCursorInfo,rawptr
func winSetConsoleCursorInfo(handle, infoPtr uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,GetNumberOfConsoleInputEvents,rawptr
func winGetNumberOfConsoleInputEvents(handle, countPtr uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,FlushConsoleInputBuffer,rawptr
func winFlushConsoleInputBuffer(handle uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,SetConsoleScreenBufferSize,rawptr
func winSetConsoleScreenBufferSize(handle, coord uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,SetConsoleWindowInfo,rawptr
func winSetConsoleWindowInfo(handle, absolute, rectPtr uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,SetConsoleCP,rawptr
func winSetConsoleCP(codePage uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,SetConsoleOutputCP,rawptr
func winSetConsoleOutputCP(codePage uintptr) (uintptr, uintptr, int32)
