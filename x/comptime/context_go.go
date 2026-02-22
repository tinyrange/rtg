//go:build !rtg

package comptime

import "os"

type hostContext struct{}

func Host() Context {
	return hostContext{}
}

func (h hostContext) ReadFile(path string) (string, bool) {
	_ = h
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return string(data), true
}
