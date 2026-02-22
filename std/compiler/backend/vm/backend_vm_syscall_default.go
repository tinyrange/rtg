//go:build !no_backend_vm && !wasi

package vm

import "os"

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
			fd := int(a0)
			bufAddr := a1
			count := a2
			if fd < 0 || fd >= 256 || !vm.fdUsed[fd] {
				vm.vmSysReturn(-1)
				return
			}
			n := int(count)
			buf := make([]byte, n)
			f := vm.fdFiles[fd]
			nr, _ := f.Read(buf)
			if nr > 0 {
				vm.copyToVM(int(bufAddr), buf, nr)
			}
			vm.vmSysReturn(int64(nr))
		case 4:
			fd := int(a0)
			bufAddr := a1
			count := a2
			if fd < 0 || fd >= 256 || !vm.fdUsed[fd] {
				vm.vmSysReturn(-1)
				return
			}
			n := int(count)
			a := int(bufAddr)
			if a+n > len(vm.memory) {
				n = len(vm.memory) - a
			}
			if n <= 0 {
				vm.vmSysReturn(0)
				return
			}
			f := vm.fdFiles[fd]
			nw, err := f.Write(vm.memory[a : a+n])
			if err != nil {
				vm.vmSysReturn(-1)
			} else {
				vm.vmSysReturn(int64(nw))
			}
		case 5:
			path := vm.readCString(a0)
			fl := int(a1)
			var flag int
			if fl&1 != 0 {
				flag = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
			} else if fl&2 != 0 {
				flag = os.O_RDWR
			} else {
				flag = os.O_RDONLY
			}
			f, err := os.OpenFile(path, flag, 0644)
			if err != nil {
				vm.vmSysReturn(-2)
				return
			}
			fd := vm.nextFD
			vm.nextFD = vm.nextFD + 1
			if fd >= 256 {
				f.Close()
				vm.vmSysReturn(-1)
				return
			}
			vm.fdFiles[fd] = f
			vm.fdUsed[fd] = true
			vm.vmSysReturn(int64(fd))
		case 6:
			fd := int(a0)
			if fd < 3 || fd >= 256 || !vm.fdUsed[fd] {
				vm.vmSysReturn(-1)
				return
			}
			f := vm.fdFiles[fd]
			f.Close()
			vm.fdFiles[fd] = nil
			vm.fdUsed[fd] = false
			vm.vmSysReturn(0)
		case 10:
			path := vm.readCString(a0)
			err := os.RemoveAll(path)
			if err != nil {
				vm.vmSysReturn(-1)
			} else {
				vm.vmSysReturn(0)
			}
		case 15:
			path := vm.readCString(a0)
			err := os.Chmod(path, os.FileMode(a1))
			if err != nil {
				vm.vmSysReturn(-1)
			} else {
				vm.vmSysReturn(0)
			}
		case 20:
			vm.push(uint64(os.Getpid()) & vm.config.WordMask)
			vm.push(0)
			vm.push(0)
		case 39:
			path := vm.readCString(a0)
			err := os.MkdirAll(path, 0755)
			if err != nil {
				vm.vmSysReturn(-17)
			} else {
				vm.vmSysReturn(0)
			}
		case 40:
			path := vm.readCString(a0)
			err := os.RemoveAll(path)
			if err != nil {
				vm.vmSysReturn(-1)
			} else {
				vm.vmSysReturn(0)
			}
		case 106:
			path := vm.readCString(a0)
			f, err := os.Open(path)
			if err == nil {
				f.Close()
				vm.vmSysReturn(0)
			} else {
				vm.vmSysReturn(-2)
			}
		case 183:
			cwd, err := os.Getwd()
			if err != nil {
				vm.vmSysReturn(-1)
				return
			}
			n := len(cwd)
			if n >= int(a1) {
				n = int(a1) - 1
			}
			vm.copyStringToVM(int(a0), cwd, n)
			vm.memory[int(a0)+n] = 0
			vm.vmSysReturn(int64(n))
		case 192:
			size := a1
			if size == 0 {
				size = 1
			}
			addr := vm.alloc(size, "mmap")
			vm.vmSysReturn(int64(addr))
		case 252:
			ExitCode = int(vm.signExtend(a0))
			vm.exited = true
		default:
			vm.vmSysReturn(-38)
		}
	}
}
