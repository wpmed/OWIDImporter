package services

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
	"github.com/wpmed-videowiki/OWIDImporter/constants"
	"github.com/wpmed-videowiki/OWIDImporter/env"
	"github.com/wpmed-videowiki/OWIDImporter/models"
	"github.com/wpmed-videowiki/OWIDImporter/utils"
)

const (
	iframeModalTimeout    = 15 * time.Second
	iframeButtonTimeout   = 30 * time.Second
	iframeDownloadTimeout = 90 * time.Second
	// How long a checkbox toggle may take to disable the buttons
	iframeRegenStartTimeout = 1500 * time.Millisecond
)

// iframePageLoadTimeout bounds navigation, load and idle of the chart page together
// (a var so tests can shorten it).
var iframePageLoadTimeout = 60 * time.Second

// iframeInfoDownloadTimeout bounds the lookup's name-only SVG download, so a slow OWID export
// cannot hold /iframe/info (a var so tests can shorten it). The task's own download is not affected.
var iframeInfoDownloadTimeout = 25 * time.Second

// iframeDetailsTimeout bounds reading the title and source from a loaded page together,
// so a renderer that hangs after load cannot stall /iframe/info or a task.
var iframeDetailsTimeout = 15 * time.Second

// IframeInfo is the read-only lookup result for an iframe import.
type IframeInfo struct {
	Path      string `json:"path"`
	ChartName string `json:"chartName"`
	Title     string `json:"title"`
	Source    string `json:"source"`
	// OWID's SVG download name (e.g. mica-production.svg), the value of $NAME; empty when the
	// lookup could neither read the displaySlug nor download the SVG
	DownloadName string `json:"downloadName"`
	// The view the chart shows, e.g. "Map" or "Line" (the value of $TAB); empty when unknown
	Tab string `json:"tab"`
	// Where DownloadName came from: iframeNameFromSlug or iframeNameFromDownload ("" when unknown)
	downloadNameSource string
}

const (
	iframeNameFromSlug     = "displaySlug"
	iframeNameFromDownload = "download"
)

// iframeSlugTimeout bounds reading the chart's displaySlug (a var so tests can shorten it).
var iframeSlugTimeout = 5 * time.Second

// OWID's DownloadModal names the SVG manager.displaySlug + ".svg", where displaySlug is the
// chart's slug or slugify(title). window.grapher is whichever Grapher mounted last (on data
// pages often a related chart), so it is not trusted on its own: grapher pages use the page's
// own chart config slug, explorer pages the explorer's grapher state (one per page).
// activeTab is the view the chart shows (GrapherTabName, e.g. "WorldMap", "LineChart", "Table").
// mainConfig is the page's own chart config fields that set its initial view (null when the
// page has no config); OWID omits fields that have their default value.
const iframeDisplaySlugJS = `() => {
	const read = f => { try { const v = f(); return typeof v === "string" ? v : ""; } catch (e) { return ""; } };
	const mainConfig = () => {
		try {
			const c = window._OWID_GRAPHER_CONFIG;
			if (!c || typeof c !== "object") return null;
			return {
				tab: typeof c.tab === "string" ? c.tab : "",
				hasMapTab: c.hasMapTab === true,
				chartTypes: Array.isArray(c.chartTypes) ? c.chartTypes.filter(t => typeof t === "string") : null,
			};
		} catch (e) { return null; }
	};
	return {
		config: read(() => window._OWID_GRAPHER_CONFIG.slug),
		grapher: read(() => window.grapher.grapherState.displaySlug),
		explorer: read(() => window.explorer.grapherState.displaySlug),
		grapherTab: read(() => window.grapher.grapherState.activeTab),
		explorerTab: read(() => window.explorer.grapherState.activeTab),
		mainConfig: mainConfig(),
	};
}`

// iframeSlugCandidates are the displaySlug and active tab values read from the chart page.
type iframeSlugCandidates struct {
	Config      string // window._OWID_GRAPHER_CONFIG.slug (grapher pages)
	Grapher     string // window.grapher.grapherState.displaySlug (last mounted Grapher)
	Explorer    string // window.explorer.grapherState.displaySlug (explorer pages)
	GrapherTab  string // window.grapher.grapherState.activeTab
	ExplorerTab string // window.explorer.grapherState.activeTab
	MainConfig  iframeMainConfig
}

// iframeMainConfig is the view part of window._OWID_GRAPHER_CONFIG, the grapher page's own
// chart (not a related chart).
type iframeMainConfig struct {
	Found      bool     // the page has a config object
	Tab        string   // config.tab; "" when not set (OWID's default "chart")
	HasMapTab  bool     // config.hasMapTab (default false)
	ChartTypes []string // config.chartTypes; nil when not set (OWID's default applies)
}

// iframeDefaultChartTypes is OWID's default config.chartTypes (grapher schema 012 and
// defaultGrapherConfig.ts: ["LineChart", "DiscreteBar"]).
var iframeDefaultChartTypes = []string{"LineChart", "DiscreteBar"}

// chartTypes are the config's chart types without duplicates, or OWID's default when unset.
// (OWID also drops invalid combinations; this keeps the given order.)
func (m iframeMainConfig) chartTypes() []string {
	if m.ChartTypes == nil {
		return iframeDefaultChartTypes
	}
	var types []string
	seen := map[string]bool{}
	for _, t := range m.ChartTypes {
		if !seen[t] {
			seen[t] = true
			types = append(types, t)
		}
	}
	return types
}

// defaultTabWord is the $TAB word of OWID's default tab: the first chart type, else the map
// when there is one, else the table (GrapherState.defaultTab).
func (m iframeMainConfig) defaultTabWord() string {
	if types := m.chartTypes(); len(types) > 0 {
		return iframeTabWord(types[0])
	}
	if m.HasMapTab {
		return "Map"
	}
	return "Table"
}

