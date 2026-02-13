package sort

import "runtime"

func Strings(s []string) {
	n := len(s)
	i := 1
	for i < n {
		j := i
		for j > 0 && runtime.StringLess(s[j], s[j-1]) {
			tmp := s[j]
			s[j] = s[j-1]
			s[j-1] = tmp
			j = j - 1
		}
		i++
	}
}
