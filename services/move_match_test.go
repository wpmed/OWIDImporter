package services

import "testing"

func TestIsOldFormatTitleFor(t *testing.T) {
	cases := []struct {
		name                        string
		title, chart, start, region string
		want                        bool
	}{
		// Valid old -> new renames that must keep working
		{"plain", "File:Banana production, 1961 to 2023, BRA.svg", "Banana production", "1961", "BRA", true},
		{"end year changed", "File:Banana production, 1961 to 2019, BRA.svg", "Banana production", "1961", "BRA", true},
		{"hyphenated old name", "File:Number-of-deaths-from-tetanus, 1980 to 2021, IND.svg", "Number of deaths from tetanus", "1980", "IND", true},
		{"case differs", "File:banana Production, 1961 to 2023, bra.svg", "Banana production", "1961", "BRA", true},
		{"date range with commas", "File:Biweekly covid deaths per million people, Jan 16, 2020 to Feb 22, 2026, USA.svg", "Biweekly covid deaths per million people", "Jan 16, 2020", "USA", true},
		{"BCE range", "File:Long term urban population region, 10,000 BCE to 2023, TTO.svg", "Long term urban population region", "10,000 BCE", "TTO", true},
		{"OWID region code", "File:Share plastic waste imports, 1988 to 2024, OWID NAM.svg", "Share plastic waste imports", "1988", "OWID_NAM", true},
		{"Namibia is not North America", "File:Wheat production, 1961 to 2023, NAM.svg", "Wheat production", "1961", "OWID_NAM", false},
		{"North America is not Namibia", "File:Share plastic waste imports, 1988 to 2024, OWID NAM.svg", "Share plastic waste imports", "1988", "NAM", false},
		{"regex characters in name", "File:Share (%) of GDP+ (2011.ppp), 1990 to 2020, FRA.svg", "Share (%) of GDP+ (2011.ppp)", "1990", "FRA", true},

		// Past bad moves that must be refused
		{"world year map, not a country file", "File:Banana production, World, 1961.svg", "Banana production", "1961", "ARE", false},
		{"region year map", "File:Urban share european commission, Asia, 1950.svg", "Urban share european commission", "1950", "ARE", false},
		{"longer chart name (prefix)", "File:Electric car sales share, 2010 to 2024, ISR.svg", "Electric car sales", "2010", "ISR", false},
		{"longer chart name in middle", "File:Poverty share on less than 30 per day 2011 ppp, 1963 to 2023, SYR.svg", "Poverty share on less than 30 per day", "1963", "SYR", false},
		{"other country", "File:Women civil liberties index, 1789 to 2024, DEU.svg", "Women civil liberties index", "1789", "ARE", false},
		{"country code is suffix of another", "File:Tungsten production, 1990 to 2020, SARE.svg", "Tungsten production", "1990", "ARE", false},
		{"different start year", "File:Banana production, 1962 to 2023, BRA.svg", "Banana production", "1961", "BRA", false},
		{"start year only a prefix", "File:Banana production, 19610 to 2023, BRA.svg", "Banana production", "1961", "BRA", false},
		{"already new format", "File:Banana production, BRA.svg", "Banana production", "1961", "BRA", false},
		{"non OWID-style file", "File:Tuberculosis world map - DALY - WHO2004.svg", "Tuberculosis death rate", "2000", "PER", false},
		{"empty inputs", "File:Banana production, 1961 to 2023, BRA.svg", "", "1961", "BRA", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isOldFormatTitleFor(c.title, c.chart, c.start, c.region); got != c.want {
				t.Fatalf("isOldFormatTitleFor(%q, %q, %q, %q) = %v, want %v", c.title, c.chart, c.start, c.region, got, c.want)
			}
		})
	}
}

func TestPageIsForChart(t *testing.T) {
	page := func(source string) string {
		return "=={{int:filedesc}}==\n{{Information\n|description={{en|1=X}}\n|source = " + source + "\n}}\n[[Category:Uploaded by OWID importer tool]]"
	}
	cases := []struct {
		name, pageText, chartURL string
		want                     bool
	}{
		{"same grapher", page("https://ourworldindata.org/grapher/banana-production"), "https://ourworldindata.org/grapher/banana-production", true},
		{"trailing question mark", page("https://ourworldindata.org/grapher/urban-share-european-commission?"), "https://ourworldindata.org/grapher/urban-share-european-commission", true},
		{"view params ignored", page("https://ourworldindata.org/grapher/electric-car-sales-share?tab=map&time=2020"), "https://ourworldindata.org/grapher/electric-car-sales-share", true},
		{"trailing slash and case", page("https://OurWorldInData.org/grapher/Banana-Production/"), "https://ourworldindata.org/grapher/banana-production", true},
		{"www host", page("https://www.ourworldindata.org/grapher/banana-production"), "https://ourworldindata.org/grapher/banana-production", true},
		{"url in summary text", "Importing from https://ourworldindata.org/grapher/banana-production with metadata", "https://ourworldindata.org/grapher/banana-production", true},
		{"explorer params reordered", page("https://ourworldindata.org/explorers/minerals?Metric=Production&Mineral=Tungsten&Type=Mine&Share+of+global=false"), "https://ourworldindata.org/explorers/minerals?Mineral=Tungsten&Metric=Production&Type=Mine&Share+of+global=false", true},
		{"explorer encoded soft hyphens", page("https://ourworldindata.org/explorers/democracy?Dataset=Polity&Metric=%C2%AD%C2%AD%C2%ADDemocracy&Sub-metric=Regulation+of+political+participation+score"), "https://ourworldindata.org/explorers/democracy?Dataset=Polity&Metric=%C2%AD%C2%AD%C2%ADDemocracy&Sub-metric=Regulation+of+political+participation+score", true},

		{"slug is prefix of page slug", page("https://ourworldindata.org/grapher/electric-car-sales-share"), "https://ourworldindata.org/grapher/electric-car-sales", false},
		{"page slug is prefix of slug", page("https://ourworldindata.org/grapher/electric-car-sales"), "https://ourworldindata.org/grapher/electric-car-sales-share", false},
		{"2011 ppp variant", page("https://ourworldindata.org/grapher/poverty-share-on-less-than-30-per-day-2011-ppp"), "https://ourworldindata.org/grapher/poverty-share-on-less-than-30-per-day", false},
		{"same explorer, other metric", page("https://ourworldindata.org/explorers/democracy?Dataset=Polity&Metric=Democracy"), "https://ourworldindata.org/explorers/democracy?Dataset=Regimes+of+the+World&Metric=Electoral+democracy", false},
		{"explorer missing params", page("https://ourworldindata.org/explorers/democracy"), "https://ourworldindata.org/explorers/democracy?Dataset=Polity", false},
		{"no owid url", page("WHO 2004"), "https://ourworldindata.org/grapher/tuberculosis-death-rate", false},
		{"empty chart url", page("https://ourworldindata.org/grapher/banana-production"), "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := pageIsForChart(c.pageText, c.chartURL); got != c.want {
				t.Fatalf("pageIsForChart(..%q.., %q) = %v, want %v", c.pageText, c.chartURL, got, c.want)
			}
		})
	}
}
