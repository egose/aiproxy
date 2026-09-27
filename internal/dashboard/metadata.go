package dashboard

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const metadataDisplayBytes = 512
const metadataDisplayClipped = " [display clipped at 512 bytes]"

func metadataText(s string) string {
	var out strings.Builder
	used := 0
	for len(s) > 0 {
		r, n := utf8.DecodeRuneInString(s)
		if used+n > metadataDisplayBytes {
			out.WriteString(metadataDisplayClipped)
			break
		}
		switch {
		case r == utf8.RuneError && n == 1:
			fmt.Fprintf(&out, "\\x%02x", s[0])
		case r == '\\':
			out.WriteString(`\\`)
		case r == '"':
			out.WriteString(`\"`)
		case unicode.IsControl(r) || r == '\u2028' || r == '\u2029':
			quoted := strconv.QuoteRune(r)
			out.WriteString(quoted[1 : len(quoted)-1])
		default:
			out.WriteRune(r)
		}
		used += n
		s = s[n:]
	}
	return out.String()
}
