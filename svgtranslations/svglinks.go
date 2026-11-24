package svgtranslations

import (
	"bytes"
	"html"
	"regexp"
	"strings"
)

var (
	// One scanner over comments and CDATA (skipped, so a "<a" written inside
	// them is never touched) and <a> and <text> start/end tags (<text> only to
	// know whether an <a> sits inside text). Quoted attribute values may contain
	// ">", so start-tag bodies are matched quote-aware. "<textPath" and
	// "<animate" do not match: the name must be followed by space, "/" or ">".
	anchorScanRe = regexp.MustCompile(`(?is)<!--.*?-->|<!\[CDATA\[.*?\]\]>|</(?:a|text)\s*>|<(?:a|text)(?:\s(?:"[^"]*"|'[^']*'|[^"'>])*)?/?>`)
	// One attribute inside a start tag, with the whitespace before it, so it can
	// be dropped without leaving a gap.
	anchorAttrRe = regexp.MustCompile(`(\s+)([^\s=/>]+)\s*=\s*("[^"]*"|'[^']*')`)
	httpSchemeRe = regexp.MustCompile(`^https?://`)
	// MediaWiki's allow-list for data: hrefs (UploadBase::checkSvgScriptCallback):
	// !^data:\s*image/(gif|jpeg|jpg|png)$parameters,!i with RFC 2397 parameters.
	// PHP's atomic groups become plain groups; the token class excludes ";", ","
	// and '"', so no match result changes. PHP \s includes \v, Go's does not.
	safeDataURIRe = regexp.MustCompile(`(?i)^data:[ \t\n\v\f\r]*image/(?:gif|jpeg|jpg|png)` +
		`(?:;[a-zA-Z0-9!#$&'*+.^_\x60{|}~-]+=(?:[a-zA-Z0-9!#$&'*+.^_\x60{|}~-]+|"(?:[\x00-\x0c\x0e-\x21\x23-\x5b\x5d-\x7f]+|\\[\x00-\x7f])*"))*` +
		`(?:;base64)?,`)
)

// linkOnlyAttrs are <a> attributes that mean nothing on a <tspan> or <g>; they are
// dropped together with the href when a link is unwrapped. Any xlink:*
// attribute (xlink:href, xlink:show, xlink:title, ...) is dropped as well.
var linkOnlyAttrs = map[string]bool{
	"href": true, "target": true, "download": true, "ping": true, "rel": true,
	"hreflang": true, "referrerpolicy": true, "type": true,
}

// HrefAllowedOnA reports whether MediaWiki's SVG upload filter accepts value as
// the href (any namespace, so also xlink:href) of an <a> element. It mirrors
// UploadBase::checkSvgScriptCallback (MediaWiki 1.43,
// includes/upload/UploadBase.php): the decoded attribute value is lowercased
// (ASCII, not trimmed) and is allowed when it is empty, starts with "#",
// matches !^https?://!i, or is a data: URI of a gif/jpeg/jpg/png image (any
// other data: fails with uploaded-href-unsafe-target-svg). Everything else
// (relative links, "dod:", "mailto:", " https://" with leading whitespace,
// ...) makes the upload fail with uploaded-href-attribute-svg.
func HrefAllowedOnA(value string) bool {
	v := asciiLower(value)
	return v == "" ||
		(strings.HasPrefix(v, "data:") && safeDataURIRe.MatchString(v)) ||
		strings.HasPrefix(v, "#") ||
		httpSchemeRe.MatchString(v)
}

