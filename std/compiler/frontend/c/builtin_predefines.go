package frontend

func defaultTargetPtrSize(arch string) int {
	switch arch {
	case "386", "arm", "armv8m", "wasm32":
		return 4
	case "c16":
		return 2
	case "c32":
		return 4
	case "c64":
		return 8
	default:
		return 8
	}
}

func builtinPredefineSpecs(targetOS string, targetArch string, ptrSize int, hosted bool) []string {
	if ptrSize <= 0 {
		ptrSize = defaultTargetPtrSize(targetArch)
	}
	longType := "long"
	ulongType := "unsigned long"
	intptrType := "long"
	uintptrType := "unsigned long"
	longMax := "9223372036854775807L"
	ulongMax := "18446744073709551615UL"
	sizeMax := "18446744073709551615UL"
	switch ptrSize {
	case 2:
		longType = "short"
		ulongType = "unsigned short"
		intptrType = "short"
		uintptrType = "unsigned short"
		longMax = "32767"
		ulongMax = "65535U"
		sizeMax = "65535U"
	case 4:
		longType = "int"
		ulongType = "unsigned int"
		intptrType = "int"
		uintptrType = "unsigned int"
		longMax = "2147483647"
		ulongMax = "4294967295U"
		sizeMax = "4294967295U"
	}

	specs := []string{
		"__RTG__=1",
		"__STDC__=1",
		"__STDC_VERSION__=199901L",
		"__STDC_HOSTED__=0",
		"__DATE__=" + quoteTokenText("Jan  1 1970"),
		"__TIME__=" + quoteTokenText("00:00:00"),
		"__ORDER_LITTLE_ENDIAN__=1234",
		"__ORDER_BIG_ENDIAN__=4321",
		"__ORDER_PDP_ENDIAN__=3412",
		"__BYTE_ORDER__=__ORDER_LITTLE_ENDIAN__",
		"__CHAR_BIT__=8",
		"__SCHAR_MAX__=127",
		"__SHRT_MAX__=32767",
		"__INT_MAX__=2147483647",
		"__FLT_RADIX__=2",
		"__FLT_MANT_DIG__=24",
		"__DBL_MANT_DIG__=53",
		"__LDBL_MANT_DIG__=53",
		"__FLT_DIG__=6",
		"__DBL_DIG__=15",
		"__LDBL_DIG__=15",
		"__FLT_MAX_EXP__=128",
		"__DBL_MAX_EXP__=1024",
		"__LDBL_MAX_EXP__=1024",
		"__FLT_MIN_EXP__=(-125)",
		"__DBL_MIN_EXP__=(-1021)",
		"__LDBL_MIN_EXP__=(-1021)",
		"__ATOMIC_RELEASE=3",
		"__ATOMIC_SEQ_CST=5",
		"__LONG_MAX__=" + longMax,
		"__ULONG_MAX__=" + ulongMax,
		"__SIZE_MAX__=" + sizeMax,
		"__WCHAR_TYPE__=int",
		"__SIZEOF_SHORT__=2",
		"__SIZEOF_INT__=4",
		"__SIZEOF_LONG__=" + decimalItoa(ptrSize),
		"__SIZEOF_POINTER__=" + decimalItoa(ptrSize),
		"__PTRDIFF_TYPE__=" + longType,
		"__SIZE_TYPE__=" + ulongType,
		"__INTPTR_TYPE__=" + intptrType,
		"__UINTPTR_TYPE__=" + uintptrType,
		"BUFSIZ=1024",
		"_IOFBF=0",
		"_IOLBF=1",
		"_IONBF=2",
		"F_OK=0",
		"X_OK=1",
		"W_OK=2",
		"R_OK=4",
		"_POSIX_TRACE=1",
		"_XOPEN_VERSION=700",
		"ICANON=0x0002",
		"ECHO=0x0008",
		"VMIN=16",
		"VTIME=17",
		"TIOCGWINSZ=0x5413",
		"SOCK_STREAM=1",
		"IPPROTO_TCP=6",
		"F_TLOCK=2",
		"F_TEST=3",
		"L_tmpnam=1024",
		"TMP_MAX=238328",
	}
	if hosted {
		specs = append(specs, "__STDC_HOSTED__=1", "__GNUC__=4", "__GNUC_MINOR__=2", "__GNUC_PATCHLEVEL__=1")
	}
	if ptrSize == 8 {
		specs = append(specs, "__LP64__=1", "_LP64=1")
	}
	switch targetArch {
	case "amd64":
		specs = append(specs, "__x86_64__=1", "__amd64__=1")
	case "386":
		specs = append(specs, "__i386__=1")
	case "arm64":
		specs = append(specs, "__arm64__=1", "__aarch64__=1")
	case "arm", "armv8m":
		specs = append(specs, "__arm__=1")
	}
	switch targetOS {
	case "darwin":
		specs = append(specs, "__APPLE__=1", "__APPLE_CC__=1", "__MACH__=1", "__unix__=1", "__unix=1", "unix=1")
	case "linux":
		specs = append(specs, "__linux__=1", "__unix__=1", "__unix=1", "unix=1")
	case "windows":
		specs = append(specs, "_WIN32=1")
	case "":
		specs = append(specs, "__unix__=1", "__unix=1", "unix=1")
	}
	return specs
}
