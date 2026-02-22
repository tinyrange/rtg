//go:build !no_backend_vm && !wasi

package vm

func (vm *VM) execSyscallIntrinsic(num uint64, ws uint64, a0 uint64, a1 uint64, a2 uint64, a3 uint64, a4 uint64, a5 uint64) {
	switch vm.targetGOARCH {
	case "amd64":
		switch num {
		case 0:
			vm.execIntrinsicArgs("SysRead", ws, a0, a1, a2)
		case 1:
			vm.execIntrinsicArgs("SysWrite", ws, a0, a1, a2)
		case 2:
			vm.execIntrinsicArgs("SysOpen", ws, a0, a1, a2)
		case 3:
			vm.execIntrinsicArgs("SysClose", ws, a0)
		case 4:
			vm.execIntrinsicArgs("SysStat", ws, a0, a1)
		case 9:
			vm.execIntrinsicArgs("SysMmap", ws, a0, a1, a2, a3, a4, a5)
		case 39:
			vm.execIntrinsicArgs("SysGetpid", ws)
		case 79:
			vm.execIntrinsicArgs("SysGetcwd", ws, a0, a1)
		case 83:
			vm.execIntrinsicArgs("SysMkdir", ws, a0, a1)
		case 84:
			vm.execIntrinsicArgs("SysRmdir", ws, a0)
		case 87:
			vm.execIntrinsicArgs("SysUnlink", ws, a0)
		case 90:
			vm.execIntrinsicArgs("SysChmod", ws, a0, a1)
		case 231:
			vm.execIntrinsicArgs("SysExit", ws, a0)
		default:
			vm.vmSysReturn(-38)
		}
	case "arm64":
		switch num {
		case 17:
			vm.execIntrinsicArgs("SysGetcwd", ws, a0, a1)
		case 34:
			// arm64 uses mkdirat(dirfd, path, mode); ignore dirfd in VM.
			vm.execIntrinsicArgs("SysMkdir", ws, a1, a2)
		case 35:
			// arm64 uses unlinkat(dirfd, path, flags); 0x200 == AT_REMOVEDIR.
			if a2 == 0x200 {
				vm.execIntrinsicArgs("SysRmdir", ws, a1)
			} else {
				vm.execIntrinsicArgs("SysUnlink", ws, a1)
			}
		case 53:
			// arm64 uses fchmodat(dirfd, path, mode, flags); ignore dirfd/flags.
			vm.execIntrinsicArgs("SysChmod", ws, a1, a2)
		case 56:
			// arm64 uses openat(dirfd, path, flags, mode); ignore dirfd.
			vm.execIntrinsicArgs("SysOpen", ws, a1, a2, a3)
		case 57:
			vm.execIntrinsicArgs("SysClose", ws, a0)
		case 63:
			vm.execIntrinsicArgs("SysRead", ws, a0, a1, a2)
		case 64:
			vm.execIntrinsicArgs("SysWrite", ws, a0, a1, a2)
		case 79:
			// arm64 uses newfstatat(dirfd, path, statbuf, flags); ignore dirfd/flags.
			vm.execIntrinsicArgs("SysStat", ws, a1, a2)
		case 94:
			vm.execIntrinsicArgs("SysExit", ws, a0)
		case 172:
			vm.execIntrinsicArgs("SysGetpid", ws)
		case 222:
			vm.execIntrinsicArgs("SysMmap", ws, a0, a1, a2, a3, a4, a5)
		default:
			vm.vmSysReturn(-38)
		}
	default:
		// linux/386 and dos16 use the same numbering for basic file syscalls.
		switch num {
		case 3:
			vm.execIntrinsicArgs("SysRead", ws, a0, a1, a2)
		case 4:
			vm.execIntrinsicArgs("SysWrite", ws, a0, a1, a2)
		case 5:
			vm.execIntrinsicArgs("SysOpen", ws, a0, a1, a2)
		case 6:
			vm.execIntrinsicArgs("SysClose", ws, a0)
		case 10:
			vm.execIntrinsicArgs("SysUnlink", ws, a0)
		case 15:
			vm.execIntrinsicArgs("SysChmod", ws, a0, a1)
		case 20:
			vm.execIntrinsicArgs("SysGetpid", ws)
		case 39:
			vm.execIntrinsicArgs("SysMkdir", ws, a0, a1)
		case 40:
			vm.execIntrinsicArgs("SysRmdir", ws, a0)
		case 106:
			vm.execIntrinsicArgs("SysStat", ws, a0, a1)
		case 183:
			vm.execIntrinsicArgs("SysGetcwd", ws, a0, a1)
		case 192:
			vm.execIntrinsicArgs("SysMmap", ws, a0, a1, a2, a3, a4, a5)
		case 252:
			vm.execIntrinsicArgs("SysExit", ws, a0)
		default:
			vm.vmSysReturn(-38)
		}
	}
}
