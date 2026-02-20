//go:build no_size_analysis

package main

type FuncSize struct {
	Name string
	Size int
}

var funcSizes []FuncSize
var sizeAnalysisPath string

func collectNativeFuncSizes(irmod *IRModule, funcOffsets map[string]int, codeLen int) {}
func writeSizeAnalysis()                                                               {}
