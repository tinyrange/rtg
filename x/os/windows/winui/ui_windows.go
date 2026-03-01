//go:build windows

package winui

import "runtime"

type Errno int32

func (e Errno) Error() string { return "windows ui error" }

func lastError() Errno {
	v, _, _ := winGetLastError()
	if v == 0 {
		return 1
	}
	return Errno(v)
}

const (
	// Window messages
	WM_CREATE  uint32 = 0x0001
	WM_DESTROY uint32 = 0x0002
	WM_CLOSE   uint32 = 0x0010
	WM_COMMAND uint32 = 0x0111

	// Class styles
	CS_VREDRAW uint32 = 0x0001
	CS_HREDRAW uint32 = 0x0002

	// ShowWindow commands
	SW_HIDE int32 = 0
	SW_SHOW int32 = 5

	// Notifications
	BN_CLICKED uint16 = 0

	// Window styles
	WS_OVERLAPPED       uint32 = 0x00000000
	WS_CAPTION          uint32 = 0x00C00000
	WS_SYSMENU          uint32 = 0x00080000
	WS_THICKFRAME       uint32 = 0x00040000
	WS_MINIMIZEBOX      uint32 = 0x00020000
	WS_MAXIMIZEBOX      uint32 = 0x00010000
	WS_OVERLAPPEDWINDOW uint32 = WS_OVERLAPPED | WS_CAPTION | WS_SYSMENU |
		WS_THICKFRAME | WS_MINIMIZEBOX | WS_MAXIMIZEBOX

	WS_VISIBLE uint32 = 0x10000000
	WS_CHILD   uint32 = 0x40000000
	WS_TABSTOP uint32 = 0x00010000

	// Common control styles
	BS_DEFPUSHBUTTON uint32 = 0x00000001

	ES_AUTOHSCROLL uint32 = 0x0080
	WS_BORDER      uint32 = 0x00800000

	// Extended styles
	WS_EX_CLIENTEDGE uint32 = 0x00000200

	// System colors (for hbrBackground = COLOR_WINDOW + 1)
	COLOR_WINDOW int32 = 5

	// Resource ids (LoadCursor)
	IDC_ARROW uintptr = 32512

	cwUseDefault int32 = -2147483648
)

type HWND = uintptr

type Window struct {
	Handle HWND
}

type Control struct {
	Handle HWND
	ID     uint16
}

type EventKind uint8

const (
	EventCommand EventKind = 1
	EventQuit    EventKind = 2
)

type Event struct {
	Kind     EventKind
	HWND     HWND
	ID       uint16
	Notify   uint16
	WParam   uintptr
	LParam   uintptr
	ExitCode int32
}

var (
	classRegistered bool
	classNameBuf    = toCString("RTG_BASIC_WINDOW")
	hInstance       uintptr

	pendingEvent    Event
	hasPendingEvent bool
)

func pushCommandEvent(hwnd HWND, id uint16, notify uint16, wParam uintptr, lParam uintptr) {
	pendingEvent = Event{
		Kind:   EventCommand,
		HWND:   hwnd,
		ID:     id,
		Notify: notify,
		WParam: wParam,
		LParam: lParam,
	}
	hasPendingEvent = true
}

func popPendingEvent() (Event, bool) {
	if !hasPendingEvent {
		return Event{}, false
	}
	ev := pendingEvent
	hasPendingEvent = false
	return ev, true
}

// rtg:zerocall
func byteAt(addr uintptr) byte {
	return byte(runtime.ReadPtr(addr) & 0xFF)
}

// rtg:zerocall
func readU32(addr uintptr) uint32 {
	b0 := uint32(byteAt(addr))
	b1 := uint32(byteAt(addr + 1))
	b2 := uint32(byteAt(addr + 2))
	b3 := uint32(byteAt(addr + 3))
	return b0 | (b1 << 8) | (b2 << 16) | (b3 << 24)
}

// rtg:zerocall
func readU64(addr uintptr) uint64 {
	b0 := uint64(byteAt(addr))
	b1 := uint64(byteAt(addr + 1))
	b2 := uint64(byteAt(addr + 2))
	b3 := uint64(byteAt(addr + 3))
	b4 := uint64(byteAt(addr + 4))
	b5 := uint64(byteAt(addr + 5))
	b6 := uint64(byteAt(addr + 6))
	b7 := uint64(byteAt(addr + 7))
	return b0 | (b1 << 8) | (b2 << 16) | (b3 << 24) |
		(b4 << 32) | (b5 << 40) | (b6 << 48) | (b7 << 56)
}

// rtg:zerocall
func writeU32(addr uintptr, v uint32) {
	runtime.WriteByte(addr, byte(v))
	runtime.WriteByte(addr+1, byte(v>>8))
	runtime.WriteByte(addr+2, byte(v>>16))
	runtime.WriteByte(addr+3, byte(v>>24))
}