// tabWord is the $TAB word of the view OWID opens for a tab option (a ?tab= value or
// config.tab, e.g. "chart", "map", "line"), following GrapherState.mapTabConfigOptionToTabName:
// "chart" (or unset) is the default tab, "map" needs a map tab, a chart option needs that
// chart type, and anything else falls back to the default tab.
func (m iframeMainConfig) tabWord(option string) string {
	switch option {
	case "table":
		return "Table"
	case "map":
		if m.HasMapTab {
			return "Map"
		}
		return m.defaultTabWord()
	case "", "chart":
		return m.defaultTabWord()
	}
	if word := iframeTabWord(option); word != "" {
		for _, t := range m.chartTypes() {
			if iframeTabWord(t) == word {
				return word
			}
		}
	}
	return m.defaultTabWord()
}

// slugify output: ASCII letters, digits, "_" and "-"
var iframeDisplaySlugRegex = regexp.MustCompile(`^[\w-]+$`)

// readIframeDisplaySlug returns the candidates, all empty when they cannot be read in time.
func readIframeDisplaySlug(page *rod.Page) iframeSlugCandidates {
	timed := page.Timeout(iframeSlugTimeout)
	defer timed.CancelTimeout()
	res, err := timed.Eval(iframeDisplaySlugJS)
	if err != nil || res == nil {
		return iframeSlugCandidates{}
	}
	c := iframeSlugCandidates{
		Config:      res.Value.Get("config").Str(),
		Grapher:     res.Value.Get("grapher").Str(),
		Explorer:    res.Value.Get("explorer").Str(),
		GrapherTab:  res.Value.Get("grapherTab").Str(),
		ExplorerTab: res.Value.Get("explorerTab").Str(),
	}
	if cfg := res.Value.Get("mainConfig"); !cfg.Nil() {
		c.MainConfig = iframeMainConfig{Found: true, Tab: cfg.Get("tab").Str(), HasMapTab: cfg.Get("hasMapTab").Bool()}
		if types := cfg.Get("chartTypes"); !types.Nil() {
			c.MainConfig.ChartTypes = []string{}
			for _, t := range types.Arr() {
				c.MainConfig.ChartTypes = append(c.MainConfig.ChartTypes, t.Str())
			}
		}
	}
	return c
}

// pickIframeDisplaySlug is the main chart's displaySlug for path, or "" when it is not
// known for sure (the caller then downloads the SVG).
func pickIframeDisplaySlug(path string, c iframeSlugCandidates) string {
	slug := c.Config
	if strings.HasPrefix(path, "explorers/") {
		slug = c.Explorer
	}
	if !iframeDisplaySlugRegex.MatchString(slug) {
		return ""
	}
	return slug
}

// iframeTabWords are the $TAB words for OWID's tab query values (?tab=...); other values are
// title-cased.
var iframeTabWords = map[string]string{
	"map":          "Map",
	"table":        "Table",
	"line":         "Line",
	"discrete-bar": "Bar",
	"slope":        "Slope",
	"stacked-area": "Stacked-area",
	"scatter":      "Scatter",
	"marimekko":    "Marimekko",
	"stacked-bar":  "Stacked-bar",
}

// iframeTabNameAliases are the GrapherTabName forms (activeTab, kebab-cased) whose tab query
// value differs.
var iframeTabNameAliases = map[string]string{
	"world-map":    "map",
	"line-chart":   "line",
	"slope-chart":  "slope",
	"scatter-plot": "scatter",
}

var (
	iframeTabCamelBoundary = regexp.MustCompile(`([a-z0-9])([A-Z])`)
	iframeTabValue         = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)
)

// iframeTabWord is the $TAB word for a tab value: a page's activeTab ("WorldMap") or a ?tab=
// query value ("map"), e.g. "Map". It returns "" for an empty or unusable value.
func iframeTabWord(value string) string {
	v := strings.TrimSpace(value)
	v = strings.ToLower(iframeTabCamelBoundary.ReplaceAllString(v, "$1-$2"))
	v = strings.ReplaceAll(v, "_", "-")
	if alias, ok := iframeTabNameAliases[v]; ok {
		v = alias
	}
	if !iframeTabValue.MatchString(v) {
		return ""
	}
	if word, ok := iframeTabWords[v]; ok {
		return word
	}
	return strings.ToUpper(v[:1]) + v[1:]
}

// iframeURLTabValue is the raw ?tab= query value of path, or "".
func iframeURLTabValue(path string) string {
	parts := strings.SplitN(path, "?", 2)
	if len(parts) < 2 {
		return ""
	}
	query, err := url.ParseQuery(parts[1])
	if err != nil {
		return ""
	}
	return query.Get("tab")
}

// iframeURLTab is the $TAB word for the ?tab= query value of path, or "".
func iframeURLTab(path string) string {
	return iframeTabWord(iframeURLTabValue(path))
}

// pickIframeTab is the $TAB word for the view the main chart shows, or "" when unknown.
// Precedence, most direct evidence of what the reader sees first:
//  1. The page's live activeTab. On grapher pages window.grapher may be a related chart (data
//     pages), so its tab is used only when its displaySlug is the URL's chart slug.
//  2. Grapher pages with the page's own config (window._OWID_GRAPHER_CONFIG): the tab OWID
//     opens for the URL's ?tab= value, or for config.tab when the URL has none. OWID applies a
//     ?tab= over the config's default tab, so an explicit URL tab wins over config.tab; the
//     config is still needed to resolve "chart" (its first chart type) and options the
//     chart does not have (OWID falls back to the default tab).
//  3. Otherwise (explorers, or no config) the URL's ?tab= value on its own.
func pickIframeTab(path string, c iframeSlugCandidates) string {
	explorer := strings.HasPrefix(path, "explorers/")
	pageTab := c.ExplorerTab
	if !explorer {
		pageTab = ""
		if c.Grapher != "" && c.Grapher == utils.IframeChartSlug(path) {
			pageTab = c.GrapherTab
		}
	}
	if word := iframeTabWord(pageTab); word != "" {
		return word
	}
	urlTab := iframeURLTabValue(path)
	if !explorer && c.MainConfig.Found {
		if urlTab == "" {
			return c.MainConfig.tabWord(c.MainConfig.Tab)
		}
		return c.MainConfig.tabWord(urlTab)
	}
	return iframeTabWord(urlTab)
}

