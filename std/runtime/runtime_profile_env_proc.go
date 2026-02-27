//go:build !windows && !(darwin && arm64) && !c

package runtime

func profileLookupEnv(key string) string {
	path := profileMakeCString("/proc/self/environ")
	fd, _, errn := SysOpen(Sliceptr(path), 0, 0)
	if errn != 0 || fd == 0 {
		return ""
	}
	var data []byte
	chunk := make([]byte, 256)
	for {
		n, _, rerr := SysRead(fd, Sliceptr(chunk), uintptr(len(chunk)))
		if rerr != 0 {
			SysClose(fd)
			return ""
		}
		if n == 0 {
			break
		}
		data = append(data, chunk[0:int(n)]...)
	}
	SysClose(fd)
	return profileLookupEnvFromBlock(data, key)
}
