package owidparser

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
)

type Data struct {
	Values   []float64 `json:"values"`
	Years    []int     `json:"years"`
	Entities []int     `json:"entities"`
}

type Entity struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Code string `json:"code"`
}

type Dimensions struct {
	Entities struct {
		Values []Entity `json:"values"`
	} `json:"entities"`
}

type Metadata struct {
	Name       string     `json:"name"`
	Unit       string     `json:"unit"`
	Timespan   string     `json:"timespan"`
	Dimensions Dimensions `json:"dimensions"`
}

type CombinedDataPoint struct {
	Value       float64 `json:"value"`
	Year        int     `json:"year"`
	EntityID    int     `json:"entityId"`
	EntityName  string  `json:"entityName"`
	CountryCode string  `json:"countryCode"`
}

// Node represents either text content or a child element
type Node struct {
	IsText  bool
	Text    string
	Element *GenericElement
}

type GenericSVG struct {
	XMLName    xml.Name `xml:"svg"`
	Attributes map[string]string
	Children   []Node
}

type GenericElement struct {
	XMLName    xml.Name
	Attributes map[string]string
	Children   []Node
}

// Custom MarshalXML for GenericSVG
func (g *GenericSVG) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	// Set the tag name
	start.Name = g.XMLName

	// Sort attribute keys for consistent ordering
	keys := make([]string, 0, len(g.Attributes))
	for key := range g.Attributes {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	// Add attributes in sorted order
	for _, key := range keys {
		start.Attr = append(start.Attr, xml.Attr{Name: xml.Name{Local: key}, Value: g.Attributes[key]})
	}

	// Start the element
	if err := e.EncodeToken(start); err != nil {
		return err
	}

	// Encode children in order
	for _, child := range g.Children {
		if child.IsText {
			if err := e.EncodeToken(xml.CharData(child.Text)); err != nil {
				return err
			}
		} else if child.Element != nil {
			if err := e.Encode(child.Element); err != nil {
				return err
			}
		}
	}

	// End the element
	return e.EncodeToken(xml.EndElement{Name: start.Name})
}

// Custom MarshalXML for GenericElement
func (g *GenericElement) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	// Set the tag name
	start.Name = g.XMLName

	// Sort attribute keys for consistent ordering
	keys := make([]string, 0, len(g.Attributes))
	for key := range g.Attributes {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	// Add attributes in sorted order
	for _, key := range keys {
		start.Attr = append(start.Attr, xml.Attr{Name: xml.Name{Local: key}, Value: g.Attributes[key]})
	}

	// Start the element
	if err := e.EncodeToken(start); err != nil {
		return err
	}

	// Encode children in order
	for _, child := range g.Children {
		if child.IsText {
			if err := e.EncodeToken(xml.CharData(child.Text)); err != nil {
				return err
			}
		} else if child.Element != nil {
			if err := e.Encode(child.Element); err != nil {
				return err
			}
		}
	}

	// End the element
	return e.EncodeToken(xml.EndElement{Name: start.Name})
}

func (g *GenericSVG) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	g.XMLName = start.Name
	g.Attributes = make(map[string]string)

	// Process attributes
	for _, attr := range start.Attr {
		g.Attributes[attr.Name.Local] = attr.Value
	}

	// Process children
	for {
		token, err := d.Token()
		if err != nil {
			return err
		}

		switch t := token.(type) {
		case xml.StartElement:
			var elem GenericElement
			if err := elem.UnmarshalXML(d, t); err != nil {
				return err
			}
			g.Children = append(g.Children, Node{
				IsText:  false,
				Element: &elem,
			})

		case xml.EndElement:
			return nil

		case xml.CharData:
			text := string(t)
			// Only add non-empty text nodes, but preserve whitespace
			if text != "" {
				g.Children = append(g.Children, Node{
					IsText: true,
					Text:   text,
				})
			}
		}
	}
}

func (g *GenericElement) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	g.XMLName = start.Name
	g.Attributes = make(map[string]string)

	// Process attributes
	for _, attr := range start.Attr {
		g.Attributes[attr.Name.Local] = attr.Value
	}

	// Process children
	for {
		token, err := d.Token()
		if err != nil {
			return err
		}

		switch t := token.(type) {
		case xml.StartElement:
			var elem GenericElement
			if err := elem.UnmarshalXML(d, t); err != nil {
				return err
			}
			g.Children = append(g.Children, Node{
				IsText:  false,
				Element: &elem,
			})

		case xml.EndElement:
			return nil

		case xml.CharData:
			text := string(t)
			// Only add non-empty text nodes, but preserve whitespace
			if text != "" {
				g.Children = append(g.Children, Node{
					IsText: true,
					Text:   text,
				})
			}
		}
	}
}