// chooseIframeDownloadName is OWID's download name from a valid displaySlug, else from
// download() (the name-only SVG download); it also returns which source was used.
func chooseIframeDownloadName(displaySlug string, download func() string) (string, string) {
	if iframeDisplaySlugRegex.MatchString(displaySlug) {
		return displaySlug + ".svg", iframeNameFromSlug
	}
	if name := download(); name != "" {
		return name, iframeNameFromDownload
	}
	return "", ""
}

// IframeNameHints are a lookup's results sent with a new iframe task. They only feed the
// provisional file name shown before the upload; the task still reads the page itself.
type IframeNameHints struct {
	Path         string `json:"path"`
	DownloadName string `json:"downloadName"`
	Title        string `json:"title"`
	Source       string `json:"source"`
	// The lookup's $TAB word ("" when unknown)
	Tab string `json:"tab"`
}

// iframeVars are the placeholder values available to iframe file names and descriptions.
type iframeVars struct {
	Name   string // OWID download file name without .svg
	Title  string
	Source string
	Url    string
	Tab    string // the view the chart shows, e.g. "Map"; "" when unknown
}

// An unknown $TAB is removed with the brackets around it ("($TAB)", "[$TAB]") and the run of
// spaces, "_" and "-" before it ("$NAME - $TAB" -> "$NAME"), or after it when it starts the
// line ("$TAB - $NAME" -> "$NAME"). Newlines are kept.
var (
	iframeEmptyTabLeading = regexp.MustCompile(`(?m)^(?:\(\$TAB\)|\[\$TAB\]|\$TAB)[ _-]*`)
	iframeEmptyTab        = regexp.MustCompile(`[ _-]*(?:\(\$TAB\)|\[\$TAB\]|\$TAB)`)
)

// resolveIframeVars substitutes $TAB, $NAME, $TITLE, $SOURCE and $URL. An unknown $TAB is
// removed with its separators and brackets (iframeEmptyTab).
func resolveIframeVars(value string, v iframeVars) string {
	if v.Tab != "" {
		value = strings.ReplaceAll(value, "$TAB", v.Tab)
	} else {
		value = iframeEmptyTabLeading.ReplaceAllString(value, "")
		value = iframeEmptyTab.ReplaceAllString(value, "")
	}
	if v.Source != "" {
		value = strings.ReplaceAll(value, "$SOURCE", v.Source)
	}
	return replaceVars(value, ReplaceVarsData{Url: v.Url, Title: v.Title, FileName: v.Name})
}

func trimSVGExtension(name string) string {
	if strings.HasSuffix(strings.ToLower(name), ".svg") {
		return name[:len(name)-len(".svg")]
	}
	return name
}

var (
	commonsSpaceRun = regexp.MustCompile(` {2,}`)
	// What MediaWiki's wfStripIllegalFilenameChars turns into "-" on upload: characters that
	// are not legal in titles (# < > [ ] | { } and control characters) plus $wgIllegalFileChars (: / \)
	commonsIllegalFileChars = regexp.MustCompile(`[#<>\[\]|{}:/\\\x00-\x1f\x7f]`)
)

// CommonsFileName is name as Commons stores it (without "File:"): characters an upload
// cannot use become "-" (as Commons does on upload), "_" becomes a space, runs of spaces
// collapse, the name is trimmed and its first letter uppercased. Final and provisional
// names both go through it, so the {{OWID}} block does not change only because of the
// spelling form.
func CommonsFileName(name string) string {
	name = strings.TrimPrefix(strings.TrimSpace(name), "File:")
	name = commonsIllegalFileChars.ReplaceAllString(name, "-")
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.TrimSpace(commonsSpaceRun.ReplaceAllString(name, " "))
	if name == "" {
		return ""
	}
	r, size := utf8.DecodeRuneInString(name)
	return string(unicode.ToUpper(r)) + name[size:]
}

// iframeFileName is the Commons name a task's file name template resolves to.
func iframeFileName(template string, v iframeVars) string {
	return CommonsFileName(resolveIframeVars(template, v))
}

// ProvisionalIframeFileName resolves a new task's file name template from the lookup hints,
// so the {{OWID}} block can be shown before the task runs. It returns "" when the template
// needs a value the hints do not give ($NAME without a download name, $TITLE/$SOURCE/$TAB
// without a lookup of this exact path). Hints without a tab fall back to the URL's ?tab=.
func ProvisionalIframeFileName(template, path string, hints *IframeNameHints) string {
	v := iframeVars{Url: utils.IframeChartURL(path)}
	lookedUp := hints != nil && hints.Path == path
	if lookedUp {
		v.Name = trimSVGExtension(hints.DownloadName)
		v.Title = hints.Title
		v.Source = hints.Source
		v.Tab = iframeTabWord(hints.Tab)
		if v.Tab == "" {
			v.Tab = iframeURLTab(path)
		}
	}
	if strings.Contains(template, "$NAME") && v.Name == "" {
		return ""
	}
	if !lookedUp && (strings.Contains(template, "$TITLE") || strings.Contains(template, "$SOURCE") || strings.Contains(template, "$TAB")) {
		return ""
	}
	return iframeFileName(template, v)
}

