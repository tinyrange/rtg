package main

func appendUnique(list []string, item string) []string {
	for _, v := range list {
		if v == item {
			return list
		}
	}
	return append(list, item)
}

func possibleTargets(base []string) []string {
	out := base
	out = appendUnique(out, "linux/amd64")
	out = appendUnique(out, "wasi/wasm32")
	return out
}

func main() {
	_ = possibleTargets(nil) // Repro shape for main helper relocation drift.
}
