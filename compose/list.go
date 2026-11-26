package compose

import "strings"

// SplitList splits a flag value on commas and newlines, trimming each entry
// and dropping empty ones, since a with: value can only be a single string.
func SplitList(s string) []string {
	var out []string
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' }) {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}
