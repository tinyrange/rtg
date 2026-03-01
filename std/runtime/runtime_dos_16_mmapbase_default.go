//go:build dos && dos16 && !tiny_dos_backend

package runtime

func dosInitMmapBase() uintptr {
	// Leave headroom for stack while reducing OOMs on larger stdlib fixtures.
	return 0xBC00
}
