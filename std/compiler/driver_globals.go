package main

import "runtime"

// Target and build-tag globals — defaults to host platform.
var targetGOOS string = runtime.GOOS
var targetGOARCH string = runtime.GOARCH
var targetPtrSize int = defaultPtrSize()

var targetBackend string = "native"       // native, c, ir, or vm
var targetCModel int = 0                  // 16/32/64 when targetBackend==c
var targetWordSize int = defaultPtrSize() // word size in bytes
var buildTags []string
var compilerDebug bool
var stripBinary bool
