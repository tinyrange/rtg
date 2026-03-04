//go:build darwin && arm64

package runtime

const profileOpenFlags = 1537

//rtg:linkstatic libSystem.dylib,_getenv,ptr
func profileDarwinGetenv(name uintptr) (uintptr, uintptr, int32)

//rtg:linkstatic libSystem.dylib,_strlen
func profileDarwinStrlen(s uintptr) (uintptr, uintptr, int32)

func profileLookupEnv(key string) string {
	if key == "" {
		return ""
	}
	ckey := profileMakeCString(key)
	valuePtr, _, _ := profileDarwinGetenv(Sliceptr(ckey))
	if valuePtr == 0 {
		return ""
	}
	length, _, _ := profileDarwinStrlen(valuePtr)
	return Makestring(valuePtr, int(length))
}