// Helper methods for GenericSVG
func (g *GenericSVG) FindElements(tagName string) []*GenericElement {
	var results []*GenericElement
	g.findElementsRecursive(g.Children, tagName, &results)
	return results
}

func (g *GenericSVG) findElementsRecursive(children []Node, tagName string, results *[]*GenericElement) {
	for _, child := range children {
		if !child.IsText && child.Element != nil {
			if child.Element.XMLName.Local == tagName {
				*results = append(*results, child.Element)
			}
			g.findElementsRecursive(child.Element.Children, tagName, results)
		}
	}
}

func (g *GenericSVG) FindElementsByAttribute(attrName, attrValue string) []*GenericElement {
	var results []*GenericElement
	g.findElementsByAttributeRecursive(g.Children, attrName, attrValue, &results)
	return results
}

func (g *GenericSVG) findElementsByAttributeRecursive(children []Node, attrName, attrValue string, results *[]*GenericElement) {
	for _, child := range children {
		if !child.IsText && child.Element != nil {
			if val, exists := child.Element.Attributes[attrName]; exists && val == attrValue {
				*results = append(*results, child.Element)
			}
			g.findElementsByAttributeRecursive(child.Element.Children, attrName, attrValue, results)
		}
	}
}

// Helper methods for GenericElement
func (g *GenericElement) FindElements(tagName string) []*GenericElement {
	var results []*GenericElement
	g.findElementsRecursive(g.Children, tagName, &results)
	return results
}

func (g *GenericElement) findElementsRecursive(children []Node, tagName string, results *[]*GenericElement) {
	for _, child := range children {
		if !child.IsText && child.Element != nil {
			if child.Element.XMLName.Local == tagName {
				*results = append(*results, child.Element)
			}
			g.findElementsRecursive(child.Element.Children, tagName, results)
		}
	}
}

func (g *GenericElement) FindElementsByAttribute(attrName, attrValue string) []*GenericElement {
	var results []*GenericElement
	g.findElementsByAttributeRecursive(g.Children, attrName, attrValue, &results)
	return results
}

func (g *GenericElement) findElementsByAttributeRecursive(children []Node, attrName, attrValue string, results *[]*GenericElement) {
	for _, child := range children {
		if !child.IsText && child.Element != nil {
			if val, exists := child.Element.Attributes[attrName]; exists && val == attrValue {
				*results = append(*results, child.Element)
			}
			g.findElementsByAttributeRecursive(child.Element.Children, attrName, attrValue, results)
		}
	}
}

// Helper method to get all elements from Children
func (g *GenericElement) GetElements() []*GenericElement {
	var elements []*GenericElement
	for _, child := range g.Children {
		if !child.IsText && child.Element != nil {
			elements = append(elements, child.Element)
		}
	}
	return elements
}

// Helper method to get text content
func (g *GenericElement) GetTextContent() string {
	var text strings.Builder
	for _, child := range g.Children {
		if child.IsText {
			text.WriteString(child.Text)
		}
	}
	return text.String()
}

// Helper method to set text content (replaces all children with single text node)
func (g *GenericElement) SetTextContent(content string) {
	g.Children = []Node{
		{
			IsText: true,
			Text:   content,
		},
	}
}

// Helper method to append element
func (g *GenericElement) AppendElement(elem GenericElement) {
	g.Children = append(g.Children, Node{
		IsText:  false,
		Element: &elem,
	})
}

// Helper method to append text
func (g *GenericElement) AppendText(text string) {
	g.Children = append(g.Children, Node{
		IsText: true,
		Text:   text,
	})
}

type SVGQuery struct {
	root *GenericSVG
}

func NewSVGQuery(svg *GenericSVG) *SVGQuery {
	return &SVGQuery{root: svg}
}

