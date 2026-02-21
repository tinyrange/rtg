//go:build !no_backend_arm64

package aarch64

import "j5.nz/rtg/std/compiler/ir"

func (g *CodeGen) compileLinkStaticIntrinsicArm64Windows(inst ir.Inst) bool {
	if g.target.GOOS != "windows" || g.irmod == nil || g.irmod.LinkStaticFuncs == nil {
		return false
	}
	raw, ok := g.irmod.LinkStaticFuncs[inst.Name]
	if !ok {
		return false
	}
	lib, sym, _, ok := decodeLinkStaticSpec(raw)
	if !ok {
		panic("ICE: invalid windows linkstatic metadata for '" + inst.Name + "'")
	}
	if lib != "kernel32.dll" {
		panic("ICE: unsupported windows linkstatic library '" + lib + "'")
	}

	switch sym {
	case "ReadFile":
		g.compileSyscallRead_winarm64()
	case "WriteFile":
		g.compileSyscallWrite_winarm64()
	case "CreateFileA":
		g.compileSyscallOpen_winarm64()
	case "CloseHandle":
		g.compileSyscallClose_winarm64()
	case "GetFileAttributesExA":
		g.compileSyscallStat_winarm64()
	case "ExitProcess":
		g.compileSyscallExit_winarm64()
	case "VirtualAlloc":
		g.compileSyscallMmap_winarm64()
	case "CreateDirectoryA":
		g.compileSyscallMkdir_winarm64()
	case "RemoveDirectoryA":
		g.compileSyscallRmdir_winarm64()
	case "DeleteFileA":
		g.compileSyscallUnlink_winarm64()
	case "GetCurrentDirectoryA":
		g.compileSyscallGetcwd_winarm64()
	case "GetCommandLineA":
		g.compileSyscallGetCommandLine_winarm64()
	case "GetEnvironmentStringsA":
		g.compileSyscallGetEnvStrings_winarm64()
	case "FindFirstFileA":
		g.compileSyscallFindFirstFile_winarm64()
	case "FindNextFileA":
		g.compileSyscallFindNextFile_winarm64()
	case "FindClose":
		g.compileSyscallFindClose_winarm64()
	case "CreateProcessA":
		g.compileSyscallCreateProcess_winarm64()
	case "WaitForSingleObject":
		g.compileSyscallWaitProcess_winarm64()
	case "CreatePipe":
		g.compileSyscallCreatePipe_winarm64()
	case "SetStdHandle":
		g.compileSyscallSetStdHandle_winarm64()
	case "GetCurrentProcessId":
		g.compileSyscallGetpid_winarm64()
	default:
		panic("ICE: unknown windows linkstatic symbol '" + sym + "'")
	}
	return true
}
