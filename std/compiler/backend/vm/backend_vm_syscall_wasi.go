//go:build !no_backend_vm && wasi

package vm

func (vm *VM) execSyscallIntrinsic(num uint64, ws uint64, a0 uint64, a1 uint64, a2 uint64, a3 uint64, a4 uint64, a5 uint64) {
	vm.vmSysReturn(-38)
}
