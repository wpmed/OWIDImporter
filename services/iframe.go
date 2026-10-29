package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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

// iframeDetailsTimeout bounds reading the title and source from a loaded page together,
// so a renderer that hangs after load cannot stall /iframe/info or a task.
var iframeDetailsTimeout = 15 * time.Second

// IframeInfo is the read-only lookup result for an iframe import.
type IframeInfo struct {
	Path      string `json:"path"`
	ChartName string `json:"chartName"`
	Title     string `json:"title"`
	Source    string `json:"source"`
}

// iframeVars are the placeholder values available to iframe file names and descriptions.
type iframeVars struct {
	Name   string // OWID download file name without .svg
	Title  string
	Source string
	Url    string
}

// resolveIframeVars substitutes $NAME, $TITLE, $SOURCE and $URL.
func resolveIframeVars(value string, v iframeVars) string {
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

// GetIframeTemplate returns the wikitext for a done iframe task.
func GetIframeTemplate(task *models.Task) (string, error) {
	processes, err := models.FindTaskProcessesByTaskId(task.ID)
	if err != nil {
		return "", err
	}
	for _, p := range processes {
		if p.FileName != "" && p.Status != models.TaskProcessStatusFailed {
			return BuildIframeWikiText(task.URL, p.FileName), nil
		}
	}
	return "", fmt.Errorf("no uploaded file for task %s", task.ID)
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
	// A hung OWID page must not block the task or the /iframe/info request
	timed := page.Timeout(iframePageLoadTimeout)
	err = rod.Try(func() {
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
		page.Close()
		return nil, fmt.Errorf("cannot open chart page %s: %w", url, err)
	}
	return page, nil
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
	path, err := utils.NormalizeIframeChartPath(input)
	if err != nil {
		return nil, err
	}
	page, err := openIframeChartPage(browser, utils.IframeChartURL(path))
	if err != nil {
		return nil, err
	}
	defer page.Close()
	title, source := readIframePageDetails(page, path)
	return &IframeInfo{Path: path, ChartName: iframeChartName(title, utils.IframeChartSlug(path)), Title: title, Source: source}, nil
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

	var taskProcess *models.TaskProcess
	if existing, _ := models.FindTaskProcessByTaskRegionDate("ALL", "", task.ID); existing != nil {
		existing.Status = models.TaskProcessStatusProcessing
		if err := existing.Update(); err != nil {
			fmt.Println("Error updating task process to processing", err)
		}
		taskProcess = existing
	} else {
		taskProcess, err = models.NewTaskProcess("ALL", "", "", models.TaskProcessStatusProcessing, models.TaskProcessTypeIframe, task.ID)
		if err != nil {
			return failIframeTask(task, nil, fmt.Errorf("couldn't create task process: %w", err))
		}
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
	downloadName, title, source, err := downloadIframeSVG(chartURL, path, tmpDir, touch)
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

	vars := iframeVars{Name: trimSVGExtension(downloadName), Title: title, Source: source, Url: chartURL}
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

// downloadIframeSVG downloads the Wikipedia-optimized SVG (terminology definitions off)
// into dir and returns OWID's file name plus the page's title and source.
// touch is called before each bounded step to mark the task as alive.
func downloadIframeSVG(chartURL, path, dir string, touch func()) (string, string, string, error) {
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
		name, err := downloadOWIDSVG(page, dir, true, false)
		page.Close()
		if err == nil {
			return name, title, source, nil
		}
		fmt.Println("Iframe SVG download attempt failed", attempt, err)
		lastErr = err
	}
	return "", "", "", lastErr
}
