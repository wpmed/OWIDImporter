package svgprocessor

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"strings"
)

// SVG structure to parse the XML
type SVG struct {
	XMLName xml.Name `xml:"svg"`
	Groups  []Group  `xml:"g"`
}

type Group struct {
	ID    string  `xml:"id,attr"`
	Group []Group `xml:"g"`
	Path  []Path  `xml:"path"`
}

type Path struct {
	ID   string `xml:"id,attr"`
	Fill string `xml:"fill,attr"`
}

// CountryFill represents a country and its fill color
type CountryFill struct {
	Country string `json:"country"`
	Fill    string `json:"fill"`
}

// ExtractCountryFills reads an SVG file and extracts country names and their fill colors
func ExtractCountryFills(filepath string) ([]CountryFill, error) {
	// Read the SVG file
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	// Parse the XML
	var svg SVG
	err = xml.Unmarshal(data, &svg)
	if err != nil {
		return nil, fmt.Errorf("error parsing XML: %w", err)
	}

	// Extract countries and fills
	var results []CountryFill

	// Recursive function to find paths in groups
	var findPaths func(groups []Group)
	findPaths = func(groups []Group) {
		for _, group := range groups {
			// Check if this is the countries-with-data group
			if group.ID == "countries-with-data" || group.ID == "countries-without-data" {
				for _, path := range group.Path {
					country := cleanCountryName(path.ID)
					results = append(results, CountryFill{
						Country: country,
						Fill:    path.Fill,
					})
				}
			}
			// Recursively check nested groups
			findPaths(group.Group)
		}
	}

	findPaths(svg.Groups)

	return results, nil
}

// cleanCountryName replaces hyphens with spaces and handles special HTML entities
func cleanCountryName(name string) string {
	// Replace hyphens with spaces
	cleaned := strings.ReplaceAll(name, "-", " ")

	// Handle HTML entities
	cleaned = strings.ReplaceAll(cleaned, "&#x27;", "'")

	return cleaned
}

// ConvertToJSON converts a slice of CountryFill to a JSON string
func ConvertToJSON(countries []CountryFill) (string, error) {
	jsonData, err := json.Marshal(countries)
	if err != nil {
		return "", fmt.Errorf("error marshaling to JSON: %w", err)
	}
	return string(jsonData), nil
}

// ParseJSONString converts a JSON string back to []CountryFill
func ParseJSONString(jsonStr string) ([]CountryFill, error) {
	var countries []CountryFill
	err := json.Unmarshal([]byte(jsonStr), &countries)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling JSON: %w", err)
	}
	return countries, nil
}
