//go:build !no_backend_i386

package i386

import "j5.nz/rtg/std/compiler/ir"

type OSHooks interface {
	EmitStart(g *CodeGen, irmod *ir.IRModule)
	CompileSyscallIntrinsic(g *CodeGen, paramCount int)
	CompileSysGetdents64(g *CodeGen)
	CompileLinkStaticIntrinsic(g *CodeGen, inst ir.Inst) bool
	CompilePanic(g *CodeGen)
}

type osHookEntry struct {
	goos string
	hook OSHooks
}

var osHookEntries []osHookEntry

func RegisterOSHooks(goos string, hook OSHooks) {
	i := 0
	for i < len(osHookEntries) {
		if osHookEntries[i].goos == goos {
			osHookEntries[i].hook = hook
			return
		}
		i++
	}
	osHookEntries = append(osHookEntries, osHookEntry{goos: goos, hook: hook})
}

func hooksForGOOS(goos string) OSHooks {
	i := 0
	for i < len(osHookEntries) {
		if osHookEntries[i].goos == goos {
			return osHookEntries[i].hook
		}
		i++
	}
	i = 0
	for i < len(osHookEntries) {
		if osHookEntries[i].goos == "linux" {
			return osHookEntries[i].hook
		}
		i++
	}
	return nil
}
