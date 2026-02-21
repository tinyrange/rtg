//go:build !no_backend_windows_i386

package i386

import (
	"strings"

	"j5.nz/rtg/std/compiler/ir"
)

func decodeLinkStaticSpecWin386(raw string) (string, string, string, bool) {
	parts := strings.Split(raw, ",")
	if len(parts) != 3 {
		return "", "", "", false
	}
	lib := strings.TrimSpace(parts[0])
	sym := strings.TrimSpace(parts[1])
	mode := strings.TrimSpace(parts[2])
	if lib == "" || sym == "" {
		return "", "", "", false
	}
	return lib, sym, mode, true
}

func (g *CodeGen) compileLinkStaticIntrinsicWin386(inst ir.Inst) bool {
	if g.target.GOOS != "windows" || g.irmod == nil || g.irmod.LinkStaticFuncs == nil {
		return false
	}
	raw, ok := g.irmod.LinkStaticFuncs[inst.Name]
	if !ok {
		return false
	}
	lib, sym, _, ok := decodeLinkStaticSpecWin386(raw)
	if !ok {
		panic("ICE: invalid windows linkstatic metadata for '" + inst.Name + "'")
	}
	if lib != "kernel32.dll" {
		panic("ICE: unsupported windows linkstatic library '" + lib + "'")
	}

	switch sym {
	case "ReadFile":
		g.compileSyscallRead_win386()
	case "WriteFile":
		g.compileSyscallWrite_win386()
	case "CreateFileA":
		g.compileSyscallOpen_win386()
	case "CloseHandle":
		g.compileSyscallClose_win386()
	case "GetFileAttributesExA":
		g.compileSyscallStat_win386()
	case "ExitProcess":
		g.compileSyscallExit_win386()
	case "VirtualAlloc":
		g.compileSyscallMmap_win386()
	case "CreateDirectoryA":
		g.compileSyscallMkdir_win386()
	case "RemoveDirectoryA":
		g.compileSyscallRmdir_win386()
	case "DeleteFileA":
		g.compileSyscallUnlink_win386()
	case "GetCurrentDirectoryA":
		g.compileSyscallGetcwd_win386()
	case "GetCommandLineA":
		g.compileSyscallGetCommandLine_win386()
	case "GetEnvironmentStringsA":
		g.compileSyscallGetEnvStrings_win386()
	case "FindFirstFileA":
		g.compileSyscallFindFirstFile_win386()
	case "FindNextFileA":
		g.compileSyscallFindNextFile_win386()
	case "FindClose":
		g.compileSyscallFindClose_win386()
	case "CreateProcessA":
		g.compileSyscallCreateProcess_win386()
	case "WaitForSingleObject":
		g.compileSyscallWaitProcess_win386()
	case "CreatePipe":
		g.compileSyscallCreatePipe_win386()
	case "SetStdHandle":
		g.compileSyscallSetStdHandle_win386()
	case "GetCurrentProcessId":
		g.compileSyscallGetpid_win386()
	default:
		panic("ICE: unknown windows linkstatic symbol '" + sym + "'")
	}
	return true
}
