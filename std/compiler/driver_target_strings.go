package main

import "runtime"

func hostCompilerTarget() CompilerTarget {
	return CompilerTarget{
		GOOS:   runtime.GOOS,
		GOARCH: runtime.GOARCH,
	}
}
