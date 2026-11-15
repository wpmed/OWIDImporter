package svgtranslations

import (
	"bytes"
	"regexp"
	"strings"
)

// WordJoiner is U+2060. Commons' SVG Translate tool rejects any SVG whose <text>
// textContent matches /\$[0-9]+/ (it uses $1, $2 as internal placeholders), so a
// WordJoiner is put between "$" and an immediately following digit. It is
// invisible and does not change line breaking.
const WordJoiner = "⁠"

var (
	// A "$" or a digit, written literally or as a numeric character reference.
	dollarForm = `(?:\$|&#0*36;|&#[xX]0*24;)`
	digitForm  = `(?:[0-9]|&#0*(?:4[89]|5[0-7]);|&#[xX]0*3[0-9];)`

	dollarDigitRe = regexp.MustCompile(`(` + dollarForm + `)(` + digitForm + `)`)
	endsDollarRe  = regexp.MustCompile(dollarForm + `$`)
	startsDigitRe = regexp.MustCompile(`^` + digitForm)
)

// InsertDollarJoiner returns svg with a WordJoiner inserted between "$" and an
// immediately following digit in the character data of every <text> element.
// "$" and the digit may be literal or numeric character references, and may be
// split across adjacent <tspan>s (textContent concatenates them). Attributes,
// <style>, <metadata>, ids and path data are never touched, and the markup
// outside the inserted joiners stays byte-for-byte identical. It is idempotent.
func InsertDollarJoiner(svg []byte) []byte {
	if !bytes.Contains(svg, []byte("$")) && !bytes.Contains(svg, []byte("&#")) {
		return svg
	}
	out := textRe.ReplaceAllStringFunc(string(svg), func(textEl string) string {
		if isSelfClosingTextElement(textEl) {
			return textEl
		}
		var b strings.Builder
		pendingDollar := false // last non-empty character data ended with "$"
		pos := 0
		emit := func(seg string) {
			if seg == "" {
				return
			}
			if pendingDollar && startsDigitRe.MatchString(seg) {
				seg = WordJoiner + seg
			}
			seg = dollarDigitRe.ReplaceAllString(seg, "${1}"+WordJoiner+"${2}")
			pendingDollar = endsDollarRe.MatchString(seg)
			b.WriteString(seg)
		}
		for _, loc := range tagRe.FindAllStringIndex(textEl, -1) {
			emit(textEl[pos:loc[0]])
			b.WriteString(textEl[loc[0]:loc[1]])
			pos = loc[1]
		}
		emit(textEl[pos:])
		return b.String()
	})
	return []byte(out)
}

// NeedsDollarJoiner reports whether InsertDollarJoiner would change svg.
func NeedsDollarJoiner(svg []byte) bool {
	return !bytes.Equal(InsertDollarJoiner(svg), svg)
}
