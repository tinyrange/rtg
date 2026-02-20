//go:build no_size_analysis

package ir

type FuncSize struct {
	Name string
	Size int
}

var FuncSizes []FuncSize
var SizeAnalysisPath string

func CollectNativeFuncSizes(irmod *IRModule, funcOffsets map[string]int, codeLen int) {}
func WriteSizeAnalysis()                                                              {}
