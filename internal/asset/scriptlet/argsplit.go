package scriptlet

import "strings"

// argSplit splits a list of strings by commas, respecting commas inside quoted strings
// and backslash-escaped commas.
func argSplit(input string) []string {
	if input == "" {
		return []string{}
	}

	var (
		res                     []string
		b                       strings.Builder
		inSingle, inDouble, esc bool
	)

	flush := func() {
		s := b.String()
		b.Reset()

		s = strings.TrimSpace(s)
		res = append(res, s)
	}

	for _, c := range input {
		if esc {
			b.WriteByte('\\')
			b.WriteRune(c)
			esc = false
			continue
		}

		switch c {
		case '\\':
			esc = true
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
			b.WriteRune(c)
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
			b.WriteRune(c)
		case ',':
			if inSingle || inDouble {
				b.WriteRune(c)
			} else {
				flush()
			}
		default:
			b.WriteRune(c)
		}
	}

	if esc {
		b.WriteByte('\\')
	}
	flush()

	return res
}
