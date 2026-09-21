package services

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	svgprocessor "github.com/wpmed-videowiki/OWIDImporter/svg_processor"
	"github.com/wpmed-videowiki/OWIDImporter/svgtranslations"
)

var (
	metadataBlockRe = regexp.MustCompile(`(?s)<metadata[^>]*>.*?</metadata>`)
	whitespaceRe    = regexp.MustCompile(`\s+`)
	pathDataRe      = regexp.MustCompile(`\sd="([^"]*)"`)
	defsBlockRe     = regexp.MustCompile(`(?s)<defs\b[^>]*>.*?</defs>`)
)

// pathGeometry collects every d="..." path attribute value in document
// order, so svgVisiblySame can detect a changed plotted line/area on charts
// where ExtractCountryFillsFromBytes has nothing to compare (line charts,
// single images that are not maps). Pattern/hatch definitions inside <defs>
// are ignored because OWID's exporter adds and removes them independently of
// the data (e.g. a "no data"/"inapplicable" hatch pattern), so they would
// otherwise cause a false "data changed" result.
func pathGeometry(svg []byte) []string {
	var out []string
	for _, m := range pathDataRe.FindAllSubmatch(defsBlockRe.ReplaceAll(svg, nil), -1) {
		out = append(out, normalizeWhitespace(string(m[1])))
	}
	return out
}

// svgVisiblySame reports whether two map/chart SVGs show the same thing:
// same untranslated text, same plotted path geometry, same country fills and
// same injected <metadata>. It ignores svgtranslate <switch> variants so that
// a translated Commons file compares equal to a fresh OWID download of
// unchanged data.
func svgVisiblySame(existing, incoming []byte) bool {
	if !reflect.DeepEqual(svgtranslations.DefaultTexts(existing), svgtranslations.DefaultTexts(incoming)) {
		return false
	}
	if normalizeWhitespace(metadataBlockRe.FindString(string(existing))) != normalizeWhitespace(metadataBlockRe.FindString(string(incoming))) {
		return false
	}
	if !reflect.DeepEqual(pathGeometry(existing), pathGeometry(incoming)) {
		return false
	}
	existingFills, err := svgprocessor.ExtractCountryFillsFromBytes(existing)
	if err != nil {
		return false
	}
	incomingFills, err := svgprocessor.ExtractCountryFillsFromBytes(incoming)
	if err != nil {
		return false
	}
	return reflect.DeepEqual(existingFills, incomingFills)
}

func normalizeWhitespace(s string) string {
	return strings.TrimSpace(whitespaceRe.ReplaceAllString(s, " "))
}

// preserveExistingTranslations re-applies the svgtranslate translations of the
// file already on Commons (fetched from existingURL) to the freshly
// downloaded file in downloadPath. It returns the (possibly rewritten) file
// info and unchanged=true when the Commons file must be left as it is
// (nothing visible changed, or the translations could not be carried over).
func preserveExistingTranslations(filename, existingURL, downloadPath string, fileInfo *FileInfo) (*FileInfo, bool, error) {
	label := filename
	existingDir := downloadPath + "_existing"
	if err := os.MkdirAll(existingDir, 0755); err != nil {
		return fileInfo, false, err
	}
	existingPath := filepath.Join(existingDir, "image.svg")
	if err := downloadCommonsFileFromURL(existingURL, existingPath); err != nil {
		return fileInfo, false, err
	}
	existing, err := os.ReadFile(existingPath)
	if err != nil {
		return fileInfo, false, err
	}
	if !svgtranslations.HasSwitch(existing) {
		return fileInfo, false, nil
	}
	if svgVisiblySame(existing, fileInfo.File) {
		fmt.Printf("Translations: %s unchanged apart from translations, keeping Commons version\n", label)
		return fileInfo, true, nil
	}
	if svgtranslations.HasSwitch(fileInfo.File) {
		fmt.Printf("Translations: %s incoming file already carries translations\n", label)
		return fileInfo, false, nil
	}
	merged, result := svgtranslations.Merge(existing, fileInfo.File)
	if result.Merged < result.Available {
		fmt.Printf("Translations: %s could carry over only %d of %d translated texts, refusing to overwrite so none are lost\n", label, result.Merged, result.Available)
		return fileInfo, true, nil
	}
	if svgtranslations.HasSwitch(existing) && !svgtranslations.HasSwitch(merged) {
		fmt.Printf("Translations: %s existing <switch> markup could not be carried over, refusing to overwrite\n", label)
		return fileInfo, true, nil
	}
	fmt.Printf("Translations: %s carried over %d of %d translated texts\n", label, result.Merged, result.Available)
	if err := xml.Unmarshal(merged, new(struct{})); err != nil {
		fmt.Printf("Translations: %s merged file is not well-formed (%v), refusing to overwrite\n", label, err)
		return fileInfo, true, nil
	}
	if err := os.WriteFile(fileInfo.FilePath, merged, 0644); err != nil {
		return fileInfo, false, err
	}
	newInfo, err := getFileInfo(downloadPath)
	if err != nil {
		return fileInfo, false, err
	}
	return newInfo, false, nil
}