// CreateIframeTaskProcess creates a new iframe task's single process up front, carrying the
// provisional file name when hints resolve it, so the {{OWID}} block is available at once.
// StartIframe reuses this process, so call it before the task is queued.
func CreateIframeTaskProcess(task *models.Task, hints *IframeNameHints) (*models.TaskProcess, error) {
	path, err := utils.NormalizeIframeChartPath(task.URL)
	if err != nil {
		return nil, err
	}
	if existing, _ := models.FindTaskProcessByTaskRegionDate("ALL", "", task.ID); existing != nil {
		return existing, nil
	}
	filename := ProvisionalIframeFileName(task.FileName, path, hints)
	return models.NewTaskProcess("ALL", "", filename, models.TaskProcessStatusProcessing, models.TaskProcessTypeIframe, task.ID)
}

// IframeCaption is the caption for a final Commons file name: the name without
// .svg, with "-" and "_" turned into spaces.
func IframeCaption(filename string) string {
	caption := trimSVGExtension(strings.TrimPrefix(filename, "File:"))
	caption = strings.ReplaceAll(caption, "-", " ")
	return strings.ReplaceAll(caption, "_", " ")
}

// BuildIframeWikiText is the {{OWID}} block used by Template:OWID + the OWIDPopup gadget.
func BuildIframeWikiText(path, filename string) string {
	filename = strings.TrimPrefix(filename, "File:")
	return strings.Join([]string{
		"{{OWID",
		"| chart   = " + path,
		"| title   =",
		"| image   = " + filename,
		"| width   = 400px",
		"| float   = right",
		"| caption = " + IframeCaption(filename),
		"| alt     =",
		"}}",
	}, "\n")
}

// GetIframeTemplate returns the wikitext for an iframe task's file: the final name once
// uploaded, before that the provisional one. A failed process has none.
func GetIframeTemplate(task *models.Task) (string, error) {
	processes, err := models.FindTaskProcessesByTaskId(task.ID)
	if err != nil {
		return "", err
	}
	// A file on Commons wins over a provisional name (e.g. a stray second process)
	var provisional *models.TaskProcess
	for i, p := range processes {
		if p.FileName == "" || p.Status == models.TaskProcessStatusFailed {
			continue
		}
		if iframeFileOnCommons(p.Status) {
			return BuildIframeWikiText(task.URL, CommonsFileName(p.FileName)), nil
		}
		if provisional == nil {
			provisional = &processes[i]
		}
	}
	if provisional != nil {
		return BuildIframeWikiText(task.URL, CommonsFileName(provisional.FileName)), nil
	}
	return "", fmt.Errorf("no file name yet for task %s", task.ID)
}

// iframeFileOnCommons reports whether a process status means its file is on Commons.
func iframeFileOnCommons(status models.TaskProcessStatus) bool {
	switch status {
	case models.TaskProcessStatusUploaded, models.TaskProcessStatusOverwritten, models.TaskProcessStatusSkipped, models.TaskProcessStatusDescriptionUpdated:
		return true
	}
	return false
}

func iframeTitleFallback(path string) string {
	slug := utils.IframeChartSlug(path)
	if slug == "" {
		return ""
	}
	return utils.ToTitle(slug)
}

// openIframeChartPage opens the exact OWID URL and waits until the chart's download button is present.
func openIframeChartPage(browser *rod.Browser, url string) (*rod.Page, error) {
	page, err := browser.Page(proto.TargetCreateTarget{URL: ""})
	if err != nil {
		return nil, err
	}
	if err := loadIframeChartPage(page, url); err != nil {
		page.Close()
		return nil, err
	}
	return page, nil
}

// loadIframeChartPage navigates page to url and waits until the chart's download button is present.
func loadIframeChartPage(page *rod.Page, url string) error {
	// A hung OWID page must not block the task or the /iframe/info request
	timed := page.Timeout(iframePageLoadTimeout)
	err := rod.Try(func() {
		timed.MustSetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: env.GetEnv().OWID_UA})
		timed.MustSetViewport(constants.VIEWPORT_WIDTH, constants.VIEWPORT_HEIGHT, 1, false)
		timed.MustNavigate(url)
		timed.MustWaitLoad()
		timed.MustWaitIdle()
	})
	timed.CancelTimeout()
	if err == nil {
		err = utils.WaitElementWithTimeout(page, DOWNLOAD_BUTTON_SELECTOR, iframeModalTimeout)
	}
	if err != nil {
		return fmt.Errorf("cannot open chart page %s: %w", url, err)
	}
	return nil
}

func readIframePageDetails(page *rod.Page, path string) (string, string) {
	// One bounded context for every lookup; on timeout the title falls back to the slug and the source stays empty
	timed := page.Timeout(iframeDetailsTimeout)
	defer timed.CancelTimeout()
	title := ""
	source := ""
	_ = rod.Try(func() {
		// Non-waiting lookups: page.Element would block on a missing selector
		for _, selector := range strings.Split(TITLE_SELECTOR, ",") {
			if els, err := timed.Elements(strings.TrimSpace(selector)); err == nil && len(els) > 0 {
				if text, err := els[0].Text(); err == nil && strings.TrimSpace(text) != "" {
					title = strings.TrimSpace(text)
					break
				}
			}
		}
		source = getMapSourceFromPage(timed)
	})
	if title == "" {
		title = iframeTitleFallback(path)
	}
	return title, source
}

// GetIframeInfo looks up title and source of an OWID grapher/explorer page (read only).
func GetIframeInfo(browser *rod.Browser, input string) (*IframeInfo, error) {
	return getIframeInfo(context.Background(), browser, input)
}