// rtg:zerocall
func writeU64(addr uintptr, v uint64) {
	runtime.WriteByte(addr, byte(v))
	runtime.WriteByte(addr+1, byte(v>>8))
	runtime.WriteByte(addr+2, byte(v>>16))
	runtime.WriteByte(addr+3, byte(v>>24))
	runtime.WriteByte(addr+4, byte(v>>32))
	runtime.WriteByte(addr+5, byte(v>>40))
	runtime.WriteByte(addr+6, byte(v>>48))
	runtime.WriteByte(addr+7, byte(v>>56))
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

// rtg:zerocall
func lowWord(v uintptr) uint16 { return uint16(v & 0xFFFF) }

// rtg:zerocall
func highWord(v uintptr) uint16 { return uint16((v >> 16) & 0xFFFF) }

func ensureClass() Errno {
	if classRegistered {
		return 0
	}

	inst, _, _ := winGetModuleHandleA(0)
	hInstance = inst

	// WNDCLASSEXA
	p := runtime.Alloc(sizeWNDCLASSEX)
	runtime.Memzero(p, sizeWNDCLASSEX)

	writeU32(p+offWCE_cbSize, uint32(sizeWNDCLASSEX))
	writeU32(p+offWCE_style, CS_HREDRAW|CS_VREDRAW)

	proc := runtime.Funcptr(wndProc)
	writePtr(p+offWCE_lpfnWndProc, proc)

	writeU32(p+offWCE_cbClsExtra, 0)
	writeU32(p+offWCE_cbWndExtra, 0)
	writePtr(p+offWCE_hInstance, hInstance)
	writePtr(p+offWCE_hIcon, 0)

	cur, _, _ := winLoadCursorA(0, IDC_ARROW)
	writePtr(p+offWCE_hCursor, cur)

	// hbrBackground = (HBRUSH)(COLOR_WINDOW + 1)
	writePtr(p+offWCE_hbrBg, uintptr(uint32(COLOR_WINDOW+1)))

	writePtr(p+offWCE_lpszMenu, 0)
	writePtr(p+offWCE_lpszClass, runtime.Sliceptr(classNameBuf))
	writePtr(p+offWCE_hIconSm, 0)

	atom, _, _ := winRegisterClassExA(p)
	if atom == 0 {
		return lastError()
	}

	classRegistered = true
	return 0
}

func NewWindow(title string, width, height int32) (Window, Errno) {
	if err := ensureClass(); err != 0 {
		return Window{}, err
	}

	t := toCString(title)
	hwnd, _, _ := winCreateWindowExA(
		0,
		runtime.Sliceptr(classNameBuf),
		runtime.Sliceptr(t),
		uintptr(WS_OVERLAPPEDWINDOW),
		uintptr(int64(cwUseDefault)),
		uintptr(int64(cwUseDefault)),
		uintptr(int64(width)),
		uintptr(int64(height)),
		0,
		0,
		hInstance,
		0,
	)
	if hwnd == 0 {
		return Window{}, lastError()
	}
	return Window{Handle: hwnd}, 0
}

func (w Window) Show(cmd int32) {
	_, _, _ = winShowWindow(w.Handle, uintptr(int64(cmd)))
	_, _, _ = winUpdateWindow(w.Handle)
}

func (w Window) Destroy() Errno {
	ok, _, _ := winDestroyWindow(w.Handle)
	if ok == 0 {
		return lastError()
	}
	return 0
}

func (w Window) CreateButton(
	id uint16,
	text string,
	x, y, width, height int32,
) (Control, Errno) {
	return w.createChild(
		0,
		"BUTTON",
		text,
		WS_CHILD|WS_VISIBLE|WS_TABSTOP|BS_DEFPUSHBUTTON,
		x,
		y,
		width,
		height,
		id,
	)
}

func (w Window) CreateEdit(
	id uint16,
	text string,
	x, y, width, height int32,
) (Control, Errno) {
	return w.createChild(
		WS_EX_CLIENTEDGE,
		"EDIT",
		text,
		WS_CHILD|WS_VISIBLE|WS_TABSTOP|WS_BORDER|ES_AUTOHSCROLL,
		x,
		y,
		width,
		height,
		id,
	)
}

func (w Window) CreateStatic(
	id uint16,
	text string,
	x, y, width, height int32,
) (Control, Errno) {
	return w.createChild(
		0,
		"STATIC",
		text,
		WS_CHILD|WS_VISIBLE,
		x,
		y,
		width,
		height,
		id,
	)
}

func (w Window) createChild(
	exStyle uint32,
	class string,
	text string,
	style uint32,
	x, y, width, height int32,
	id uint16,
) (Control, Errno) {
	cn := toCString(class)
	tx := toCString(text)

	h, _, _ := winCreateWindowExA(
		uintptr(exStyle),
		runtime.Sliceptr(cn),
		runtime.Sliceptr(tx),
		uintptr(style),
		uintptr(int64(x)),
		uintptr(int64(y)),
		uintptr(int64(width)),
		uintptr(int64(height)),
		w.Handle,
		uintptr(id), // hMenu used as child ID
		hInstance,
		0,
	)
	if h == 0 {
		return Control{}, lastError()
	}
	return Control{Handle: h, ID: id}, 0
}

func (c Control) SetText(text string) Errno {
	tx := toCString(text)
	ok, _, _ := winSetWindowTextA(c.Handle, runtime.Sliceptr(tx))
	if ok == 0 {
		return lastError()
	}
	return 0
}

func (c Control) Text(maxChars int) (string, Errno) {
	if maxChars <= 0 {
		maxChars = 1
	}
	buf := runtime.Alloc(maxChars + 1)
	runtime.Memzero(buf, maxChars+1)
	n, _, _ := winGetWindowTextA(c.Handle, buf, uintptr(int64(maxChars+1)))
	if n == 0 {
		// 0 can mean empty string or error; distinguish via GetLastError is messy.
		// Keep it simple: return empty string with no error.
		return "", 0
	}
	return runtime.Makestring(buf, int(n)), 0
}

func NextEvent() (Event, Errno) {
	if ev, ok := popPendingEvent(); ok {
		return ev, 0
	}
	msg := allocMsg()
	for {
		r, _, _ := winGetMessageA(msg, 0, 0, 0)
		if int32(r) == -1 {
			return Event{}, lastError()
		}
		if r == 0 {
			// WM_QUIT; MSG.wParam is the exit code
			code := int32(readPtr(msg + offMSG_wParam))
			return Event{Kind: EventQuit, ExitCode: code}, 0
		}
		_, _, _ = winTranslateMessage(msg)
		_, _, _ = winDispatchMessageA(msg)
		if ev, ok := popPendingEvent(); ok {
			return ev, 0
		}
	}
}

func Run() (int32, Errno) {
	for {
		ev, err := NextEvent()
		if err != 0 {
			return 0, err
		}
		if ev.Kind == EventQuit {
			return ev.ExitCode, 0
		}
	}
}

// --- Window procedure (callback) ---

//rtg:callback
func wndProc(hwnd HWND, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_COMMAND:
		id := lowWord(wParam)
		notify := highWord(wParam)
		pushCommandEvent(hwnd, id, notify, wParam, lParam)
		return 0
	case WM_CLOSE:
		_, _, _ = winDestroyWindow(hwnd)
		return 0
	case WM_DESTROY:
		_, _, _ = winPostQuitMessage(0)
		return 0
	}

	r, _, _ := winDefWindowProcA(hwnd, uintptr(msg), wParam, lParam)
	return r
}

