//go:build no_backend_windows_i386

package i386

import (
	"fmt"

	"j5.nz/rtg/std/compiler/common"
	"j5.nz/rtg/std/compiler/ir"
)

func GenerateWinPE(target *common.Target, irmod *ir.IRModule, outputPath string) error {
	return fmt.Errorf("windows/386 backend disabled (built with no_backend_windows_i386 tag)")
}

func generateWin386PE(irmod *IRModule, outputPath string) error {
	return fmt.Errorf("windows/386 backend disabled (built with no_backend_windows_i386 tag)")
}

func (g *CodeGen) compileSyscallMmap_win386()           { panic("windows/386 backend disabled") }
func (g *CodeGen) compileSyscallWrite_win386()          { panic("windows/386 backend disabled") }
func (g *CodeGen) compileSyscallRead_win386()           { panic("windows/386 backend disabled") }
func (g *CodeGen) compileSyscallOpen_win386()           { panic("windows/386 backend disabled") }
func (g *CodeGen) compileSyscallClose_win386()          { panic("windows/386 backend disabled") }
func (g *CodeGen) compileSyscallExit_win386()           { panic("windows/386 backend disabled") }
func (g *CodeGen) compileSyscallMkdir_win386()          { panic("windows/386 backend disabled") }
func (g *CodeGen) compileSyscallRmdir_win386()          { panic("windows/386 backend disabled") }
func (g *CodeGen) compileSyscallUnlink_win386()         { panic("windows/386 backend disabled") }
func (g *CodeGen) compileSyscallGetcwd_win386()         { panic("windows/386 backend disabled") }
func (g *CodeGen) compileSyscallGetdents_win386()       { panic("windows/386 backend disabled") }
func (g *CodeGen) compileSyscallGetCommandLine_win386() { panic("windows/386 backend disabled") }
func (g *CodeGen) compileSyscallGetEnvStrings_win386()  { panic("windows/386 backend disabled") }
func (g *CodeGen) compileSyscallGetpid_win386()         { panic("windows/386 backend disabled") }
func (g *CodeGen) compileSyscallFindFirstFile_win386()  { panic("windows/386 backend disabled") }
func (g *CodeGen) compileSyscallFindNextFile_win386()   { panic("windows/386 backend disabled") }
func (g *CodeGen) compileSyscallFindClose_win386()      { panic("windows/386 backend disabled") }
func (g *CodeGen) compileSyscallCreateProcess_win386()  { panic("windows/386 backend disabled") }
func (g *CodeGen) compileSyscallWaitProcess_win386()    { panic("windows/386 backend disabled") }
func (g *CodeGen) compileSyscallCreatePipe_win386()     { panic("windows/386 backend disabled") }
func (g *CodeGen) compileSyscallSetStdHandle_win386()   { panic("windows/386 backend disabled") }
func (g *CodeGen) compileSyscallStat_win386()           { panic("windows/386 backend disabled") }
func (g *CodeGen) compileLinkStaticIntrinsicWin386(inst ir.Inst) bool {
	return false
}
func (g *CodeGen) compilePanic_win386() { panic("windows/386 backend disabled") }
