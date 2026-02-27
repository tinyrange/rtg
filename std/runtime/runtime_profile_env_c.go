//go:build c

package runtime

func profileLookupEnv(key string) string {
	if key == "" {
		return ""
	}
	ckey := profileMakeCString(key)
	ptr, _, errn := SysGetenv(Sliceptr(ckey))
	if errn != 0 || ptr == 0 {
		return ""
	}
	length := 0
	p := ptr
	for {
		b := byte(ReadPtr(p))
		if b == 0 {
			break
		}
		length++
		p++
	}
	if length == 0 {
		return ""
	}
	return Makestring(ptr, length)
}
