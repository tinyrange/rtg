//go:build !no_backend_windows_amd64

package x64

import (
	"strings"

	"j5.nz/rtg/std/compiler/ir"
)

func decodeLinkStaticSpecWin64(raw string) (string, string, string, bool) {
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

func (g *CodeGen) compileLinkStaticIntrinsicWin64(inst ir.Inst) bool {
	if g.target.GOOS != "windows" || g.irmod == nil || g.irmod.LinkStaticFuncs == nil {
		return false
	}
	raw, ok := g.irmod.LinkStaticFuncs[inst.Name]
	if !ok {
		return false
	}
	lib, sym, _, ok := decodeLinkStaticSpecWin64(raw)
	if !ok {
		panic("ICE: invalid windows linkstatic metadata for '" + inst.Name + "'")
	}
	if lib != "kernel32.dll" {
		panic("ICE: unsupported windows linkstatic library '" + lib + "'")
	}

	switch sym {
	case "ReadFile":
		g.compileSyscallRead_win64()
	case "WriteFile":
		g.compileSyscallWrite_win64()
	case "CreateFileA":
		g.compileSyscallOpen_win64()
	case "CloseHandle":
		g.compileSyscallClose_win64()
	case "GetFileAttributesExA":
		g.compileSyscallStat_win64()
	case "ExitProcess":
		g.compileSyscallExit_win64()
	case "VirtualAlloc":
		g.compileSyscallMmap_win64()
	case "CreateDirectoryA":
		g.compileSyscallMkdir_win64()
	case "RemoveDirectoryA":
		g.compileSyscallRmdir_win64()
	case "DeleteFileA":
		g.compileSyscallUnlink_win64()
	case "GetCurrentDirectoryA":
		g.compileSyscallGetcwd_win64()
	case "GetCommandLineA":
		g.compileSyscallGetCommandLine_win64()
	case "GetEnvironmentStringsA":
		g.compileSyscallGetEnvStrings_win64()
	case "FindFirstFileA":
		g.compileSyscallFindFirstFile_win64()
	case "FindNextFileA":
		g.compileSyscallFindNextFile_win64()
	case "FindClose":
		g.compileSyscallFindClose_win64()
	case "CreateProcessA":
		g.compileSyscallCreateProcess_win64()
	case "WaitForSingleObject":
		g.compileSyscallWaitProcess_win64()
	case "CreatePipe":
		g.compileSyscallCreatePipe_win64()
	case "SetStdHandle":
		g.compileSyscallSetStdHandle_win64()
	case "GetCurrentProcessId":
		g.compileSyscallGetpid_win64()
	default:
		panic("ICE: unknown windows linkstatic symbol '" + sym + "'")
	}
	return true
}
