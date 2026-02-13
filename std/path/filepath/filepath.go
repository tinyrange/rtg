package filepath

func Join(a string, b string) string {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}
	// Trim trailing / from a
	for len(a) > 0 && a[len(a)-1] == '/' {
		a = a[0 : len(a)-1]
	}
	// Trim leading / from b
	for len(b) > 0 && b[0] == '/' {
		b = b[1:len(b)]
	}
	if len(b) == 0 {
		return a
	}
	return a + "/" + b
}

func Dir(path string) string {
	i := len(path) - 1
	for i >= 0 && path[i] != '/' {
		i = i - 1
	}
	if i < 0 {
		return "."
	}
	if i == 0 {
		return "/"
	}
	return path[0:i]
}

func Base(path string) string {
	if len(path) == 0 {
		return "."
	}
	// Strip trailing slashes
	for len(path) > 1 && path[len(path)-1] == '/' {
		path = path[0 : len(path)-1]
	}
	i := len(path) - 1
	for i >= 0 && path[i] != '/' {
		i = i - 1
	}
	return path[i+1 : len(path)]
}
