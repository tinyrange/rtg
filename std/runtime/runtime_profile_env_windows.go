//go:build windows

package runtime

func profileLookupEnv(key string) string {
	envPtr, _, _ := SysGetEnvStrings()
	if envPtr == 0 {
		return ""
	}
	ptr := envPtr
	for {
		first := byte(ReadPtr(ptr))
		if first == 0 {
			break
		}
		entryPtr := ptr
		length := 0
		for {
			cb := byte(ReadPtr(ptr))
			if cb == 0 {
				ptr++
				break
			}
			length++
			ptr++
		}
		if length == 0 {
			continue
		}
		entry := Makestring(entryPtr, length)
		if value, ok := profileSplitEnvEntry(entry, key); ok {
			return value
		}
	}
	return ""
}
