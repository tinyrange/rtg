package main

func walkEmbedDir() []string {
	return []string{"std/runtime/runtime.go"}
}

func loadEmbeddedFS() []string {
	return []string{"std/runtime/runtime.go"}
}

func loadSources() []string {
	if files := walkEmbedDir(); len(files) > 0 {
		return files
	}
	return loadEmbeddedFS()
}

func main() {
	_ = loadSources() // Repro scaffold for embed-init path drift when preferring disk walk.
}
