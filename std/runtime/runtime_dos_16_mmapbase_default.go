//go:build dos && dos16 && !tiny_dos_backend

package runtime

func dosInitMmapBase() uintptr {
	return 0xD000
}