func (q *SVGQuery) Select(selector string) []*GenericElement {
	// Simple selector implementation
	if strings.HasPrefix(selector, "#") {
		// ID selector
		id := strings.TrimPrefix(selector, "#")
		return q.root.FindElementsByAttribute("id", id)
	} else if strings.HasPrefix(selector, ".") {
		// Class selector
		class := strings.TrimPrefix(selector, ".")
		return q.root.FindElementsByAttribute("class", class)
	} else {
		// Tag selector
		return q.root.FindElements(selector)
	}
}

func (q *SVGQuery) SelectAll(selector string) []*GenericElement {
	return q.Select(selector)
}

func (q *SVGQuery) Filter(elements []*GenericElement, predicate func(*GenericElement) bool) []*GenericElement {
	var results []*GenericElement
	for _, elem := range elements {
		if predicate(elem) {
			results = append(results, elem)
		}
	}
	return results
}

type FillItem struct {
	Value    float64
	StrValue string
	Fill     string
}

type WriteResult struct {
	Path string
	Year int
}

// Country name mapping function - you'll need to expand this
func getCountryID(entityName string) string {
	return strings.ReplaceAll(entityName, " ", "-")
}

func GenerateImages(title, dataPath, metadataPath, mapPath, outPath string) (*[]WriteResult, error) {
	dataBytes, err := os.ReadFile(dataPath)
	if err != nil {
		return nil, err
	}

	var data Data
	if err := json.Unmarshal(dataBytes, &data); err != nil {
		return nil, err
	}

	// Read and parse data.metadata.json
	metadataBytes, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, err
	}

	var metadata Metadata
	if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
		return nil, err
	}

	// Create entity lookup map
	entityLookup := make(map[int]Entity)
	for _, entity := range metadata.Dimensions.Entities.Values {
		entityLookup[entity.ID] = entity
	}

	// Combine data into structured format
	var combinedData []CombinedDataPoint
	yearlyData := make(map[int][]CombinedDataPoint)

	for i, value := range data.Values {
		entityID := data.Entities[i]
		entity, exists := entityLookup[entityID]

		var entityName, countryCode string
		if exists {
			entityName = entity.Name
			countryCode = entity.Code
		} else {
			entityName = "Unknown"
			countryCode = ""
		}

		combinedData = append(combinedData, CombinedDataPoint{
			Value:       value,
			Year:        data.Years[i],
			EntityID:    entityID,
			EntityName:  entityName,
			CountryCode: countryCode,
		})
		yearlyData[data.Years[i]] = append(yearlyData[data.Years[i]], CombinedDataPoint{
			Value:       value,
			Year:        data.Years[i],
			EntityID:    entityID,
			EntityName:  entityName,
			CountryCode: countryCode,
		})
	}

	// Print the combined data (or process as needed)
	for i, point := range combinedData {
		fmt.Printf("Data point %d: %.2f%% in %d for %s (%s)\n",
			i+1, point.Value, point.Year, point.EntityName, point.CountryCode)

		// Print only first 10 to avoid overwhelming output
		if i >= 9 {
			fmt.Printf("... and %d more data points\n", len(combinedData)-10)
			break
		}
	}

	var genericSVG GenericSVG
	svgData, err := os.ReadFile(mapPath)
	if err != nil {
		return nil, err
	}

	// Clean up the SVG content to remove duplicate xmlns declarations
	svgContent := string(svgData)

	// Remove duplicate xmlns declarations but keep the first one
	lines := strings.Split(svgContent, "\n")
	var cleanedLines []string
	seenXmlns := false

	for _, line := range lines {
		// If this line contains xmlns="http://www.w3.org/2000/svg" and we've seen it before, skip it
		if strings.Contains(line, `xmlns="http://www.w3.org/2000/svg"`) {
			if seenXmlns && !strings.Contains(line, "<svg") {
				// Skip this line as it's a duplicate xmlns
				continue
			}
			seenXmlns = true
		}
		cleanedLines = append(cleanedLines, line)
	}

	cleanedSVG := strings.Join(cleanedLines, "\n")

	if err := xml.Unmarshal([]byte(cleanedSVG), &genericSVG); err != nil {
		log.Printf("Generic struct parser error: %v", err)
		return nil, err
	}

	paths := genericSVG.FindElements("path")
	fmt.Printf("Found %d path elements\n", len(paths))

	query := NewSVGQuery(&genericSVG)
	pathElements := query.Select("path")
	fmt.Printf("Query found %d path elements\n", len(pathElements))

	lines2 := query.Select("#lines")
	swatches := query.Select("#swatches")

	if len(lines2) == 0 || len(swatches) == 0 {
		return nil, fmt.Errorf("Could not find legend lines or swatches in SVG")
	}

	fillMap := make([]FillItem, 0)

	linesElements := lines2[0].GetElements()
	swatchesElements := swatches[0].GetElements()

	for index, item := range linesElements {
		if index >= len(swatchesElements) {
			break
		}

		key := item.Attributes["id"]
		if metadata.Unit != "" {
			key = strings.ReplaceAll(key, metadata.Unit, "")
		}

		floatKey, err := strconv.ParseFloat(key, 64)
		if err != nil {
			fmt.Println("Adding non-numeric key:", key)
			fillMap = append(fillMap, FillItem{
				StrValue: key,
				Fill:     swatchesElements[index].Attributes["fill"],
			})
		} else {
			fillMap = append(fillMap, FillItem{
				Value: floatKey,
				Fill:  swatchesElements[index].Attributes["fill"],
			})
		}
	}

	sort.SliceStable(fillMap, func(a int, b int) bool {
		return fillMap[a].Value < fillMap[b].Value
	})

	if len(fillMap) < 2 {
		return nil, fmt.Errorf("Error finding fill map")
	}

	fmt.Println("Fill map: ", fillMap)

	// Create yearly_maps directory if it doesn't exist
	err = os.WriteFile(fmt.Sprintf("%s/.gitkeep", outPath), []byte(""), 0755)
	if err != nil {
		log.Printf("Could not create yearly_maps directory: %v", err)
		return nil, err
	}

	writeResults := make([]WriteResult, 0)
	CleanupTextElementsPreserveStructure(&genericSVG)

	for year, yearData := range yearlyData {
		fmt.Printf("Processing year: %d with %d data points\n", year, len(yearData))

		// Create a copy of the SVG for this year
		yearSVG := genericSVG
		yearQuery := NewSVGQuery(&yearSVG)
		yearPathElements := yearQuery.Select("path")

		titleLink := yearQuery.Select("#title")

		// Update title in the svg
		if len(titleLink) > 0 {
			titleElements := titleLink[0].GetElements()
			if len(titleElements) > 0 {
				titleSubElements := titleElements[0].GetElements()
				if len(titleSubElements) > 0 {
					titleSubElements[0].SetTextContent(fmt.Sprintf("%s, %v", title, year))
				} else {
					titleElements[0].SetTextContent(fmt.Sprintf("%s, %v", title, year))
				}
			}
		}

		matchedCount := 0

		// Generate image for this year by replacing the colors of the data
		for _, item := range yearData {
			fillValue := getFillValue(fillMap, item.Value)
			countryID := getCountryID(item.EntityName)

			// Find the path element for this country
			for i := range yearPathElements {
				if yearPathElements[i].Attributes["id"] == countryID {
					yearPathElements[i].Attributes["fill"] = fillValue
					matchedCount++
					break
				}
			}
		}

		// Write the file
		dirpath := path.Join(outPath, strconv.Itoa(year))
		err := os.MkdirAll(dirpath, 0755)
		if err != nil {
			fmt.Println("Error creating year directory", year, err)
			continue
		}

		fmt.Println("CREATING DIRECTORY: ", dirpath, err)
		filename := path.Join(outPath, strconv.Itoa(year), fmt.Sprintf("%d.svg", year))
		err = WriteSVGFile(&yearSVG, filename)
		if err != nil {
			log.Printf("Error writing file %s: %v", filename, err)
		} else {
			fmt.Printf("Successfully wrote %s\n", filename)
			writeResults = append(writeResults, WriteResult{
				Path: filename,
				Year: year,
			})
		}
	}

	return &writeResults, nil
}