// getIframeInfo is GetIframeInfo ended early when ctx ends (the /iframe/info client gave up).
func getIframeInfo(ctx context.Context, browser *rod.Browser, input string) (*IframeInfo, error) {
	path, err := utils.NormalizeIframeChartPath(input)
	if err != nil {
		return nil, err
	}
	tab, err := browser.Page(proto.TargetCreateTarget{URL: ""})
	if err != nil {
		return nil, err
	}
	// Closed through tab, whose context ctx does not end, so a cancelled lookup still closes it
	defer tab.Close()
	pageCtx, cancel := context.WithCancel(tab.GetContext())
	defer cancel()
	defer context.AfterFunc(ctx, cancel)()
	page := tab.Context(pageCtx)
	if err := loadIframeChartPage(page, utils.IframeChartURL(path)); err != nil {
		return nil, err
	}
	title, source := readIframePageDetails(page, path)
	info := &IframeInfo{Path: path, ChartName: iframeChartName(title, utils.IframeChartSlug(path)), Title: title, Source: source}
	slugs := readIframeDisplaySlug(page)
	info.Tab = pickIframeTab(path, slugs)
	info.DownloadName, info.downloadNameSource = chooseIframeDownloadName(pickIframeDisplaySlug(path, slugs), func() string {
		return lookupIframeDownloadName(page, path)
	})
	fmt.Printf("Iframe info: download name %q tab %q for %s from %q (slugs %+v)\n", info.DownloadName, info.Tab, path, info.downloadNameSource, slugs)
	return info, nil
}

// lookupDownloadSlot lets one lookup download at a time (see lookupIframeDownloadName).
var lookupDownloadSlot = make(chan struct{}, 1)

// lookupIframeDownloadName downloads the SVG once like the task does to learn OWID's file
// name ($NAME). It gives up after iframeInfoDownloadTimeout; a failure or timeout returns "".
// The caller closing page (and its browser) ends a download still running after the deadline.
func lookupIframeDownloadName(page *rod.Page, path string) string {
	// Lookups share a browser, and a browser's download path and download events are
	// browser-wide: two downloads at once could each catch the other's file name. The wait
	// comes before the download's own deadline; the downloads ahead can't take longer than this cap.
	select {
	case lookupDownloadSlot <- struct{}{}:
	case <-page.GetContext().Done():
		return ""
	case <-time.After(lookupMaxConcurrent * (iframeInfoDownloadTimeout + time.Second)):
		fmt.Println("Iframe info: gave up waiting for another SVG download", path)
		return ""
	}
	dir, err := os.MkdirTemp("", "owid-iframe-info")
	if err != nil {
		<-lookupDownloadSlot
		return ""
	}
	timed := page.Timeout(iframeInfoDownloadTimeout)
	result := make(chan string, 1)
	go func() {
		defer func() { <-lookupDownloadSlot }()
		defer os.RemoveAll(dir)
		defer timed.CancelTimeout()
		name, err := downloadOWIDSVG(timed, dir, true, false)
		if err != nil {
			fmt.Println("Iframe info: SVG download failed", path, err)
		}
		result <- name
	}()
	select {
	case name := <-result:
		return name
	case <-time.After(iframeInfoDownloadTimeout + time.Second):
		fmt.Println("Iframe info: SVG download timed out", path)
		return ""
	}
}

// iframeChartName mirrors the map import: the page title when there is one, else the slug in title case.
func iframeChartName(title, slug string) string {
	if title != "" && title != slug {
		return title
	}
	if slug == "" {
		return ""
	}
	return utils.ToTitle(slug)
}

// findDownloadCheckbox returns the modal checkbox input whose label starts with labelPrefix.
func findDownloadCheckbox(page *rod.Page, labelPrefix string) (*rod.Element, *rod.Element, error) {
	labels, err := page.Elements(IFRAME_DOWNLOAD_CHECKBOX_LABEL_SELECTOR)
	if err != nil {
		return nil, nil, err
	}
	for _, label := range labels {
		textEl, err := label.Element(IFRAME_DOWNLOAD_CHECKBOX_TEXT_SELECTOR)
		if err != nil {
			continue
		}
		text, err := textEl.Text()
		if err != nil || !strings.HasPrefix(strings.ToLower(strings.TrimSpace(text)), strings.ToLower(labelPrefix)) {
			continue
		}
		in, err := label.Element(IFRAME_DOWNLOAD_CHECKBOX_INPUT_SELECTOR)
		if err != nil {
			return nil, nil, err
		}
		return label, in, nil
	}
	return nil, nil, nil
}

func checkboxChecked(in *rod.Element) (bool, error) {
	v, err := in.Property("checked")
	if err != nil {
		return false, err
	}
	return v.Bool(), nil
}

// setDownloadCheckbox puts the labelled checkbox into the wanted state and verifies it.
// A missing checkbox is an error only when required.
func setDownloadCheckbox(page *rod.Page, labelPrefix string, want, required bool) error {
	label, in, err := findDownloadCheckbox(page, labelPrefix)
	if err != nil {
		return err
	}
	if in == nil {
		if required {
			return fmt.Errorf("checkbox %q not found in download modal", labelPrefix)
		}
		return nil
	}
	for attempt := 0; attempt < 3; attempt++ {
		checked, err := checkboxChecked(in)
		if err != nil {
			return err
		}
		if checked == want {
			return nil
		}
		if err := label.Click(proto.InputMouseButtonLeft, 1); err != nil {
			return fmt.Errorf("clicking checkbox %q: %w", labelPrefix, err)
		}
		// Each toggle regenerates the export (buttons disabled for a few seconds).
		// Another toggle during that window can be lost, so wait it out.
		if err := waitModalRegenerated(page); err != nil {
			return err
		}
	}
	return fmt.Errorf("checkbox %q did not reach checked=%v", labelPrefix, want)
}

