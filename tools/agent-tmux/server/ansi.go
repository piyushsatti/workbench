package main

import "strings"

// stripANSI removes escape sequences (7-bit and C1 forms) and every control character except \n and \t.
func stripANSI(s string) string {
	rs := []rune(strings.ToValidUTF8(s, "�"))
	var b strings.Builder
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case r == 0x1b:
			i = skipEscape(rs, i)
		case r == 0x9b: // C1 CSI
			i = skipCSI(rs, i+1)
		case r == 0x90 || r == 0x98 || r == 0x9d || r == 0x9e || r == 0x9f: // C1 DCS, SOS, OSC, PM, APC
			i = skipString(rs, i+1)
		case r == '\n' || r == '\t':
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f):
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Each skip* returns the index of the last rune consumed.
func skipEscape(rs []rune, i int) int {
	if i+1 >= len(rs) {
		return i
	}
	switch rs[i+1] {
	case '[':
		return skipCSI(rs, i+2)
	case ']', 'P', 'X', '^', '_':
		return skipString(rs, i+2)
	}
	j := i + 1
	for j < len(rs) && rs[j] >= 0x20 && rs[j] <= 0x2f { // intermediates, then one final
		j++
	}
	if j >= len(rs) {
		return len(rs) - 1
	}
	return j
}

func skipCSI(rs []rune, j int) int {
	for ; j < len(rs); j++ {
		r := rs[j]
		if r >= 0x40 && r <= 0x7e {
			return j
		}
		if r < 0x20 || r > 0x3f {
			return j - 1 // malformed: stop, let the caller handle this rune
		}
	}
	return len(rs) - 1
}

// skipString consumes an OSC/DCS-style string up to BEL or ST (ESC \ or 0x9c); unterminated swallows the rest.
func skipString(rs []rune, j int) int {
	for ; j < len(rs); j++ {
		switch rs[j] {
		case 0x07, 0x9c:
			return j
		case 0x1b:
			if j+1 < len(rs) && rs[j+1] == '\\' {
				return j + 1
			}
		}
	}
	return len(rs) - 1
}
