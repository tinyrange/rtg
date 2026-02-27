//go:build !windows && !wasi

package runtime

func profileNormalizePermissions(path []byte) {
	SysChmod(Sliceptr(path), uintptr(profileFilePerm))
}