// UnwrapDisallowedLinks returns svg with every <a> element whose href or
// xlink:href MediaWiki's SVG upload filter rejects (see HrefAllowedOnA)
// turned into a plain container: <tspan> when the <a> is inside an open
// <text> element, <g> otherwise (a <tspan> outside <text>, e.g. around a <g>,
// is invalid SVG and its content is not drawn). The start and matching end
// tag are renamed, the
// href and other link-only attributes are dropped, and every other attribute
// (e.g. style="text-decoration: underline;"), the children and the text stay
// byte-for-byte the same. OWID writes its "details on demand" definitions as
// <a href="dod:..."> inside chart subtitles and notes, which Commons refuses.
//
// Allowed links and everything outside rewritten tags are untouched, and a
// file without disallowed links comes back unchanged (same slice). The tags
// are matched with a quote-aware scanner and a stack, so nested <a> (invalid
// SVG but well-formed XML) still pairs each end tag with its own start tag;
// comments and CDATA are skipped. It is idempotent.
func UnwrapDisallowedLinks(svg []byte) []byte {
	if !bytes.Contains(svg, []byte("<a")) && !bytes.Contains(svg, []byte("<A")) {
		return svg
	}
	locs := anchorScanRe.FindAllIndex(svg, -1)
	var b bytes.Buffer
	changed := false
	pos := 0
	textDepth := 0           // open <text> elements
	var anchorStack []string // per open <a>: replacement element name, "" = kept
	for _, loc := range locs {
		tag := svg[loc[0]:loc[1]]
		if tag[1] == '!' { // comment or CDATA
			continue
		}
		closing := tag[1] == '/'
		nameStart := 1
		if closing {
			nameStart = 2
		}
		selfClosing := !closing && bytes.HasSuffix(tag, []byte("/>"))
		if tag[nameStart] == 't' || tag[nameStart] == 'T' { // <text> / </text>
			if closing && textDepth > 0 {
				textDepth--
			} else if !closing && !selfClosing {
				textDepth++
			}
			continue // copied as part of the next segment
		}
		b.Write(svg[pos:loc[0]])
		pos = loc[1]
		if closing {
			replacement := ""
			if n := len(anchorStack); n > 0 {
				replacement = anchorStack[n-1]
				anchorStack = anchorStack[:n-1]
			}
			if replacement != "" {
				b.WriteString("</" + replacement + ">")
			} else {
				b.Write(tag)
			}
			continue
		}
		replacement := "g"
		if textDepth > 0 {
			replacement = "tspan"
		}
		rewritten, unwrap := unwrapAnchorStartTag(string(tag), selfClosing, replacement)
		if unwrap {
			changed = true
			b.WriteString(rewritten)
		} else {
			b.Write(tag)
			replacement = ""
		}
		if !selfClosing {
			anchorStack = append(anchorStack, replacement)
		}
	}
	if !changed {
		return svg
	}
	b.Write(svg[pos:])
	return b.Bytes()
}

// unwrapAnchorStartTag returns the start tag of element replacement ("tspan"
// or "g") that replaces tag, and true, when tag carries an href MediaWiki
// rejects on <a>; otherwise "", false.
func unwrapAnchorStartTag(tag string, selfClosing bool, replacement string) (string, bool) {
	end := len(tag) - 1 // drop ">"
	if selfClosing {
		end--
	}
	attrs := tag[2:end] // after "<a"
	matches := anchorAttrRe.FindAllStringSubmatchIndex(attrs, -1)
	disallowed := false
	for _, m := range matches {
		if attrLocal(attrs[m[4]:m[5]]) == "href" && !HrefAllowedOnA(attrValue(attrs[m[6]:m[7]])) {
			disallowed = true
			break
		}
	}
	if !disallowed {
		return "", false
	}
	var b strings.Builder
	b.WriteString("<" + replacement)
	pos := 0
	for _, m := range matches {
		if isLinkOnlyAttr(attrs[m[4]:m[5]]) {
			b.WriteString(attrs[pos:m[0]])
			pos = m[1]
		}
	}
	b.WriteString(attrs[pos:])
	if selfClosing {
		b.WriteString("/>")
	} else {
		b.WriteString(">")
	}
	return b.String(), true
}

// attrLocal is the lowercased local part of an attribute name, as MediaWiki's
// splitXmlNamespace sees it ("xlink:href" -> "href").
func attrLocal(name string) string {
	name = asciiLower(name)
	return name[strings.LastIndex(name, ":")+1:]
}

func isLinkOnlyAttr(name string) bool {
	lower := asciiLower(name)
	return attrLocal(lower) == "href" || strings.HasPrefix(lower, "xlink:") ||
		(!strings.Contains(lower, ":") && linkOnlyAttrs[lower])
}

// attrValue is the value an XML parser reports for a quoted attribute: quotes
// removed, character/entity references decoded, and literal tab, CR and LF
// normalized to spaces.
func attrValue(quoted string) string {
	v := html.UnescapeString(quoted[1 : len(quoted)-1])
	return strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' {
			return ' '
		}
		return r
	}, v)
}

// asciiLower lowercases A-Z only, like PHP's strtolower.
func asciiLower(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return r
	}, s)
}