func verifyDownloadCheckbox(page *rod.Page, labelPrefix string, want, required bool) error {
	_, in, err := findDownloadCheckbox(page, labelPrefix)
	if err != nil {
		return err
	}
	if in == nil {
		if required {
			return fmt.Errorf("checkbox %q not found in download modal", labelPrefix)
		}
		return nil
	}
	checked, err := checkboxChecked(in)
	if err != nil {
		return err
	}
	if checked != want {
		return fmt.Errorf("checkbox %q is checked=%v, want %v", labelPrefix, checked, want)
	}
	return nil
}

// waitModalRegenerated waits for the SVG button to turn disabled (regeneration
// started) and then enabled again.
func waitModalRegenerated(page *rod.Page) error {
	deadline := time.Now().Add(iframeRegenStartTimeout)
	for time.Now().Before(deadline) && page.GetContext().Err() == nil {
		if btn, err := page.Element(IFRAME_DOWNLOAD_SVG_BUTTON_SELECTOR); err == nil && btn != nil {
			if disabled, err := btn.Attribute("disabled"); err == nil && disabled != nil {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	_, err := waitSVGButtonEnabled(page)
	return err
}

// waitSVGButtonEnabled waits for the modal's SVG button to exist and lose its disabled attribute.
func waitSVGButtonEnabled(page *rod.Page) (*rod.Element, error) {
	deadline := time.Now().Add(iframeButtonTimeout)
	for time.Now().Before(deadline) {
		if err := page.GetContext().Err(); err != nil {
			return nil, fmt.Errorf("SVG download button not enabled: %w", err)
		}
		if btn, err := page.Element(IFRAME_DOWNLOAD_SVG_BUTTON_SELECTOR); err == nil && btn != nil {
			if disabled, err := btn.Attribute("disabled"); err == nil && disabled == nil {
				return btn, nil
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	return nil, fmt.Errorf("SVG download button not enabled")
}

// downloadOWIDSVG opens url, opens the Download modal, sets the two SVG options,
// verifies them, downloads the SVG into dir and returns OWID's download file name.
// The whole step is bounded by 2*iframeDownloadTimeout.
func downloadOWIDSVG(page *rod.Page, dir string, optimize, terminology bool) (string, error) {
	page = page.Timeout(2 * iframeDownloadTimeout)
	defer page.CancelTimeout()

	downloadBtn, err := page.Element(DOWNLOAD_BUTTON_SELECTOR)
	if err != nil {
		return "", fmt.Errorf("cannot find download button: %w", err)
	}
	if err := downloadBtn.Focus(); err != nil {
		return "", err
	}
	time.Sleep(200 * time.Millisecond)
	if err := page.Keyboard.Press(input.Enter); err != nil {
		return "", fmt.Errorf("error opening download modal: %w", err)
	}
	if err := utils.WaitElementWithTimeout(page, IFRAME_DOWNLOAD_CHECKBOX_LABEL_SELECTOR, iframeModalTimeout); err != nil {
		return "", fmt.Errorf("download modal options not found: %w", err)
	}
	defer CloseDownloadPopup(page)

	// Toggle only once the modal is interactive, or the change can be lost
	if _, err := waitSVGButtonEnabled(page); err != nil {
		return "", err
	}
	if err := setDownloadCheckbox(page, IFRAME_OPTIMIZE_SVG_LABEL, optimize, true); err != nil {
		return "", err
	}
	if err := setDownloadCheckbox(page, IFRAME_TERMINOLOGY_LABEL, terminology, false); err != nil {
		return "", err
	}

	svgBtn, err := waitSVGButtonEnabled(page)
	if err != nil {
		return "", err
	}
	// Re-check right before downloading: toggling can re-render the modal
	if err := verifyDownloadCheckbox(page, IFRAME_OPTIMIZE_SVG_LABEL, optimize, true); err != nil {
		return "", err
	}
	if err := verifyDownloadCheckbox(page, IFRAME_TERMINOLOGY_LABEL, terminology, false); err != nil {
		return "", err
	}

	wait := page.Browser().WaitDownload(dir)
	if err := svgBtn.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return "", fmt.Errorf("error clicking SVG download button: %w", err)
	}

	done := make(chan *proto.PageDownloadWillBegin, 1)
	go func() { done <- wait() }()
	var info *proto.PageDownloadWillBegin
	select {
	case info = <-done:
	case <-time.After(iframeDownloadTimeout):
		return "", fmt.Errorf("timeout waiting for SVG download")
	case <-page.GetContext().Done():
		// Overall step deadline, so one attempt stays well inside the stalled-task window
		return "", fmt.Errorf("timeout waiting for SVG download: %w", page.GetContext().Err())
	}
	if info == nil {
		return "", fmt.Errorf("SVG download did not start")
	}

	downloaded := filepath.Join(dir, info.GUID)
	if optimize {
		// The Wikipedia-optimized export drops the external font @import
		content, err := os.ReadFile(downloaded)
		if err != nil {
			return "", fmt.Errorf("downloaded file not found: %w", err)
		}
		if strings.Contains(string(content), "@import url(") {
			os.Remove(downloaded)
			return "", fmt.Errorf("downloaded SVG is not the Wikipedia-optimized export")
		}
	}

	name := filepath.Base(info.SuggestedFilename)
	if name == "" || name == "." || name == "/" {
		name = info.GUID + ".svg"
	}
	// Downloads are saved under the GUID; keep OWID's name on disk
	if err := os.Rename(downloaded, filepath.Join(dir, name)); err != nil {
		return "", fmt.Errorf("downloaded file not found: %w", err)
	}
	return name, nil
}

// iframeUploadInput holds already-resolved values for uploadIframeFile.
type iframeUploadInput struct {
	FileName    string
	Description string
	Comment     string
	Url         string
	Behaviour   models.DescriptionOverwriteBehaviour
}

// uploadIframeFile uploads the single SVG in downloadPath as-is (no SVG cleanup),
// except that an overwrite keeps the existing file's svgtranslate translations.
// Same name with different content is overwritten; identical content already on the
// wiki under another name is not uploaded and that name is returned instead.
func uploadIframeFile(user *models.User, token string, in iframeUploadInput, downloadPath string) (string, string, error) {
	filename := in.FileName
	fileInfo, err := getFileInfo(downloadPath)
	if err != nil {
		return filename, "", err
	}

	page, err := getCommonsFilePageByName(filename, user)
	if err != nil {
		return filename, "", err
	}
	// Use the title Commons stores (first letter uppercased, "_" as spaces), even for
	// a missing page: it is the name the {{OWID}} block must reference.
	if canonical := strings.TrimPrefix(page.Title, "File:"); canonical != "" {
		filename = canonical
	}

	replacingRedirect := page.Redirect
	if replacingRedirect {
		page.ImageInfo = nil
	}

	upload := func(text string) error {
		res, err := utils.DoApiReq[UploadResponse](user, map[string]string{
			"action":         "upload",
			"comment":        in.Comment,
			"text":           text,
			"filename":       filename,
			"ignorewarnings": "1",
			"token":          token,
		}, &utils.UploadedFile{
			Filename: filename,
			File:     string(fileInfo.File),
			Mime:     "image/svg+xml",
		})
		if err != nil {
			return err
		}
		if res.Upload.Result != "Success" {
			return fmt.Errorf("upload failed: %s", res.Upload.Result)
		}
		// The name the file was actually stored under
		if res.Upload.Filename != "" {
			filename = res.Upload.Filename
		}
		return nil
	}

	if len(page.ImageInfo) == 0 {
		shaRes, err := utils.DoApiReq[QueryBySHA1Response](user, map[string]string{
			"action": "query",
			"list":   "allimages",
			"aisha1": fileInfo.Sha1,
			"aiprop": "url|sha1|size",
		}, nil)
		if err != nil {
			return filename, "", err
		}
		if len(shaRes.Query.AllImages) > 0 {
			return shaRes.Query.AllImages[0].Name, "skipped", nil
		}
		if err := upload(in.Description); err != nil {
			return filename, "", err
		}
		if replacingRedirect {
			// "text" only applies to new pages; the redirect page has to be edited
			if _, err := updateFileDescriptionIfChanged(user, token, filename, "", in.Description, in.Url); err != nil {
				return filename, "", err
			}
		}
		return filename, "uploaded", nil
	}

	// "Skip upload/edit": an existing file is left exactly as it is
	if in.Behaviour == models.DescriptionOverwriteBehaviourSkip {
		return filename, "skipped", nil
	}

	var wikiText string
	newFileDesc := strings.TrimSpace(in.Description)
	switch in.Behaviour {
	case models.DescriptionOverwriteBehaviourExceptCategories:
		wikiText, err = getFileWikiText(user, filename)
		if err != nil {
			return filename, "", fmt.Errorf("Error getting wikitext for except-category overwrite")
		}
		for _, text := range extractCategories(in.Description) {
			newFileDesc = strings.ReplaceAll(newFileDesc, text, "")
		}
		newFileDesc = strings.TrimSpace(newFileDesc)
		for _, text := range extractCategories(wikiText) {
			newFileDesc = newFileDesc + "\n" + text
		}
	case models.DescriptionOverwriteBehaviourOnlyFile:
		wikiText, err = getFileWikiText(user, filename)
		if err != nil {
			return filename, "", fmt.Errorf("Error getting wikitext for file-only overwrite")
		}
		newFileDesc = wikiText
	}

	// Never lose svgtranslate translations of the Commons file (as map imports do):
	// re-apply them to OWID's download, or keep the Commons file when nothing visible
	// changed or they cannot all be carried over.
	keepExisting := false
	if page.ImageInfo[0].SHA1 != fileInfo.Sha1 {
		defer os.RemoveAll(downloadPath + "_existing")
		var unchanged bool
		fileInfo, unchanged, err = preserveExistingTranslations(filename, page.ImageInfo[0].URL, downloadPath, fileInfo)
		if err != nil {
			return filename, "", fmt.Errorf("error preserving translations: %w", err)
		}
		keepExisting = unchanged
	}

	if keepExisting || page.ImageInfo[0].SHA1 == fileInfo.Sha1 {
		updated, err := updateFileDescriptionIfChanged(user, token, filename, wikiText, newFileDesc, in.Url)
		if err != nil {
			return filename, "", err
		}
		if updated {
			return filename, "description_updated", nil
		}
		return filename, "skipped", nil
	}

	if err := upload(newFileDesc); err != nil {
		return filename, "", err
	}
	// The upload API only applies "text" to new file pages
	if _, err := updateFileDescriptionIfChanged(user, token, filename, wikiText, newFileDesc, in.Url); err != nil {
		return filename, "", err
	}
	return filename, "overwritten", nil
}

func failIframeTask(task *models.Task, taskProcess *models.TaskProcess, err error) error {
	if taskProcess != nil {
		FailTaskProcess(taskProcess)
	}
	task.Status = models.TaskStatusFailed
	task.Update()
	utils.SendWSTask(task)
	return err
}

// StartIframe processes a queued iframe task: download OWID's Wikipedia-optimized SVG
// of the exact chart URL and upload it.
func StartIframe(taskId string, user *models.User) error {
	task, err := models.FindTaskById(taskId)
	if err != nil {
		return err
	}
	// Cancelled after the queue claimed it
	if task.Status == models.TaskStatusCancelled {
		return nil
	}

	task.Status = models.TaskStatusProcessing
	if err := task.Update(); err != nil {
		fmt.Println("Error setting task to Processing: ", err)
	}
	models.UpdateTaskLastOperationAt(task.ID)
	utils.SendWSTask(task)

	path, err := utils.NormalizeIframeChartPath(task.URL)
	if err != nil || task.FileName == "" || task.Description == "" {
		return failIframeTask(task, nil, fmt.Errorf("invalid iframe task data"))
	}
	chartURL := utils.IframeChartURL(path)

	taskProcess, err := ClaimIframeTaskProcess(task.ID)
	if err != nil {
		return failIframeTask(task, nil, fmt.Errorf("couldn't create task process: %w", err))
	}
	utils.SendWSTaskProcess(task.ID, taskProcess)

	tokenResponse, err := utils.DoApiReq[TokenResponse](user, map[string]string{
		"action": "query",
		"meta":   "tokens",
		"format": "json",
	}, nil)
	if err != nil {
		return failIframeTask(task, taskProcess, fmt.Errorf("error fetching edit token: %w", err))
	}
	token := tokenResponse.Query.Tokens.CsrfToken

	tmpDir, err := os.MkdirTemp("", "owid-iframe")
	if err != nil {
		return failIframeTask(task, taskProcess, err)
	}
	defer os.RemoveAll(tmpDir)

	// Each browser step is bounded (page load, modal, download), and the task's
	// LastOperationAt is refreshed between them, so the 5-minute stalled-task monitor
	// does not fail a task that is still working.
	touch := func() { models.UpdateTaskLastOperationAt(task.ID) }
	downloadName, title, source, tab, err := downloadIframeSVG(chartURL, path, tmpDir, touch)
	if err != nil {
		return failIframeTask(task, taskProcess, err)
	}
	models.UpdateTaskLastOperationAt(task.ID)

	// Cancelled while downloading
	if err := task.Reload(); err == nil && task.Status != models.TaskStatusProcessing {
		FailTaskProcess(taskProcess)
		utils.SendWSTask(task)
		return nil
	}

	// Same name as the chart details box; saved with the task's next update
	task.ChartName = iframeChartName(title, utils.IframeChartSlug(path))

	vars := iframeVars{Name: trimSVGExtension(downloadName), Title: title, Source: source, Url: chartURL, Tab: tab}
	// The name is known now: publish it before the upload so the {{OWID}} block shows at once
	if provisional := iframeFileName(task.FileName, vars); provisional != "" && provisional != taskProcess.FileName {
		taskProcess.FileName = provisional
		if err := taskProcess.Update(); err != nil {
			fmt.Println("Error saving provisional iframe file name", err)
		}
		utils.SendWSTaskProcess(task.ID, taskProcess)
	}

	filename, status, err := uploadIframeFile(user, token, iframeUploadInput{
		FileName:    resolveIframeVars(task.FileName, vars),
		Description: resolveIframeVars(task.Description, vars),
		Comment:     "Importing from " + chartURL,
		Url:         chartURL,
		Behaviour:   task.DescriptionOverwriteBehaviour,
	}, tmpDir)
	if err != nil {
		fmt.Println("Iframe upload error: ", err)
		return failIframeTask(task, taskProcess, fmt.Errorf("upload error: %w", err))
	}

	taskProcess.FileName = filename
	switch status {
	case "skipped":
		taskProcess.Status = models.TaskProcessStatusSkipped
	case "description_updated":
		taskProcess.Status = models.TaskProcessStatusDescriptionUpdated
	case "overwritten":
		taskProcess.Status = models.TaskProcessStatusOverwritten
	default:
		taskProcess.Status = models.TaskProcessStatusUploaded
	}
	taskProcess.Update()
	utils.SendWSTaskProcess(task.ID, taskProcess)

	task.Status = models.TaskStatusDone
	if err := task.Update(); err != nil {
		fmt.Println("Error saving task status to done: ", err)
	}
	utils.SendWSTask(task)
	return nil
}

// ClaimIframeTaskProcess marks a task's "ALL" process as processing, reusing the one created
// with the task (or on an earlier run), else creating it.
func ClaimIframeTaskProcess(taskId string) (*models.TaskProcess, error) {
	if existing, _ := models.FindTaskProcessByTaskRegionDate("ALL", "", taskId); existing != nil {
		existing.Status = models.TaskProcessStatusProcessing
		if err := existing.Update(); err != nil {
			fmt.Println("Error updating task process to processing", err)
		}
		return existing, nil
	}
	return models.NewTaskProcess("ALL", "", "", models.TaskProcessStatusProcessing, models.TaskProcessTypeIframe, taskId)
}

// downloadIframeSVG downloads the Wikipedia-optimized SVG (terminology definitions off)
// into dir and returns OWID's file name plus the page's title, source and $TAB word.
// touch is called before each bounded step to mark the task as alive.
func downloadIframeSVG(chartURL, path, dir string, touch func()) (string, string, string, string, error) {
	l, browser := GetBrowser()
	defer l.Cleanup()
	defer browser.Close()

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		touch()
		page, err := openIframeChartPage(browser, chartURL)
		if err != nil {
			lastErr = err
			continue
		}
		touch()
		title, source := readIframePageDetails(page, path)
		// Read before the download modal opens, from the same eval as the lookup's
		tab := pickIframeTab(path, readIframeDisplaySlug(page))
		name, err := downloadOWIDSVG(page, dir, true, false)
		page.Close()
		if err == nil {
			return name, title, source, tab, nil
		}
		fmt.Println("Iframe SVG download attempt failed", attempt, err)
		lastErr = err
	}
	return "", "", "", "", lastErr
}
