package services

import (
	"net/url"
	"regexp"
	"strings"
)

// Helpers deciding whether an existing Commons file is the old-named version of
// the country chart being imported, so it can safely be moved to the new name.
// Matching must be exact: substring checks used to move files of other charts
// (e.g. "Electric car sales share" into "Electric car sales").

// normalizeTitle lowercases and treats "-", "_" and runs of spaces as one space,
// so "File:Death-rate_smoking" and "file:death rate smoking" compare equal.
func normalizeTitle(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.NewReplacer("-", " ", "_", " ").Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

// isOldFormatTitleFor reports whether title is exactly
// "File:<name>, <startYear> to <endYear>, <region>.svg".
// The end year is free (it may have changed, and can be a date such as
// "Feb 22, 2026"). The region must match exactly: "NAM" is Namibia while
// "OWID_NAM" is North America.
func isOldFormatTitleFor(title, name, startYear, region string) bool {
	name, startYear, region = normalizeTitle(name), normalizeTitle(startYear), normalizeTitle(region)
	if name == "" || startYear == "" || region == "" {
		return false
	}
	re := regexp.MustCompile("^file:" + regexp.QuoteMeta(name) +
		", " + regexp.QuoteMeta(startYear) +
		" to .+, " + regexp.QuoteMeta(region) + `\.svg$`)
	return re.MatchString(normalizeTitle(title))
}

var owidURLRe = regexp.MustCompile(`(?i)https?://(?:www\.)?ourworldindata\.org/[^\s\]|<>"]*`)

// Query parameters that only change the view, not which chart it is.
var viewOnlyParams = map[string]bool{"tab": true, "time": true, "region": true, "country": true, "facet": true, "mapselect": true}

// chartKey identifies a chart by its lowercased path plus its meaningful query
// parameters, sorted. Explorers need the parameters: one explorer path holds many charts.
func chartKey(raw string) string {
	u, err := url.Parse(strings.TrimRight(raw, ".,;)"))
	if err != nil {
		return ""
	}
	path := strings.ToLower(strings.TrimRight(u.Path, "/"))
	if path == "" {
		return ""
	}
	q := url.Values{}
	for k, vs := range u.Query() {
		if !viewOnlyParams[strings.ToLower(k)] {
			q[k] = vs
		}
	}
	if enc := q.Encode(); enc != "" { // Encode sorts by key
		return path + "?" + enc
	}
	return path
}

// pageIsForChart reports whether any Our World in Data URL in the page text is
// the same chart as chartURL.
func pageIsForChart(pageText, chartURL string) bool {
	want := chartKey(chartURL)
	if want == "" {
		return false
	}
	for _, m := range owidURLRe.FindAllString(pageText, -1) {
		if chartKey(m) == want {
			return true
		}
	}
	return false
}
