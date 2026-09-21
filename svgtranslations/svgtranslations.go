// Package svgtranslations re-applies svgtranslate.toolforge.org <switch> translations
// from a file already on Commons onto a freshly downloaded OWID SVG.
//
// It works on the raw markup with regular expressions on purpose: the OWID SVG must be
// uploaded byte-for-byte as downloaded (apart from the wrapped text elements), and Go's
// encoding/xml would re-serialize and reorder attributes.
package svgtranslations

import (
	"html"
	"regexp"
	"strings"
)

var (
	switchRe    = regexp.MustCompile(`(?s)<switch\b[^>]*>.*?</switch>`)
	textRe      = regexp.MustCompile(`(?s)<text\b[^>]*/>|<text\b[^>]*>.*?</text>`)
	textOpenRe  = regexp.MustCompile(`(?s)^<text\b[^>]*>`)
	tspanRe     = regexp.MustCompile(`(?s)<tspan\b[^>]*/>|<tspan\b[^>]*>.*?</tspan>`)
	tspanOpenRe = regexp.MustCompile(`(?s)^<tspan\b[^>]*>`)
	sysLangRe   = regexp.MustCompile(`\ssystemLanguage="([^"]*)"`)
	idAttrRe    = regexp.MustCompile(`\sid="([^"]*)"`)
	tagRe       = regexp.MustCompile(`<[^>]+>`)
	spaceRe     = regexp.MustCompile(`\s+`)
)

// Translation is one translated variant of a text element.
// Parts holds the text of each <tspan> in order, or a single entry when the
// element has no <tspan> children.
type Translation struct {
	Lang  string
	Parts []string
}

// Translations maps the normalized default (untranslated) text content of a
// <switch> block to the translated variants found in it.
type Translations map[string][]Translation

// HasSwitch reports whether the SVG contains any <switch> element.
func HasSwitch(svg []byte) bool {
	return switchRe.Match(svg)
}

// NormalizeText strips tags, decodes HTML entities and collapses whitespace so
// that text from two differently formatted SVG files can be compared.
func NormalizeText(markup string) string {
	text := tagRe.ReplaceAllString(markup, " ")
	text = html.UnescapeString(text)
	return strings.TrimSpace(spaceRe.ReplaceAllString(text, " "))
}

func textParts(textEl string) []string {
	tspans := tspanRe.FindAllString(textEl, -1)
	if len(tspans) == 0 {
		return []string{NormalizeText(textEl)}
	}
	parts := make([]string, len(tspans))
	for i, ts := range tspans {
		parts[i] = NormalizeText(ts)
	}
	return parts
}

// isSelfClosingTextElement reports whether a <text ...> element matched by
// textRe is a self-closing element (e.g. <text id="z" x="5"/>) rather than a
// <text>...</text> pair. Self-closing text elements carry no text content.
func isSelfClosingTextElement(textEl string) bool {
	return strings.HasSuffix(strings.TrimSpace(textEl), "/>") && !strings.Contains(textEl, "</text>")
}

func isTranslated(textEl string) (string, bool) {
	m := sysLangRe.FindStringSubmatch(textOpenRe.FindString(textEl))
	if m == nil {
		return "", false
	}
	return m[1], true
}

// Extract collects the translations from every <switch> block of an SVG that
// has one default <text> and at least one <text systemLanguage="..."> variant.
// When the same default text appears in several blocks, the first block's
// translation for each language is used.
func Extract(existing []byte) Translations {
	result := Translations{}
	for _, sw := range switchRe.FindAllString(string(existing), -1) {
		var defaultText string
		var translated []string
		for _, t := range textRe.FindAllString(sw, -1) {
			if isSelfClosingTextElement(t) {
				continue
			}
			if _, ok := isTranslated(t); ok {
				translated = append(translated, t)
			} else if defaultText == "" {
				defaultText = t
			}
		}
		if defaultText == "" || len(translated) == 0 {
			continue
		}
		key := NormalizeText(defaultText)
		if key == "" {
			continue
		}
		for _, t := range translated {
			lang, _ := isTranslated(t)
			if hasLang(result[key], lang) {
				continue
			}
			result[key] = append(result[key], Translation{Lang: lang, Parts: textParts(t)})
		}
	}
	return result
}

func hasLang(trs []Translation, lang string) bool {
	for _, tr := range trs {
		if tr.Lang == lang {
			return true
		}
	}
	return false
}

// MergeResult reports how many translated text blocks existed in the old file
// and how many could be re-applied to the new one.
type MergeResult struct {
	Available int
	Merged    int
}

// Merge wraps every <text> element of incoming whose normalized content matches a
// translated block of existing in a <switch> carrying the translated variants.
// incoming is returned unchanged when it already contains <switch> elements
// (it is then assumed to be an already translated file) or when nothing matches.
func Merge(existing, incoming []byte) ([]byte, MergeResult) {
	res := MergeResult{}
	if HasSwitch(incoming) {
		return incoming, res
	}
	translations := Extract(existing)
	res.Available = len(translations)
	if res.Available == 0 {
		return incoming, res
	}

	used := map[string]bool{}
	out := textRe.ReplaceAllStringFunc(string(incoming), func(textEl string) string {
		normalized := NormalizeText(textEl)
		trs, ok := translations[normalized]
		if !ok {
			return textEl
		}
		var variants []string
		for _, tr := range trs {
			if v, ok := translateElement(textEl, tr); ok {
				variants = append(variants, v)
			}
		}
		if len(variants) == 0 {
			return textEl
		}
		used[normalized] = true
		return "<switch>" + strings.Join(variants, "") + textEl + "</switch>"
	})
	res.Merged = len(used)
	if res.Merged == 0 {
		return incoming, res
	}
	return []byte(out), res
}

// translateElement clones the incoming <text> element with systemLanguage set,
// ids suffixed with "-<lang>" and the text content replaced by the translation.
func translateElement(textEl string, tr Translation) (string, bool) {
	if isSelfClosingTextElement(textEl) {
		return "", false
	}
	openTag := textOpenRe.FindString(textEl)
	if openTag == "" {
		return "", false
	}
	newOpen := strings.TrimSuffix(suffixIDs(openTag, tr.Lang), ">") + ` systemLanguage="` + tr.Lang + `">`
	body := textEl[len(openTag):]
	tspans := tspanRe.FindAllString(body, -1)
	if len(tspans) == 0 {
		if len(tr.Parts) != 1 {
			return "", false
		}
		return newOpen + html.EscapeString(tr.Parts[0]) + "</text>", true
	}
	if len(tspans) != len(tr.Parts) {
		return "", false
	}
	i := 0
	body = tspanRe.ReplaceAllStringFunc(body, func(ts string) string {
		open := tspanOpenRe.FindString(ts)
		s := suffixIDs(open, tr.Lang) + html.EscapeString(tr.Parts[i]) + "</tspan>"
		i++
		return s
	})
	return newOpen + body, true
}

func suffixIDs(openTag, lang string) string {
	return idAttrRe.ReplaceAllStringFunc(openTag, func(m string) string {
		return strings.TrimSuffix(m, `"`) + "-" + lang + `"`
	})
}

// DefaultTexts returns the normalized content of every <text> element that is
// not a translated variant, in document order.
func DefaultTexts(svg []byte) []string {
	var texts []string
	for _, t := range textRe.FindAllString(string(svg), -1) {
		if _, translated := isTranslated(t); translated {
			continue
		}
		texts = append(texts, NormalizeText(t))
	}
	return texts
}