// --- Imports ---

//rtg:linkstatic kernel32.dll,GetLastError,rawptr
func winGetLastError() (uintptr, uintptr, int32)

//rtg:linkstatic kernel32.dll,GetModuleHandleA,rawptr
func winGetModuleHandleA(namePtr uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic user32.dll,RegisterClassExA,rawptr
func winRegisterClassExA(wndClassExPtr uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic user32.dll,CreateWindowExA,rawptr
func winCreateWindowExA(
	exStyle, classNamePtr, windowNamePtr, style uintptr,
	x, y, width, height uintptr,
	parent, menu, instance, param uintptr,
) (uintptr, uintptr, int32)

//rtg:linkstatic user32.dll,DefWindowProcA,rawptr
func winDefWindowProcA(hwnd, msg, wParam, lParam uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic user32.dll,ShowWindow,rawptr
func winShowWindow(hwnd, cmdShow uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic user32.dll,UpdateWindow,rawptr
func winUpdateWindow(hwnd uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic user32.dll,GetMessageA,rawptr
func winGetMessageA(msgPtr, hwnd, min, max uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic user32.dll,TranslateMessage,rawptr
func winTranslateMessage(msgPtr uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic user32.dll,DispatchMessageA,rawptr
func winDispatchMessageA(msgPtr uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic user32.dll,PostQuitMessage,rawptr
func winPostQuitMessage(exitCode uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic user32.dll,DestroyWindow,rawptr
func winDestroyWindow(hwnd uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic user32.dll,LoadCursorA,rawptr
func winLoadCursorA(instance, cursorName uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic user32.dll,SetWindowTextA,rawptr
func winSetWindowTextA(hwnd, textPtr uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic user32.dll,GetWindowTextA,rawptr
func winGetWindowTextA(hwnd, bufPtr, maxCount uintptr) (uintptr, uintptr, int32)