func RemoveNestedTSpans(svg *GenericSVG) {
	// This function can be implemented if needed
}

func WriteSVGFile(yearSVG *GenericSVG, filename string) error {
	// Marshal the SVG to XML with proper formatting
	output := &bytes.Buffer{}
	encoder := xml.NewEncoder(output)
	encoder.Indent("", "  ")

	// Encode the SVG
	err := encoder.Encode(yearSVG)
	if err != nil {
		return err
	}

	err = encoder.Flush()
	if err != nil {
		return err
	}

	svgData := output.Bytes()

	// For some reason, xmlns is attached to every element
	// So we need to remove it manually and add it
	// only to <svg /> elements
	svgString := strings.ReplaceAll(string(svgData), ` xmlns="http://www.w3.org/2000/svg"`, "")
	svgString = strings.ReplaceAll(svgString, "<svg", `<svg xmlns="http://www.w3.org/2000/svg"`)
	svgData = []byte(svgString)

	// Write the file
	err = os.WriteFile(filename, svgData, 0744)
	if err != nil {
		log.Printf("Error writing file %s: %v", filename, err)
		return err
	}

	return nil
}

func getFillValue(fillMap []FillItem, value float64) string {
	// Handle special cases first
	if value < 0 || value != value { // NaN check
		for _, item := range fillMap {
			if item.StrValue == "No-data" {
				return item.Fill
			}
		}
		return ""
	}

	// Sort by value in descending order for proper threshold matching
	sortedMap := make([]FillItem, 0)
	for _, item := range fillMap {
		if item.StrValue == "" { // Only numeric items
			sortedMap = append(sortedMap, item)
		}
	}

	sort.SliceStable(sortedMap, func(a, b int) bool {
		return sortedMap[a].Value > sortedMap[b].Value
	})

	for _, item := range sortedMap {
		if value >= item.Value {
			return item.Fill
		}
	}

	// Return the lowest threshold color if no match found
	if len(sortedMap) > 0 {
		return sortedMap[len(sortedMap)-1].Fill
	}

	return ""
}

