package services

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wpmed-videowiki/OWIDImporter/constants"
	"github.com/wpmed-videowiki/OWIDImporter/utils"
)

// discoverChartCountries keeps the sidebar available as a compatibility fallback
// for any chart whose historical CSV cannot be fetched or parsed.
func discoverChartCountries(chartURL string, sidebar func() ([]string, map[string]string, error)) ([]string, map[string]string, error) {
	codes, names, csvErr := getHistoricalChartCountries(chartURL)
	if csvErr == nil {
		return codes, names, nil
	}
	fmt.Println("Historical CSV discovery unavailable; using entity sidebar:", csvErr)
	codes, names, sidebarErr := sidebar()
	if sidebarErr != nil {
		return nil, nil, fmt.Errorf("historical CSV discovery failed (%v); sidebar discovery failed: %w", csvErr, sidebarErr)
	}
	return codes, names, nil
}

// countryNamesFromLabels uses name-only sidebar text, never the selected-year
// value next to it. These aliases retain supported codes for OWID's newer names.
func countryNamesFromLabels(labels []string) ([]string, map[string]string, error) {
	aliases := map[string]string{
		"Palestine":                    "PSE",
		"British Virgin Islands":       "VGB",
		"Curacao":                      "CUW",
		"United States Virgin Islands": "VIR",
	}
	names := make(map[string]string)
	for _, label := range labels {
		name := strings.TrimSpace(label)
		code := constants.COUNTRY_CODES[name]
		if code == "" {
			code = aliases[name]
		}
		if code != "" {
			names[code] = name
		}
	}
	if len(names) == 0 {
		return nil, nil, fmt.Errorf("no supported entities found in sidebar")
	}
	codes := make([]string, 0, len(names))
	for code := range names {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return codes, names, nil
}

// getHistoricalChartCountries discovers supported entities from observations
// across the entire series, rather than the entity picker's selected year.
func getHistoricalChartCountries(chartURL string) ([]string, map[string]string, error) {
	u, err := url.Parse(chartURL)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing chart URL: %w", err)
	}
	u.Path = strings.TrimSuffix(u.Path, ".csv") + ".csv"
	u.Fragment = ""
	q := u.Query()
	for _, key := range []string{"tab", "country", "time", "mapSelect", "region"} {
		q.Del(key)
	}
	q.Set("v", "1")
	q.Set("csvType", "full")
	q.Set("useColumnShortNames", "false")
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, nil, fmt.Errorf("creating chart data request: %w", err)
	}
	if userAgent := os.Getenv("OWID_UA"); userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("fetching historical chart data: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("fetching historical chart data: unexpected status %s", resp.Status)
	}
	return parseHistoricalChartCountries(resp.Body)
}

func parseHistoricalChartCountries(data io.Reader) ([]string, map[string]string, error) {
	r := csv.NewReader(data)
	r.ReuseRecord = true
	header, err := r.Read()
	if err != nil {
		return nil, nil, fmt.Errorf("reading chart CSV header: %w", err)
	}
	codeIdx, dateIdx, entityIdx := -1, -1, -1
	var valueIndices []int
	for i, name := range header {
		switch strings.TrimSpace(name) {
		case "Code":
			codeIdx = i
		case "Year", "Day":
			dateIdx = i
		case "Entity":
			entityIdx = i
		default:
			valueIndices = append(valueIndices, i)
		}
	}
	if entityIdx < 0 || codeIdx < 0 || dateIdx < 0 || len(valueIndices) == 0 {
		return nil, nil, fmt.Errorf("chart CSV requires Entity, Code, Year/Day, and observation columns: %v", header)
	}

	supported := constants.GetCountryCodeNameMap()
	names := make(map[string]string)
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("reading chart CSV: %w", err)
		}
		code := strings.TrimSpace(record[codeIdx])
		if code == "" && entityIdx >= 0 {
			code = constants.COUNTRY_CODES[strings.TrimSpace(record[entityIdx])]
		}
		if _, ok := supported[code]; !ok || names[code] != "" {
			continue
		}
		if _, err := utils.ParseDate(strings.TrimSpace(record[dateIdx])); err != nil {
			continue
		}
		for _, i := range valueIndices {
			value, err := strconv.ParseFloat(strings.TrimSpace(record[i]), 64)
			if err == nil && !math.IsNaN(value) && !math.IsInf(value, 0) {
				name := strings.TrimSpace(record[entityIdx])
				if name == "" {
					return nil, nil, fmt.Errorf("chart CSV has no entity name for %s", code)
				}
				names[code] = name
				break
			}
		}
	}
	countries := make([]string, 0, len(names))
	for code := range names {
		countries = append(countries, code)
	}
	sort.Strings(countries)
	return countries, names, nil
}

// countryChartURL preserves metric choices while overriding map selections and
// requesting the full series for the initial entity in a reusable page.
func countryChartURL(chartURL, chartParameters, code string) (string, error) {
	u, err := url.Parse(chartURL)
	if err != nil {
		return "", fmt.Errorf("parsing country chart URL: %w", err)
	}
	params, err := url.ParseQuery(chartParameters)
	if err != nil {
		return "", fmt.Errorf("parsing chart parameters: %w", err)
	}
	q := u.Query()
	for key, values := range params {
		q[key] = values
	}
	q.Set("tab", "chart")
	q.Set("country", "~"+code)
	q.Set("time", "earliest..latest")
	u.RawQuery = q.Encode()
	return u.String(), nil
}
