package main

import "fmt"

//rtg:zerocall
type MMIO struct {
	base uintptr
}

func (m MMIO) addrValue(off uintptr) uintptr {
	return m.base + off
}

func (m *MMIO) addrPtr(off uintptr) uintptr {
	return m.base + off + 1
}

//rtg:zerocall
func add(a uintptr, b uintptr) uintptr {
	return a + b
}

//rtg:zerocall
func (m MMIO) Addr(off uintptr) uintptr {
	return add(m.addrValue(off), 0)
}

//rtg:zerocall
func (m *MMIO) AddrViaPtr(off uintptr) uintptr {
	return m.addrPtr(off)
}

func main() {
	m := MMIO{base: 0x100}
	if m.Addr(0x10) != 0x110 {
		panic("value zerocall method mismatch")
	}
	pm := &m
	if pm.AddrViaPtr(0x10) != 0x111 {
		panic("pointer zerocall method mismatch")
	}
	fmt.Print("PASS")
}