// CleanupTextElements removes nested elements (like <a> tags) from <text> elements
// and ensures each text element contains only simple text or a single tspan with text
func CleanupTextElements(svg *GenericSVG) {
	textElements := svg.FindElements("text")

	for _, textElement := range textElements {
		cleanupTextElement(textElement)
	}
}

func cleanupTextElement(textElement *GenericElement) {
	// Extract all text content from the element and its children
	textContent := extractAllTextContent(textElement)

	// If we have text content, replace the entire structure with simple text
	if strings.TrimSpace(textContent) != "" {
		// Check if the original element had a single tspan - if so, preserve that structure
		tspans := textElement.FindElements("tspan")

		if len(tspans) == 1 {
			// Keep single tspan structure but clean its content
			tspan := tspans[0]
			tspan.SetTextContent(strings.TrimSpace(textContent))

			// Replace text element's children with just the cleaned tspan
			textElement.Children = []Node{
				{
					IsText:  false,
					Element: tspan,
				},
			}
		} else {
			// Replace with direct text content
			textElement.SetTextContent(strings.TrimSpace(textContent))
		}
	}
}

// extractAllTextContent recursively extracts all text content from an element and its children
func extractAllTextContent(element *GenericElement) string {
	var textBuilder strings.Builder

	for _, child := range element.Children {
		if child.IsText {
			textBuilder.WriteString(child.Text)
		} else if child.Element != nil {
			// Recursively extract text from child elements
			childText := extractAllTextContent(child.Element)
			textBuilder.WriteString(childText)
		}
	}

	return textBuilder.String()
}

// Alternative version that preserves more structure if needed
func CleanupTextElementsPreserveStructure(svg *GenericSVG) {
	textElements := svg.FindElements("text")

	for _, textElement := range textElements {
		cleanupTextElementPreserveStructure(textElement)
	}
}

func cleanupTextElementPreserveStructure(textElement *GenericElement) {
	var newChildren []Node

	for _, child := range textElement.Children {
		if child.IsText {
			// Keep text nodes as-is
			newChildren = append(newChildren, child)
		} else if child.Element != nil {
			// Process child elements
			if child.Element.XMLName.Local == "tspan" {
				// Clean the tspan and keep it
				cleanedTspan := cleanTspan(child.Element)
				newChildren = append(newChildren, Node{
					IsText:  false,
					Element: cleanedTspan,
				})
			}
			// Skip other elements like <a> tags
		}
	}

	textElement.Children = newChildren
}

func cleanTspan(tspan *GenericElement) *GenericElement {
	// Extract all text content from the tspan
	textContent := extractAllTextContent(tspan)

	// Create a new tspan with cleaned content
	cleanedTspan := &GenericElement{
		XMLName:    tspan.XMLName,
		Attributes: make(map[string]string),
	}

	// Copy attributes
	for k, v := range tspan.Attributes {
		cleanedTspan.Attributes[k] = v
	}

	// Set only the text content
	cleanedTspan.SetTextContent(strings.TrimSpace(textContent))

	return cleanedTspan
}
