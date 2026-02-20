//go:build no_size_analysis

package ir

type FuncSize struct {
	Name string
	Size int
}

var funcSizes []FuncSize
var sizeAnalysisPath string

func CollectNativeFuncSizes(irmod *IRModule, funcOffsets map[string]int, codeLen int) {}
func WriteSizeAnalysis()                                                              {}
