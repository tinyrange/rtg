package main

import (
	"fmt"
	"runtime"
	"strings"
)

func defaultPtrSize() int {
	if runtime.GOARCH == "386" || runtime.GOARCH == "wasm32" {
		return 4
	}
	return 8
}

func applyTargetFlag(target string) string {
	if target == "c" || strings.HasPrefix(target, "c/") {
		targetBackend = "c"
		targetCModel = 64
		if strings.HasPrefix(target, "c/") {
			model := target[2:]
			if model == "16" {
				targetCModel = 16
			} else if model == "32" {
				targetCModel = 32
			} else if model == "64" {
				targetCModel = 64
			} else {
				return fmt.Sprintf("invalid target %q: expected c, c/16, c/32, or c/64", target)
			}
		}
		if targetCModel == 16 {
			targetPtrSize = 2
		} else if targetCModel == 32 {
			targetPtrSize = 4
		} else {
			targetPtrSize = 8
		}
		targetGOOS = "c"
		targetGOARCH = fmt.Sprintf("c%d", targetCModel)
		return ""
	}
	if target == "ir" {
		targetBackend = "ir"
		return ""
	}
	if strings.HasPrefix(target, "vm/") {
		targetBackend = "vm"
		model := target[3:]
		if model == "8" {
			targetWordSize = 1
			targetPtrSize = 2
		} else if model == "16" {
			targetWordSize = 2
			targetPtrSize = 2
		} else if model == "32" {
			targetWordSize = 4
			targetPtrSize = 4
		} else if model == "64" {
			targetWordSize = 8
			targetPtrSize = 8
		} else {
			return fmt.Sprintf("invalid target %q: expected vm/8, vm/16, vm/32, or vm/64", target)
		}
		targetGOOS = "c"
		bits := targetWordSize * 8
		targetGOARCH = fmt.Sprintf("c%d", bits)
		return ""
	}
	if target == "dos/8086" {
		targetGOOS = "dos"
		targetGOARCH = "dos16"
		targetPtrSize = 2
		return ""
	}

	slashIdx := strings.Index(target, "/")
	if slashIdx < 0 {
		return fmt.Sprintf("invalid target %q: expected os/arch, dos/8086, c[/16|32|64], ir, or vm/<8|16|32|64>", target)
	}
	targetGOOS = target[0:slashIdx]
	targetGOARCH = target[slashIdx+1:]
	if targetGOARCH == "386" || targetGOARCH == "wasm32" {
		targetPtrSize = 4
	} else {
		targetPtrSize = 8
	}
	return ""
}

func rebuildActiveBuildTags(extraTags string) {
	buildTags = buildTags[:0]
	if targetBackend == "c" {
		buildTags = append(buildTags, "c")
		buildTags = append(buildTags, fmt.Sprintf("c%d", targetCModel))
	} else if targetGOOS == "wasi" && targetGOARCH == "wasm32" {
		buildTags = append(buildTags, "wasi")
		buildTags = append(buildTags, "wasm32")
	} else {
		buildTags = append(buildTags, targetGOOS)
		buildTags = append(buildTags, targetGOARCH)
	}
	if extraTags != "" {
		parts := strings.Split(extraTags, ",")
		for _, t := range parts {
			if t != "" {
				buildTags = append(buildTags, t)
			}
		}
	}
	buildTags = append(buildTags, "rtg")
}
