package embed

import "strings"

type FS struct {
	names []string
	data  []string
}

func AddFile(f *FS, name string, data string) {
	f.names = append(f.names, name)
	f.data = append(f.data, data)
}

func (f *FS) ReadFile(name string) string {
	i := 0
	for i < len(f.names) {
		if f.names[i] == name {
			return f.data[i]
		}
		i = i + 1
	}
	return ""
}

func (f *FS) WalkDir(dir string) ([]string, []string) {
	prefix := ""
	if dir != "" && dir != "." {
		prefix = dir + "/"
	}
	var names []string
	var data []string
	i := 0
	for i < len(f.names) {
		if prefix == "" || strings.HasPrefix(f.names[i], prefix) {
			name := f.names[i]
			if prefix != "" {
				name = f.names[i][len(prefix):len(f.names[i])]
			}
			names = append(names, name)
			data = append(data, f.data[i])
		}
		i = i + 1
	}
	return names, data
}

func (f *FS) ReadDir(dir string) []string {
	var result []string
	prefix := dir + "/"
	seen := make(map[string]bool)
	i := 0
	for i < len(f.names) {
		if strings.HasPrefix(f.names[i], prefix) {
			rest := f.names[i][len(prefix):len(f.names[i])]
			if !strings.Contains(rest, "/") {
				if !seen[rest] {
					seen[rest] = true
					result = append(result, rest)
				}
			}
		}
		i = i + 1
	}
	return result
}
