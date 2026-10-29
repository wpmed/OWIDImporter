package utils

import (
	"errors"
	"regexp"
	"strings"
	"unicode"
)

// OWID_SITE_URL is prefixed to a normalized iframe chart path to get the page URL.
const OWID_SITE_URL = "https://ourworldindata.org/"

// IframeChartPathRegex is the pattern the enwiki/MDWiki OWIDPopup gadget
// accepts for data-chart; a normalized path must match it.
var IframeChartPathRegex = regexp.MustCompile(`^(grapher|explorers)\/[a-z0-9-]+?($|\?)`)

var owidSitePrefixRegex = regexp.MustCompile(`^https?://(www\.)?ourworldindata\.org`)

var ErrIframeChartCannotBeImported = errors.New("This chart cannot be imported")

// NormalizeIframeChartPath turns a full OWID URL or a bare path into the path
// form used by Template:OWID (e.g. "grapher/life-expectancy?tab=map").
// The query string is kept exactly as given.
func NormalizeIframeChartPath(input string) (string, error) {
	path := strings.TrimSpace(input)
	path = owidSitePrefixRegex.ReplaceAllString(path, "")
	path = strings.TrimLeft(path, "/")
	if !IframeChartPathRegex.MatchString(path) || breaksWikiTemplate(path) {
		return "", ErrIframeChartCannotBeImported
	}
	return path, nil
}

// breaksWikiTemplate reports whether path would break the {{OWID}} block it is
// written into: template/link delimiters, "|" or any control character (newline, CR, ...).
func breaksWikiTemplate(path string) bool {
	for _, token := range []string{"|", "{{", "}}", "[[", "]]", "<!--", "~~~"} {
		if strings.Contains(path, token) {
			return true
		}
	}
	return strings.IndexFunc(path, unicode.IsControl) >= 0
}

// IframeChartURL returns the OWID page URL for a normalized chart path.
func IframeChartURL(path string) string {
	return OWID_SITE_URL + path
}

// IframeChartSlug returns the chart slug of a normalized path
// ("explorers/minerals?tab=map" -> "minerals").
func IframeChartSlug(path string) string {
	path = strings.SplitN(path, "?", 2)[0]
	if i := strings.Index(path, "/"); i >= 0 {
		path = path[i+1:]
	}
	return path
}
