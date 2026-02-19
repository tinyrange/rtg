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

func parseTargetFlag(base CompilerTarget, target string) (CompilerTarget, string) {
	next := base
	if target == "c" || strings.HasPrefix(target, "c/") {
		next.Backend = "c"
		next.CModel = 64
		if strings.HasPrefix(target, "c/") {
			model := target[2:]
			if model == "16" {
				next.CModel = 16
			} else if model == "32" {
				next.CModel = 32
			} else if model == "64" {
				next.CModel = 64
			} else {
				return next, fmt.Sprintf("invalid target %q: expected c, c/16, c/32, or c/64", target)
			}
		}
		if next.CModel == 16 {
			next.PtrSize = 2
		} else if next.CModel == 32 {
			next.PtrSize = 4
		} else {
			next.PtrSize = 8
		}
		next.GOOS = "c"
		next.GOARCH = fmt.Sprintf("c%d", next.CModel)
		return next, ""
	}
	if target == "ir" {
		next.Backend = "ir"
		return next, ""
	}
	if strings.HasPrefix(target, "vm/") {
		next.Backend = "vm"
		model := target[3:]
		if model == "8" {
			next.WordSize = 1
			next.PtrSize = 2
		} else if model == "16" {
			next.WordSize = 2
			next.PtrSize = 2
		} else if model == "32" {
			next.WordSize = 4
			next.PtrSize = 4
		} else if model == "64" {
			next.WordSize = 8
			next.PtrSize = 8
		} else {
			return next, fmt.Sprintf("invalid target %q: expected vm/8, vm/16, vm/32, or vm/64", target)
		}
		next.GOOS = "c"
		bits := next.WordSize * 8
		next.GOARCH = fmt.Sprintf("c%d", bits)
		return next, ""
	}
	if target == "dos/8086" {
		next.GOOS = "dos"
		next.GOARCH = "dos16"
		next.PtrSize = 2
		return next, ""
	}

	slashIdx := strings.Index(target, "/")
	if slashIdx < 0 {
		return next, fmt.Sprintf("invalid target %q: expected os/arch, dos/8086, c[/16|32|64], ir, or vm/<8|16|32|64>", target)
	}
	next.GOOS = target[0:slashIdx]
	next.GOARCH = target[slashIdx+1:]
	if next.GOARCH == "386" || next.GOARCH == "wasm32" {
		next.PtrSize = 4
	} else {
		next.PtrSize = 8
	}
	return next, ""
}

func buildActiveBuildTagsForTarget(target CompilerTarget, extraTags string) []string {
	var tags []string
	if target.Backend == "c" {
		tags = append(tags, "c")
		tags = append(tags, fmt.Sprintf("c%d", target.CModel))
	} else if target.GOOS == "wasi" && target.GOARCH == "wasm32" {
		tags = append(tags, "wasi")
		tags = append(tags, "wasm32")
	} else {
		tags = append(tags, target.GOOS)
		tags = append(tags, target.GOARCH)
	}
	if extraTags != "" {
		parts := strings.Split(extraTags, ",")
		for _, t := range parts {
			if t != "" {
				tags = append(tags, t)
			}
		}
	}
	tags = append(tags, "rtg")
	return tags
}
