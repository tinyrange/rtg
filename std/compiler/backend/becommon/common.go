package becommon

// === String literal helpers ===

// decodeStringLiteral processes escape sequences in a string literal.
func DecodeStringLiteral(s string) string {
	var result []byte
	i := 0
	for i < len(s) {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'n':
				result = append(result, '\n')
			case 't':
				result = append(result, '\t')
			case 'r':
				result = append(result, '\r')
			case '\\':
				result = append(result, '\\')
			case '"':
				result = append(result, '"')
			case '\'':
				result = append(result, '\'')
			case '0':
				result = append(result, 0)
			case 'x':
				if i+3 < len(s) {
					hi := Unhex(s[i+2])
					lo := Unhex(s[i+3])
					result = append(result, byte(hi<<4|lo))
					i = i + 4
					continue
				}
				result = append(result, s[i+1])
			default:
				result = append(result, s[i+1])
			}
			i = i + 2
		} else {
			result = append(result, s[i])
			i++
		}
	}
	return string(result)
}

func Unhex(c byte) int {
	if c >= '0' && c <= '9' {
		return int(c - '0')
	}
	if c >= 'a' && c <= 'f' {
		return int(c - 'a' + 10)
	}
	if c >= 'A' && c <= 'F' {
		return int(c - 'A' + 10)
	}
	return 0
}

// DispatchEntry pairs a type ID with a method function name for interface dispatch.
type DispatchEntry struct {
	TypeID   int
	FuncName string
}
