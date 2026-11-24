package services

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
	"github.com/wpmed-videowiki/OWIDImporter/constants"
	"github.com/wpmed-videowiki/OWIDImporter/env"
	"github.com/wpmed-videowiki/OWIDImporter/models"
	svgprocessor "github.com/wpmed-videowiki/OWIDImporter/svg_processor"
	"github.com/wpmed-videowiki/OWIDImporter/utils"
	"golang.org/x/sync/errgroup"
)

func StartMap(taskId string, user *models.User, data StartData) error {
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

	url := utils.AttachQueryParamToUrl(data.Url, "tab=map")

	// url := fmt.Sprintf("%s%s?tab=map", constants.OWID_BASE_URL, chartName)
	if task.ChartParameters != "" {
		url = utils.AttachQueryParamToUrl(url, task.ChartParameters)
	}

	fmt.Println("==================== CONSTRUCTED URL: ", url)

	err = ValidateParameters(data)
	if err != nil {
		task.Status = models.TaskStatusFailed
		task.Update()
		utils.SendWSTask(task)
		return err
	}

	chartInfo, err := GetChartInfo(url, data.TemplateNameFormat, task.ChartParameters)
	if err != nil {
		fmt.Println("Error getting chart info: ", err)
		task.Status = models.TaskStatusFailed
		task.Update()
		utils.SendWSTask(task)
		return fmt.Errorf("Error getting chart info")
	}

	task.ChartName = chartInfo.ChartName
	if task.ChartName == "" {
		task.Status = models.TaskStatusFailed
		task.Update()
		utils.SendWSTask(task)
		return fmt.Errorf("invalid url")
	}

	if chartInfo.StableUrl != "" {
		data.Url = chartInfo.StableUrl
		if task.URL != data.Url {
			task.URL = data.Url
			task.Update()
		}
	}

	fmt.Println("Chart Name:", task.ChartName)

	tmpDir, err := os.MkdirTemp("", "owid-exporter")
	if err != nil {
		fmt.Println("Error creating temp directory", err)
		return err
	}
	defer os.RemoveAll(tmpDir)

	// Check for being a single image
	if chartInfo.SingleImage {
		// Direct upload
		if err := processSingleImage(task, user, chartInfo, tmpDir, data); err != nil {
			task.Status = models.TaskStatusFailed
			task.Update()
			utils.SendWSTask(task)
			return err
		}

		task.Status = models.TaskStatusDone
		task.Update()
		utils.SendWSTask(task)
		return nil
	}

	chartParamsMap := chartInfo.ParamsMap
	templateName := GenerateTemplateCommonsName(data.TemplateNameFormat, task.ChartName, chartParamsMap)
	task.CommonsTemplateName = templateName
	models.UpdateTaskLastOperationAt(task.ID)
	task.Update()
	utils.SendWSTask(task)

	startYear := chartInfo.StartYear
	endYear := chartInfo.EndYear
	title := chartInfo.Title

	fmt.Println("Got start/end year: ", startYear, endYear, title)
	fmt.Println("Got params: ", chartParamsMap)
	fmt.Print("Has countries: ", chartInfo.HasCountries)

	if startYear == "" || endYear == "" {
		// utils.SendWSMessage(session, "debug", fmt.Sprintf("%s:failed", region))
		task.Status = models.TaskStatusFailed
		task.Update()
		utils.SendWSTask(task)
		return fmt.Errorf("failed to get start and end years")
	}

	// startTime := time.Now()

	done := false
	defer func() {
		done = true
	}()

	// Reload task every 5 sec to handle cancellation
	go func() {
		for {
			time.Sleep(5 * time.Second)
			if done {
				break
			}
			task.Reload()
			if task.Status != models.TaskStatusProcessing {
				break
			}
		}
	}()

	// Work that failed without failed item rows to show it (errIncomplete); the rest still runs
	var incomplete []error
	if task.ImportCountries == 1 && task.Status == models.TaskStatusProcessing {
		fmt.Print("================= STARTED IMPORTING COUNTRIES")
		if err := processCountries(chartInfo, user, task, data.Url, tmpDir, title, startYear, endYear, chartParamsMap); err != nil {
			if !errors.Is(err, errIncomplete) {
				task.Status = models.TaskStatusFailed
				task.Update()
				utils.SendWSTask(task)
				return err
			}
			incomplete = append(incomplete, err)
		}
	}

	if task.Status == models.TaskStatusProcessing {
		// Process regions
		if err := processRegions(task, user, tmpDir, title, chartParamsMap, data); err != nil {
			incomplete = append(incomplete, err)
		}
	}

	finishMapTask(task, user, data, errors.Join(incomplete...))
	return nil
}

// finishMapTask ends a map task that is still processing. Items (years, countries) that failed
// keep their failed rows, which the task page counts and offers to retry, under a done task.
// Work with no rows to show its failure (incomplete: a whole region or the popover pass) ends
// the task failed instead, so it never looks fully successful; a retry redoes only what is
// missing.
func finishMapTask(task *models.Task, user *models.User, data StartData, incomplete error) {
	if task.Status == models.TaskStatusProcessing {
		if data.GenerateTemplateCommons {
			processCommonsTemplate(task, user)
		}

		task.Status = models.TaskStatusDone
		if incomplete != nil {
			fmt.Printf("Task %s finished with work missing, marking it failed: %v\n", task.ID, incomplete)
			task.Status = models.TaskStatusFailed
		}
		if err := task.Update(); err != nil {
			fmt.Println("Error saving task staus to done: ", err)
		}
	}

	utils.SendWSTask(task)
}

func processSingleImage(task *models.Task, user *models.User, chartInfo *ChartInfo, tmpDir string, data StartData) error {
	fmt.Println("===================== Prcessing Single Image ===============", task.URL)
	tokenResponse, err := utils.DoApiReq[TokenResponse](user, map[string]string{
		"action": "query",
		"meta":   "tokens",
		"format": "json",
	}, nil)
	if err != nil {
		fmt.Println("Error fetching edit token", err)
		return err
	}
	token := tokenResponse.Query.Tokens.CsrfToken

	var taskProcess *models.TaskProcess
	// Try to find existing process, otherwise create one
	existingTB, err := models.FindTaskProcessByTaskRegionDate("ALL", "", task.ID)
	if existingTB != nil {
		existingTB.Status = models.TaskProcessStatusProcessing
		if err := existingTB.Update(); err != nil {
			fmt.Println("Error updating task process to processing")
		}
		taskProcess = existingTB
	} else {
		taskProcess, err = models.NewTaskProcess("ALL", "", "", models.TaskProcessStatusProcessing, models.TaskProcessTypeCountry, task.ID)
		if err != nil {
			fmt.Println("ERROR finding task process for country code", "ALL", err)
			return fmt.Errorf("Couldn't create task process")
		}
	}

	utils.SendWSTaskProcess(task.ID, taskProcess)
	models.UpdateTaskLastOperationAt(task.ID)
	downloadPath := path.Join(tmpDir, "ALL")
	if err := os.Mkdir(downloadPath, 0755); err != nil {
		fmt.Println("Error creating download directory: ", "ALL", err)
		FailTaskProcess(taskProcess)
		return fmt.Errorf("Error creating download directory")
	}

	w, err := openBrowserWindow()
	if err != nil {
		FailTaskProcess(taskProcess)
		return err
	}
	defer w.Close()

	// A rerun in a relaunched browser when the browser dies while downloading
	if err := w.runWithCrashRetry("single image "+task.URL, func(p *rod.Page) error {
		page := p
		// Not ready (load timeout, controls missing): retried once in a fresh window
		if err := loadChartPage(page, task.URL); err != nil {
			return fmt.Errorf("loading %s: %v: %w", task.URL, err, errPageNotReady)
		}
		// utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:processing", country))
		if err := utils.WaitElementWithTimeout(page, DOWNLOAD_BUTTON_SELECTOR, time.Second*10); err != nil {
			return fmt.Errorf("Cannot find download button in page: %w", errPageNotReady)
		}

		downloadBtn := page.MustElement(DOWNLOAD_BUTTON_SELECTOR)
		downloadBtn.MustFocus()
		time.Sleep(time.Millisecond * 200)

		if err := page.Keyboard.Press(input.Enter); err != nil {
			return fmt.Errorf("Error clicking download button: %w", errPageNotReady)
		}

		fmt.Println("GOT DOWNLOAD BTN SELECTOR, WAITING FOR SVG")
		if err := utils.WaitElementWithTimeout(page, DOWNLOAD_SVG_ICON_SELECTOR, time.Second*10); err != nil {
			CloseDownloadPopup(page)
			return fmt.Errorf("Can't find DOWNLOAD_SVG_SELECTOR: %w", errPageNotReady)
		}

		elements := page.MustElements(DOWNLOAD_SVG_BUTTON_SELECTOR)

		if _, err := captureOWIDSVGToDir(page, downloadPath, svgCaptureTimeout, clickElement(elements[0])); err != nil {
			// utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:failed", country))
			fmt.Println("Error downloading svg: ", err)
			if !windowResponds(page) {
				// Replaced and retried by runWithCrashRetry
				return fmt.Errorf("downloading the SVG: %w", errWindowStuck)
			}
			CloseDownloadPopup(page)
			return fmt.Errorf("Error clicking download svg button")
		}
		fmt.Println("============= DOWNLOAD DONE =============")

		CloseDownloadPopup(page)
		return nil
	}, nil); err != nil {
		fmt.Println("Error downloading single image: ", err)
		FailTaskProcess(taskProcess)
		return err
	}
	if _, err := os.Stat(downloadPath); os.IsNotExist(err) {
		// utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:failed", country))
		FailTaskProcess(taskProcess)
		fmt.Println(err)
		return fmt.Errorf("File not found")
	}

	replaceData := ReplaceVarsData{
		Url:      data.Url,
		Title:    chartInfo.Title,
		FileName: GetFileNameFromChartName(chartInfo.Title),
		Comment:  "Importing from " + data.Url,
	}

	filename, status, err := uploadMapFile(user, token, replaceData, downloadPath, data)
	if err != nil {
		FailTaskProcess(taskProcess)
		fmt.Println("Uplaod error: ", err)
		return fmt.Errorf("Upload error")
	}

	taskProcess.FileName = filename
	switch status {
	case "skipped":
		taskProcess.Status = models.TaskProcessStatusSkipped
	case "description_updated":
		taskProcess.Status = models.TaskProcessStatusDescriptionUpdated
	case "overwritten":
		taskProcess.Status = models.TaskProcessStatusOverwritten
	case "uploaded":
		taskProcess.Status = models.TaskProcessStatusUploaded
	default:
		taskProcess.Status = models.TaskProcessStatusFailed
	}

	taskProcess.Update()
	utils.SendWSTaskProcess(task.ID, taskProcess)

	return nil
}

func processCommonsTemplate(task *models.Task, user *models.User) {
	// Create template page in commons
	fmt.Print("============= GENERTING COMMONS TEMPLATE")
	wikiText, err := GetMapTemplate(task.ID)
	fmt.Println("GOT WIKITEXT: ", err)
	if err == nil {
		tokenResponse, err := utils.DoApiReq[TokenResponse](user, map[string]string{
			"action": "query",
			"meta":   "tokens",
			"format": "json",
		}, nil)

		if err != nil {
			fmt.Println("Error fetching edit token", err)
		} else if tokenResponse.Query.Tokens.CsrfToken != "" {
			token := tokenResponse.Query.Tokens.CsrfToken
			title, err := createCommonsTemplatePage(user, token, task.CommonsTemplateName, wikiText)
			if err == nil {
				task.CommonsTemplateName = title
				fmt.Print("=============== DONE CREATING COMMONS TEMPLATE")
			} else {
				fmt.Println("Error creating commons template page: ", err)
			}
		}
	} else {
		fmt.Println("Error getting task wikitext", task.ID, err)
	}

}

// processRegions runs every region. It returns an errIncomplete error naming the regions that
// could not be gone through (they have no year rows to show it); the other regions still run.
func processRegions(task *models.Task, user *models.User, tmpDir, title string, chartParamsMap map[string]string, data StartData) error {
	regionGroup, _ := errgroup.WithContext(context.Background())
	regionGroup.SetLimit(constants.CONCURRENT_REQUESTS)
	var failedMu sync.Mutex
	var failed []string

	for index, region := range constants.REGIONS {
		if task.Status != models.TaskStatusProcessing {
			break
		}
		region := region
		regionGroup.Go(func(region string, index int) func() error {
			return func() error {
				if task.Status != models.TaskStatusProcessing {
					return nil
				}
				time.Sleep(regionStagger * time.Duration(index))
				err := runRegion(user, task, task.ChartName, region, filepath.Join(tmpDir, region), chartParamsMap, title, data)
				fmt.Println("============= FINISHED PROCESSING REGION: ", region)
				if err != nil {
					fmt.Printf("ERROR task %s: region %s failed: %v\n", task.ID, region, err)
					failedMu.Lock()
					failed = append(failed, region)
					failedMu.Unlock()
				}
				return nil
			}
		}(region, index))
	}

	regionGroup.Wait()
	fmt.Println("================= FINISHED PROCESSING ALL REGIONS ==================")
	if len(failed) > 0 {
		return fmt.Errorf("regions %v: %w", failed, errIncomplete)
	}
	return nil
}

// runRegion processes one region and regionStagger spaces out the regions' starts (vars so
// tests can substitute them).
var (
	runRegion     = processRegion
	regionStagger = 3 * time.Second
)

// errIncomplete marks work that could not be done for part of a task and has no failed item
// rows to show it (a region or popover pass that never got going), so the task must end failed
// rather than done.
var errIncomplete = errors.New("part of the task could not be done")

func processCountries(chartInfo *ChartInfo, user *models.User, task *models.Task, url, tmpDir, title, startYear, endYear string, chartParamsMap map[string]string) error {
	if chartInfo.HasCountries {
		fmt.Println("======= Has Countries, using regular flow ========")

		tokenResponse, err := utils.DoApiReq[TokenResponse](user, map[string]string{
			"action": "query",
			"meta":   "tokens",
			"format": "json",
		}, nil)
		if err != nil {
			fmt.Println("Error fetching edit token", err)
			return err
		}
		token := tokenResponse.Query.Tokens.CsrfToken

		done := false

		defer func() {
			done = true
		}()

		go func() {
			for {
				time.Sleep(time.Second * 20)
				if task.Status != models.TaskStatusProcessing || done {
					break
				}
				tokenResponse, err := utils.DoApiReq[TokenResponse](user, map[string]string{
					"action": "query",
					"meta":   "tokens",
					"format": "json",
				}, nil)
				if err != nil {
					fmt.Println("Error fetching edit token", err)
				} else if tokenResponse.Query.Tokens.CsrfToken != "" {
					token = tokenResponse.Query.Tokens.CsrfToken
				}
			}
		}()

		fmt.Println("Countries:====================== ", chartInfo.CountriesList)

		countryGroup, _ := errgroup.WithContext(context.Background())
		countryGroup.SetLimit(constants.CONCURRENT_REQUESTS)

		countrySlices := utils.SplitSlice(chartInfo.CountriesList, constants.CONCURRENT_REQUESTS)
		startTime := time.Now()

		for _, countryList := range countrySlices {
			if task.Status != models.TaskStatusProcessing {
				break
			}
			countryList := countryList
			// countryList := make([]string, 0)
			// countryList = append(countryList, "NAM")
			countryGroup.Go(func(countryList []string) func() error {
				return func() error {
					countriesStartData := StartData{
						Url:                           url,
						FileName:                      task.CountryFileName,
						Description:                   task.CountryDescription,
						DescriptionOverwriteBehaviour: task.CountryDescriptionOverwriteBehaviour,
					}
					err := TraverseDownloadCountriesList(user, task, &token, task.ChartName, title, startYear, endYear, tmpDir, countriesStartData, chartParamsMap, countryList, chartInfo.CountryNames)

					if err != nil {
						fmt.Println("Error processing countries", err)
						return err
					}
					return nil
				}
			}(countryList))
		}
		if err := countryGroup.Wait(); err != nil {
			return err
		}

		elapsedTime := time.Since(startTime)
		fmt.Println("Started in", time.Since(startTime).String())
		fmt.Println("Finished in", elapsedTime.String())
	} else {
		fmt.Println("============= Doesn't have countries, downloading popup chart instead ===============")
		countriesDir := path.Join(tmpDir, "countries")
		err := os.Mkdir(countriesDir, 0755)
		if err == nil {
			if err := ProcessCountriesFromPopover(user, task, task.ChartName, title, startYear, endYear, countriesDir, StartData{
				Url:                           url,
				FileName:                      task.CountryFileName,
				Description:                   task.CountryDescription,
				DescriptionOverwriteBehaviour: task.CountryDescriptionOverwriteBehaviour,
			}, chartParamsMap); err != nil {
				// Regions still run; the task ends failed (errIncomplete)
				fmt.Println("Error processing popover countries: ", err)
				return err
			}
		} else {
			fmt.Println("Error creating countries directory: ", err)
			return fmt.Errorf("creating the countries dir: %v: %w", err, errIncomplete)
		}
	}

	return nil
}

type ChartParameter struct {
	Name        string                 `json:"name"`
	Slug        string                 `json:"slug"`
	Description string                 `json:"description"`
	Choices     []ChartParameterChoice `json:"choices"`
}

type ChartParameterChoice struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type ChartInfo struct {
	Params        *[]ChartParameter `json:"params"`
	ParamsMap     map[string]string `json:"paramsMap"`
	StartYear     string            `json:"startYear"`
	EndYear       string            `json:"endYear"`
	Title         string            `json:"title"`
	ChartName     string            `json:"chartName"`
	TemplateName  string            `json:"templateName"`
	HasCountries  bool              `json:"hasCountries"`
	CountriesList []string          `json:"countriesList"`
	CountryNames  map[string]string `json:"-"`
	StableUrl     string            `json:"stableUrl"`
	SingleImage   bool              `json:"singleImage"`
	Source        string            `json:"source"`
}

/*
* Chart title
* start/end year
* params // :DONE
* HasCountries
 */

// GetChartInfo reads a map chart's details in its own window of the shared browser.
func GetChartInfo(url, chartFormat, selectedParams string) (*ChartInfo, error) {
	chartInfo := ChartInfo{}
	var err error

	for i := 0; i < 2; i++ {
		err = rod.Try(func() {
			// A window per attempt; a browser that died is replaced when the next one opens
			w, err := openBrowserWindow()
			if err != nil {
				panic(err)
			}
			defer w.Close()
			page := w.Page
			time.Sleep(time.Second * time.Duration(i*5))
			page.MustSetViewport(constants.VIEWPORT_WIDTH, constants.VIEWPORT_HEIGHT, 1, false)
			page.MustSetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: env.GetEnv().OWID_UA})

			page.MustNavigate(url)
			page.MustWaitLoad()
			page.MustWaitIdle()

			if err := utils.WaitElementWithTimeout(page, DOWNLOAD_BUTTON_SELECTOR, time.Second*10); err != nil {
				fmt.Println("Timeout waiting for DOWNLOAD_BUTTON_SELECTOR")
				panic(err)
			}
			if err := utils.WaitElementWithTimeout(page, PLAY_TIMELAPSE_BUTTON_SELECTOR, time.Second*5); err != nil {
				fmt.Println("Timeout waiting for PLAY_TIMELAPSE_BUTTON_SELECTOR, might be a single image")
				chartInfo.HasCountries = false
				chartInfo.SingleImage = true

				chartInfo.Title = getMapTitleFromPage(page)
				chartInfo.Source = getMapSourceFromPage(page)
				if chartInfo.Title != "" {
					// for single image charts, we'll use the title as the chartName
					chartInfo.ChartName = chartInfo.Title
					chartInfo.TemplateName = GenerateTemplateCommonsNameNoPrefix(chartFormat, chartInfo.Title, chartInfo.ParamsMap)
				} else {
					chartName, err := GetChartNameFromUrl(url)
					if err == nil && chartName != "" {
						chartInfo.ChartName = utils.ToTitle(chartName)
						chartInfo.TemplateName = GenerateTemplateCommonsNameNoPrefix(chartFormat, chartName, chartInfo.ParamsMap)
					} else if chartInfo.Title != "" {
						chartInfo.ChartName = utils.ToTitle(chartInfo.Title)
						chartInfo.TemplateName = GenerateTemplateCommonsNameNoPrefix(chartFormat, chartInfo.Title, chartInfo.ParamsMap)
					}
				}
				_, err = TestDownloadPage(page)
				fmt.Println("CAN DOWNLOAD ERR", err)
				if err != nil {
					panic(err)
				}

				return
			}

			page = page.Timeout(constants.CHART_WAIT_TIME_SECONDS * time.Second)
			time.Sleep(time.Second * 1)

			_, err = GetPageCanDownload(page)
			fmt.Println("CAN DOWNLOAD ERR", err)
			if err != nil {
				panic(err)
			}

			chartInfo.Params = GetChartParametersFromPage(page)
			fmt.Println("GOT PARAMS")
			chartInfo.ParamsMap = GetChartParametersMapFromPage(page, selectedParams)
			fmt.Println("GOT PARAMSMAP")

			startYear, endYear, title := getMapStartEndYearTitleFromPage(page)
			fmt.Println("GOT title", startYear, endYear, title)
			chartInfo.StartYear = startYear
			chartInfo.EndYear = endYear
			chartInfo.Title = title
			chartInfo.Source = getMapSourceFromPage(page)
			fmt.Println("GOT Source", chartInfo.Source)
			hasCountries, countriesList, countryNames, err := getMapHasCountriesFromPage(page)
			if err != nil {
				panic(err)
			}
			chartInfo.HasCountries = hasCountries
			chartInfo.CountriesList = countriesList
			chartInfo.CountryNames = countryNames
			fmt.Println("GOT HasCountries", chartInfo.HasCountries)

			chartName, err := GetChartNameFromUrl(url)
			fmt.Println("GOT ChartName", chartName)

			if err == nil && chartName != "" {
				chartInfo.ChartName = utils.ToTitle(chartName)
				chartInfo.TemplateName = GenerateTemplateCommonsNameNoPrefix(chartFormat, chartName, chartInfo.ParamsMap)
			} else if chartInfo.Title != "" {
				chartInfo.ChartName = utils.ToTitle(chartInfo.Title)
				chartInfo.TemplateName = GenerateTemplateCommonsNameNoPrefix(chartFormat, chartInfo.Title, chartInfo.ParamsMap)
			}

			chartInfo.StableUrl = utils.CleanupTaskURLQueryParams(page.MustInfo().URL)
		})
		if err == nil {
			break
		}
	}

	return &chartInfo, err
}

func GetChartCountryYearsDataList(url string) (map[string][]time.Time, error) {
	newUrl := url
	urlParts := strings.Split(url, "?")

	newUrl = fmt.Sprintf("%s%s", urlParts[0], ".csv")
	if len(urlParts) > 1 {
		for i, part := range urlParts {
			if i == 0 {
				continue
			}
			newUrl = fmt.Sprintf("%s?%s", newUrl, part)
		}
	}

	resp, err := http.Get(newUrl)
	if err != nil {
		return nil, fmt.Errorf("fetching csv: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	r := csv.NewReader(resp.Body)
	r.ReuseRecord = true

	// Read header and locate the columns we need.
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("reading header: %w", err)
	}

	codeIdx, yearIdx := -1, -1
	for i, name := range header {
		switch name {
		case "Code":
			codeIdx = i
		case "Year":
			yearIdx = i
		}
	}
	if codeIdx == -1 || yearIdx == -1 {
		return nil, fmt.Errorf("missing Code or Year column in header: %v", header)
	}

	result := make(map[string][]time.Time)
	for {
		record, err := r.Read()
		if err != nil {
			break // io.EOF or a parse error ends the loop
		}
		code := record[codeIdx]
		if code == "" {
			continue // some rows (e.g. regions) have no code
		}
		date, err := utils.ParseDate(record[yearIdx])
		if err != nil {
			fmt.Println("Error parsing date: ", record[yearIdx], err)
		} else {
			result[code] = append(result[code], date)
		}
	}

	for code := range result {
		slices.SortFunc(result[code], func(a, b time.Time) int { return a.Compare(b) })
	}

	return result, nil
}

func TestDownloadPage(page *rod.Page) (bool, error) {
	defer func() {
		CloseDownloadPopup(page)
	}()

	if err := utils.WaitElementWithTimeout(page, DOWNLOAD_BUTTON_SELECTOR, time.Second*10); err != nil {
		return false, err
	}
	downloadBtn := page.MustElement(DOWNLOAD_BUTTON_SELECTOR)
	downloadBtn.MustFocus()
	time.Sleep(time.Millisecond * 200)
	fmt.Println("Focused download btn")

	err := page.Keyboard.Press(input.Enter)
	if err != nil {
		fmt.Println(err)
		return false, fmt.Errorf("Cannot find/click on download button")
	}

	// fmt.Println("CLICKED ENTER")
	if err := utils.WaitElementWithTimeout(page, DOWNLOAD_SVG_ICON_SELECTOR, time.Second*10); err != nil {
		return false, err
	}
	downloadIcon := page.MustElement(DOWNLOAD_SVG_BUTTON_SELECTOR)
	fmt.Println("GOT DOWNLOAD ICON", downloadIcon)
	if downloadIcon == nil {
		return false, fmt.Errorf("SVG Download icon not found")
	}

	return true, nil
}

func GetPageCanDownload(page *rod.Page) (bool, error) {
	mapTab, err := GetTabByLabel(page, "map")
	if err != nil {
		return false, err
	}
	if mapTab != nil {
		mapTab.Click(proto.InputMouseButtonLeft, 1)
		time.Sleep(time.Second)
	}

	startMarker := page.MustElement(START_MARKER_SELECTOR)
	endMarker := page.MustElement(END_MARKER_SELECTOR)
	fmt.Println("Getting page can download markers: ", startMarker, endMarker)
	if startMarker == nil && endMarker == nil {
		return false, fmt.Errorf("No start & end markers")
	}

	return TestDownloadPage(page)
}

func GetChartParametersFromPage(page *rod.Page) *[]ChartParameter {
	configJSON := ""

	var err error
	params := make([]ChartParameter, 0)

	configJSON = page.MustEval(`() => {
				if (window._OWID_MULTI_DIM_PROPS) {
					return JSON.stringify(window._OWID_MULTI_DIM_PROPS.configObj.dimensions);
 
				}
				return JSON.stringify([]);
			}`).String()

	err = json.Unmarshal([]byte(configJSON), &params)
	if err != nil {
		fmt.Println("Error parsing configObj.dimensions: ", err)
		return nil
	}

	return &params
}

func GetChartParameters(browser *rod.Browser, url string) *[]ChartParameter {
	configJSON := ""

	var err error
	params := make([]ChartParameter, 0)

	for trials := 0; trials < 2; trials++ {
		err = rod.Try(func() {
			page, err := openWindowPage(browser)
			if err != nil {
				panic(err)
			}
			defer closeWindowPage(browser, page)
			page.MustSetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: env.GetEnv().OWID_UA})
			page = page.Timeout(constants.CHART_WAIT_TIME_SECONDS * time.Second)

			page.MustNavigate(url)
			page.MustWaitIdle()
			page.MustWaitLoad()
			page.MustWaitElementsMoreThan(DOWNLOAD_BUTTON_SELECTOR, 0)
			configJSON = page.MustEval(`() => {
				if (window._OWID_MULTI_DIM_PROPS) {
					return JSON.stringify(window._OWID_MULTI_DIM_PROPS.configObj.dimensions);
 
				}
				return JSON.stringify([]);
			}`).String()
		})

		if err == nil {
			break
		}
	}

	if err != nil {
		fmt.Println("Error getting chart parameters: ", err)
		return nil
	}
	err = json.Unmarshal([]byte(configJSON), &params)
	if err != nil {
		fmt.Println("Error parsing configObj.dimensions: ", err)
		return nil
	}

	return &params
}

// traverseDownloadRegion downloads and uploads the region's years. A year that fails is marked
// failed and the rest go on; the error is for a region it could not go through at all (or only
// in part), which has no year rows to show it.
func traverseDownloadRegion(task *models.Task, data StartData, user *models.User, chartParams map[string]string, token *string, chartName, title, region, url, downloadPath string) error {
	regionStr := region
	if regionStr == "NorthAmerica" {
		regionStr = "North America"
	}
	if regionStr == "SouthAmerica" {
		regionStr = "South America"
	}

	// The worker's own window in the shared browser, released on every path
	w, err := openBrowserWindow()
	if err != nil {
		fmt.Println("ERROR opening browser window for region: ", region, err)
		return fmt.Errorf("region %s: %w", region, err)
	}
	defer w.Close()

	// Survives a rerun after a browser crash, like the year processes it skips
	triedUsingCommonsTemplate := false
	// The year being downloaded (a browser crash puts it back, failed, so the rerun retries it)
	// and the years given up after their retry, which later passes skip
	items := newPassItems("region "+region+" year", func(p *models.TaskProcess) string { return p.Date })
	// One pass walks the years from the latest; years already done are skipped, so a rerun in a
	// relaunched browser picks up where the crash left off without redoing or re-uploading.
	regionPass := func(p *rod.Page) error {
		page := p
		fmt.Println("==================== Traversing to: ", url)
		// Not ready (load timeout, controls missing): retried in a fresh window, never a silent stop
		if err := loadChartPage(page, url); err != nil {
			return fmt.Errorf("loading %s: %v: %w", url, err, errPageNotReady)
		}
		if err := waitRegionControls(page, region); err != nil {
			return err
		}

		startMarker, endMarker := timelineMarkers(page)
		if startMarker == nil && endMarker == nil {
			return fmt.Errorf("timeline of %s: %w", region, errPageNotReady)
		}
		if startMarker != nil || endMarker != nil {
			startYear := ""
			endYear := ""
			if startMarker != nil {
				if attr, err := startMarker.Attribute("aria-valuemin"); err == nil {
					startYear = *attr
				}
				// TODO
				if attr, err := startMarker.Attribute("aria-valuemax"); err == nil {
					endYear = *attr
				}
				// if _, err := startMarker.Attribute("aria-valuemax"); err == nil {
				// 	endYear = "2023"
				// }
			} else if endMarker != nil {
				if attr, err := endMarker.Attribute("aria-valuemin"); err == nil {
					startYear = *attr
				}
				// TODO
				if attr, err := endMarker.Attribute("aria-valuemax"); err == nil {
					endYear = *attr
				}
				// if _, err := endMarker.Attribute("aria-valuemax"); err == nil {
				// 	endYear = "2023"
				// }
			}

			fmt.Println("00000000000000000000000000 GOT START MARKER 00000000000000000000000000000", startYear, endYear)

			counter := 0
			owidEnv := env.GetEnv().OWID_ENV
			sources, err := GetTemplateExistingSources(user, task.CommonsTemplateName)
			regionExistingData := make(map[string]string, 0)
			if err == nil && sources != nil {
				data, exists := sources.Regions[region]
				if exists {
					regionExistingData = data
				}
			}

			// restartWindow replaces the worker's window (not the browser, so the other workers keep
			// going) and reopens the region's own URL in it; the loop then steps the timeline past
			// the years already done. Never the page's current URL: while the download dialog is
			// open it is ?overlay=download-vis, without the region (OWID drops region=World and
			// time=latest as defaults), and loading it opens the dialog again.
			restartWindow := func() bool {
				if err := w.Reopen(); err != nil {
					fmt.Println("ERROR reopening browser window for region: ", region, err)
					return false
				}
				page = w.Page
				if err := loadChartPage(page, url); err != nil {
					fmt.Println("ERROR reloading page for region: ", region, err)
					return false
				}
				return waitRegionControls(page, region) == nil
			}
			// The timeline values shown since the page was loaded. Each step moves the timeline,
			// so a value seen again means the loop is not getting anywhere: it would spin on the
			// same year for ever (values are compared as text; they need not be years).
			seen := map[string]bool{}

			for task.Status == models.TaskStatusProcessing {
				if owidEnv == "development" && counter >= 5 {
					// break
				}

				models.UpdateTaskLastOperationAt(task.ID)
				if counter >= regionWindowRestartEvery {
					// A fresh window every 50 files keeps the page's memory in check. It counts
					// files, not steps: walking past the years done after the restart must not
					// bring the next restart (a region with 50+ years done would never get on).
					if !restartWindow() {
						// The rerun reloads the region and goes on after the years done
						return fmt.Errorf("restarting the window for %s: %w", region, errPageNotReady)
					}
					counter = 0
					seen = map[string]bool{}
				}

				if err := utils.WaitElementWithTimeout(page, fmt.Sprintf("%s, %s", START_MARKER_SELECTOR, END_MARKER_SELECTOR), time.Second*5); err != nil {
					fmt.Println("ERROR waiting for EITHER START_MARKER_SELECTOR or END_MARKER_SELECTOR for region: ", region)
					return fmt.Errorf("timeline of %s: %w", region, errPageNotReady)
				}
				startMarker, endMarker = timelineMarkers(page)
				if startMarker == nil && endMarker == nil {
					return fmt.Errorf("timeline of %s: %w", region, errPageNotReady)
				}

				currentYear := timelineValue(startMarker, endMarker)
				year := currentYear
				if seen[year] {
					return fmt.Errorf("the timeline of %s shows %q again instead of moving on: %w", region, year, errPageNotReady)
				}
				seen[year] = true

				if items.skip(year) {
					// Failed after its retry; the rest of the years go on
					if moved, err := moveToNextYear(page, startMarker, endMarker, currentYear, startYear); err != nil {
						return err
					} else if moved {
						continue
					}
					break
				}

				replaceData := ReplaceVarsData{
					Url:      data.Url,
					Title:    title,
					Region:   regionStr,
					Year:     currentYear,
					FileName: GetFileNameFromChartName(chartName),
					Comment:  "Importing from " + data.Url,
					Params:   chartParams,
				}

				if task.DescriptionOverwriteBehaviour == models.DescriptionOverwriteBehaviourSkip && !triedUsingCommonsTemplate {
					fmt.Println("=============== HAS SKIP BEHAVIOUR")
					filename, exists := regionExistingData[currentYear]
					fmt.Println("EXISTS: ", regionStr, currentYear)
					if exists {
						startYear, _, _ := getMapStartEndYearTitleFromPage(page)
						fmt.Println("Start year: ", startYear, filename)
						if startYear != "" {
							// Update replacedata year to the startYear
							err := handleExistingMetadataCommonsFile(replaceData, regionExistingData, startYear, data, downloadPath, user, task, region, token)
							if err == nil {
								break
							} else {
								fmt.Println("=============== Error reusing commons template: ", err)
							}
							triedUsingCommonsTemplate = true
						}
					}
				}

				var taskProcess *models.TaskProcess

				existingTB, err := models.FindTaskProcessByTaskRegionDate(region, year, task.ID)
				if existingTB != nil {
					// Done, or failed on its own attempt in this run: stepped past
					if existingTB.Status != models.TaskProcessStatusFailed || !items.retryFailed(year) {
						if moved, err := moveToNextYear(page, startMarker, endMarker, currentYear, startYear); err != nil {
							return err
						} else if moved {
							continue
						}
						break
					}

					existingTB.Status = models.TaskProcessStatusProcessing
					if err := existingTB.Update(); err != nil {
						fmt.Println("Error updating task process to processing")
					}
					taskProcess = existingTB
				} else {
					taskProcess, err = models.NewTaskProcess(region, year, "", models.TaskProcessStatusProcessing, models.TaskProcessTypeMap, task.ID)
					if err != nil {
						return fmt.Errorf("recording %s %s: %w", region, year, err)
					}
				}

				items.start(taskProcess)
				counter = counter + 1
				mapPath := filepath.Join(downloadPath, currentYear)
				if err := utils.WaitElementWithTimeout(page, DOWNLOAD_BUTTON_SELECTOR, time.Second*5); err != nil {
					fmt.Println("ERROR waiting for DOWNLOAD_BUTTON_SELECTOR for region: ", region, currentYear)
					FailTaskProcess(taskProcess)
					return fmt.Errorf("download button for %s %s: %w", region, currentYear, errPageNotReady)
				}

				downloadBtn := page.MustElement(DOWNLOAD_BUTTON_SELECTOR)
				downloadBtn.MustFocus()
				time.Sleep(time.Millisecond * 200)
				// page.Keyboard.Press(input.Enter)
				err = page.Keyboard.Press(input.Enter)
				if err != nil {
					fmt.Println(fmt.Sprintf("%s %s %v", url, "Error clicking download button", err))
					FailTaskProcess(taskProcess)
					return fmt.Errorf("opening the download dialog for %s %s: %v: %w", region, currentYear, err, errPageNotReady)
				}

				if err := utils.WaitElementWithTimeout(page, DOWNLOAD_SVG_ICON_SELECTOR, time.Second*10); err != nil {
					fmt.Println("ERROR waiting for DOWNLOAD_SVG_ICON_SELECTOR for region: ", region, currentYear)
					FailTaskProcess(taskProcess)
					// moveToNextYear closes the download dialog first
					if moved, err := moveToNextYear(page, startMarker, endMarker, currentYear, startYear); err != nil {
						return err
					} else if moved {
						continue
					}
					break
				}
				time.Sleep(time.Millisecond * 200)

				svgButton := page.MustElements(DOWNLOAD_SVG_BUTTON_SELECTOR)[0]
				// A retry must not find an earlier attempt's file next to its own
				os.RemoveAll(mapPath)
				_, err = captureOWIDSVGToDir(page, mapPath, svgCaptureTimeout, clickElement(svgButton))
				if err != nil {
					// utils.SendWSProgress(session, taskProcess)
					fmt.Printf("%s, %s, %v", url, "Error downloading svg", err)
					FailTaskProcess(taskProcess)
					if !windowResponds(page) {
						// The window or the browser broke: runWithCrashRetry replaces it and
						// reruns the pass, which retries this year (years done are skipped)
						return fmt.Errorf("downloading %s %s: %w", region, currentYear, errWindowStuck)
					}
					// moveToNextYear closes the download dialog first
					if moved, err := moveToNextYear(page, startMarker, endMarker, currentYear, startYear); err != nil {
						return err
					} else if moved {
						continue
					}
					break
				}
				time.Sleep(time.Millisecond * 100)
				if _, err = os.Stat(mapPath); os.IsNotExist(err) {
					FailTaskProcess(taskProcess)
					// moveToNextYear closes the download dialog first
					if moved, err := moveToNextYear(page, startMarker, endMarker, currentYear, startYear); err != nil {
						return err
					} else if moved {
						continue
					}
					break
				}

				// Close download modal (checked again by moveToNextYear before the year moves)
				CloseDownloadPopup(page)

				fileInfo, err := getFileInfo(mapPath)
				if err != nil {
					FailTaskProcess(taskProcess)
					if moved, err := moveToNextYear(page, startMarker, endMarker, currentYear, startYear); err != nil {
						return err
					} else if moved {
						continue
					}
					break
				}

				lowerCaseContent := strings.ToLower(string(fileInfo.File))
				if strings.Contains(lowerCaseContent, "missing map column") {
					os.Remove(fileInfo.FilePath)
					fmt.Printf("Missing map column %s %s %s, retrying", regionStr, currentYear, GetFileNameFromChartName(chartName))
					FailTaskProcess(taskProcess)
					if moved, err := moveToNextYear(page, startMarker, endMarker, currentYear, startYear); err != nil {
						return err
					} else if moved {
						continue
					}
					break
				}

				// Save the countries fill data
				countryFlls, err := svgprocessor.ExtractCountryFills(fileInfo.FilePath)
				if err != nil {
					fmt.Println("Error extracting country fills ", err)
				} else {
					jsonStr, err := svgprocessor.ConvertToJSON(countryFlls)
					if jsonStr != "" && err == nil {
						taskProcess.FillData = jsonStr
						taskProcess.Update()
					}
				}

				// Collect metadata and inject it if at last file
				if didReachStartYear(startMarker, endMarker, startYear) {
					metadata, err := getRegionFileMetadata(task, region)
					if err != nil {
						fmt.Println("Error generating metadata: ", err)
					} else if metadata != "" {
						if err := InjectMetadataIntoSVGSameFile(fileInfo.FilePath, metadata); err != nil {
							fmt.Println("Error injecting metadata into svg: ", err)
						} else {
							replaceData.Comment = "Importing from " + data.Url + " with metadata"
						}
					}
				}

				Filename, status, err := uploadMapFile(user, *token, replaceData, mapPath, data)
				//  Retry twice
				if err != nil {
					taskProcess.Status = models.TaskProcessStatusRetrying
					taskProcess.Update()
					utils.SendWSTaskProcess(task.ID, taskProcess)

					time.Sleep(time.Second * 2)
					Filename, status, err = uploadMapFile(user, *token, replaceData, mapPath, data)
					if err != nil {
						taskProcess.Status = models.TaskProcessStatusRetrying
						taskProcess.Update()
						utils.SendWSTaskProcess(task.ID, taskProcess)

						time.Sleep(time.Second * 4)
						Filename, status, err = uploadMapFile(user, *token, replaceData, mapPath, data)
					}
				}

				if err != nil {
					fmt.Println("Error processing", region, year)
					FailTaskProcess(taskProcess)
				} else {
					taskProcess.FileName = Filename

					switch status {
					case "skipped":
						taskProcess.Status = models.TaskProcessStatusSkipped
					case "description_updated":
						taskProcess.Status = models.TaskProcessStatusDescriptionUpdated
					case "overwritten":
						taskProcess.Status = models.TaskProcessStatusOverwritten
					case "uploaded":
						taskProcess.Status = models.TaskProcessStatusUploaded
					}

					taskProcess.Update()
					utils.SendWSTaskProcess(task.ID, taskProcess)
					// Only a year done in full resets the window's stuck-retry count: a capture
					// whose upload then fails must not let a stuck window retry for ever
					w.itemDone()
				}

				if moved, err := moveToNextYear(page, startMarker, endMarker, currentYear, startYear); err != nil {
					return err
				} else if !moved {
					break
				}
			}
		}
		return nil
	}
	// Fails the year left "processing" by a crash, a stuck window, an error or an early stop
	if err := w.runItems("region "+region, regionPass, items.onFail, items.giveUp); err != nil {
		fmt.Println("ERROR processing region: ", region, err)
		return fmt.Errorf("region %s: %w", region, err)
	}
	return nil
}

// waitRegionControls waits for the region page's download button, timeline play button and
// markers. Missing ones are errPageNotReady: retried in a fresh window, never a silent stop.
func waitRegionControls(page *rod.Page, region string) error {
	for _, c := range []struct {
		sel  string
		wait time.Duration
	}{
		{DOWNLOAD_BUTTON_SELECTOR, 10 * time.Second},
		{PLAY_TIMELAPSE_BUTTON_SELECTOR, 5 * time.Second},
		{fmt.Sprintf("%s, %s", START_MARKER_SELECTOR, END_MARKER_SELECTOR), 5 * time.Second},
	} {
		if err := utils.WaitElementWithTimeout(page, c.sel, c.wait); err != nil {
			fmt.Println("ERROR waiting for", c.sel, "for region:", region)
			return fmt.Errorf("%s not shown for region %s: %w", c.sel, region, errPageNotReady)
		}
	}
	return nil
}

// timelineMarkers returns the timeline's start and end markers, nil for one that is missing
// (MustElement would wait for it forever).
func timelineMarkers(page *rod.Page) (start, end *rod.Element) {
	if has, el, err := page.Has(START_MARKER_SELECTOR); err == nil && has {
		start = el
	}
	if has, el, err := page.Has(END_MARKER_SELECTOR); err == nil && has {
		end = el
	}
	return start, end
}

// itemUnfinished is an item a worker may still give up on: neither done nor skipped.
func itemUnfinished(p *models.TaskProcess) bool {
	return p.Status == models.TaskProcessStatusProcessing || p.Status == models.TaskProcessStatusFailed ||
		p.Status == models.TaskProcessStatusRetrying
}

// passItems is a worker's bookkeeping for runItems over all its passes (reruns in a fresh
// window, window restarts): the item (year or country row) being worked on, the items tried in
// this run, the one item a retry redoes, and the items given up after their retry, which later
// passes skip.
type passItems struct {
	what       string
	key        func(*models.TaskProcess) string
	inProgress *models.TaskProcess
	givenUp    map[string]bool
	// attempted are the items worked on in this run. One that ended failed failed on its own
	// attempt: a later pass steps past it like an item done, so a reload never retries the same
	// failing items over and over and never gets to the rest (a failed row from an earlier run
	// is still retried, once).
	attempted map[string]bool
	// redo is the item a rerun after a stuck window or a dead browser works on again (once).
	redo string
}

func newPassItems(what string, key func(*models.TaskProcess) string) *passItems {
	return &passItems{what: what, key: key, givenUp: map[string]bool{}, attempted: map[string]bool{}}
}

// start records the item now being worked on.
func (p *passItems) start(tp *models.TaskProcess) {
	p.inProgress = tp
	p.attempted[p.key(tp)] = true
}

// skip reports an item given up earlier in this run.
func (p *passItems) skip(key string) bool { return p.givenUp[key] }

// retryFailed reports whether a pass works on an item whose row is failed again: one not tried
// in this run yet, or the item a rerun redoes (once). Any other failed item failed on its own
// attempt in this run and is stepped past.
func (p *passItems) retryFailed(key string) bool {
	if !p.attempted[key] {
		return true
	}
	if p.redo == key {
		p.redo = ""
		return true
	}
	return false
}

// onFail fails the item left "processing" by a crash, a stuck window, an error or an early stop,
// and marks the item being worked on for the rerun (if any) to redo.
func (p *passItems) onFail() {
	if p.inProgress == nil {
		return
	}
	if p.inProgress.Status == models.TaskProcessStatusProcessing {
		FailTaskProcess(p.inProgress)
	}
	p.redo = p.key(p.inProgress)
}

// giveUp leaves the item that got stuck again failed, for the next pass to go on after it.
// False when no unfinished item is in progress or it was given up already.
func (p *passItems) giveUp() bool {
	if p.inProgress == nil || p.givenUp[p.key(p.inProgress)] || !itemUnfinished(p.inProgress) {
		return false
	}
	k := p.key(p.inProgress)
	fmt.Println("ERROR giving up on", p.what, k, "after its retry; going on with the rest")
	if p.inProgress.Status != models.TaskProcessStatusFailed {
		FailTaskProcess(p.inProgress)
	}
	p.givenUp[k] = true
	p.inProgress = nil
	return true
}

// loadChartPage opens url in a worker's window, bounded by windowLoadTimeout.
func loadChartPage(page *rod.Page, url string) error {
	return withoutTryStack(rod.Try(func() {
		p := page.Timeout(windowLoadTimeout)
		defer p.CancelTimeout()
		p.MustSetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: env.GetEnv().OWID_UA})
		p.MustSetViewport(constants.VIEWPORT_WIDTH, constants.VIEWPORT_HEIGHT, 1, false)
		p.MustNavigate(url)
		p.MustWaitLoad()
		p.MustWaitIdle()
	}))
}

// withoutTryStack turns a rod.Try error from a failed Must call (a load timeout, a navigation
// error) into that one-line error. rod.TryError's message carries the goroutine stack, which
// made such logs look like panics. Real panics (runtime errors, non-error values) keep it.
func withoutTryStack(err error) error {
	var tryErr *rod.TryError
	if !errors.As(err, &tryErr) {
		return err
	}
	cause, ok := tryErr.Value.(error)
	if !ok {
		return err
	}
	var runtimeErr runtime.Error
	if errors.As(cause, &runtimeErr) {
		return err
	}
	return cause
}

func handleExistingMetadataCommonsFile(replaceData ReplaceVarsData, regionExistingData map[string]string, startYear string, data StartData, downloadPath string, user *models.User, task *models.Task, region string, token *string) error {
	replaceData.Year = startYear
	filename := replaceVars(data.FileName, replaceData)
	existingMapPath := filepath.Join(downloadPath, "_existing_final")
	existingMapFilePath := path.Join(existingMapPath, "image.svg")
	if err := os.Mkdir(existingMapPath, 0755); err != nil {
		return err
	}

	err := downloadCommonsFile(filename, existingMapFilePath, user)
	if err != nil {
		return err
	}

	/**
		Check if the old file has metadata. If so, we'll append the new files
		metadata to it and reupload the file. If not, skip all of this and the flow will continue to
		collect the metadata fields
	**/

	content, err := os.ReadFile(existingMapFilePath)
	if err != nil {
		return err
	}
	if len(content) == 0 {
		return fmt.Errorf("Empty file content")
	}

	fmt.Println("Existing commons file: ", existingMapFilePath)
	// By here, we have existing task processes and the fills from the old metadata file

	metadataFills, err := parseSVGMetadata(string(content))
	if err != nil {
		return err
	}
	if len(metadataFills) == 0 {
		return fmt.Errorf("Empty fills")
	}

	// Get the fills of the processed task processes, those are newly added years
	newFills, err := getRegionFills(task, region)
	if err != nil {
		return err
	}

	backfills := utils.DiffSliceBy(metadataFills, *newFills, func(a CountryFillWithYear) string {
		return fmt.Sprintf("%s%s", a.Year, a.Country)
	})

	// We need to combine both metadata
	allFills := append(backfills, *newFills...)

	// Convert fills to SVG metadata element
	metadata := generateSVGMetadataFromFills(allFills)
	if err := InjectMetadataIntoSVGSameFile(existingMapFilePath, metadata); err != nil {
		return fmt.Errorf("Error injecting metadata into svg: %w", err)
	}

	replaceData.Comment = "Importing from " + data.Url + " with metadata"
	Filename, status, err := uploadMapFile(user, *token, replaceData, existingMapPath, data)
	//  Retry twice
	if err != nil {
		time.Sleep(time.Second * 2)
		Filename, status, err = uploadMapFile(user, *token, replaceData, existingMapPath, data)
		if err != nil {
			time.Sleep(time.Second * 4)
			Filename, status, err = uploadMapFile(user, *token, replaceData, existingMapPath, data)
			if err != nil {
				return err
			}
		}
	}
	fmt.Println("Filename: ", Filename, status)
	/**
		We need to backfill the database with the tasks we got from metadata that we didn't import
	**/

	taskProcesses, err := models.FindTaskProcessesByTaskIdAndRegion(task.ID, region)
	if err == nil {
		existingDates := make(map[string]bool)
		for _, tp := range taskProcesses {
			existingDates[tp.Date] = true
		}

		for date, filename := range regionExistingData {
			_, exists := existingDates[date]
			if !exists {
				taskProcess, err := models.NewTaskProcess(region, date, filename, models.TaskProcessStatusSkipped, models.TaskProcessTypeMap, task.ID)
				if err == nil {
					utils.SendWSTaskProcess(task.ID, taskProcess)
				}
			}
		}
	}

	return nil
}

func didReachStartYear(startMarker, endMarker *rod.Element, startYear string) bool {
	if (startMarker != nil && *startMarker.MustAttribute("aria-valuenow") == startYear) || (endMarker != nil && *endMarker.MustAttribute("aria-valuenow") == startYear) {
		fmt.Println("REACHING START YEAR")
		return true
	}
	return false
}

// regionWindowRestartEvery is how many files a region worker downloads in one window before
// it moves to a fresh one (a var so tests can shorten it).
var regionWindowRestartEvery = 50

// yearMoveTimeout bounds waiting for the timeline to show another value after a step (a var
// so tests can shorten it).
var yearMoveTimeout = 5 * time.Second

// moveToNextYear steps the timeline back one value. It returns false when the timeline already
// shows startYear (nothing left). It closes the download dialog first (an open dialog keeps the
// focus, so the timeline never gets the key) and checks the timeline moved: one that stays put
// is an errPageNotReady error, retried in a fresh window, never a loop that reads the same
// year again. Values are compared as text: OWID's timeline need not show years.
func moveToNextYear(page *rod.Page, startMarker, endMarker *rod.Element, currentYear, startYear string) (bool, error) {
	if didReachStartYear(startMarker, endMarker, startYear) {
		return false, nil
	}

	for attempt := 0; attempt < 2; attempt++ {
		if err := CloseDownloadPopup(page); err != nil {
			return false, err
		}
		if attempt > 0 {
			// A step that landed late counts: never step twice
			if v := readTimelineValue(page); v != "" && v != currentYear {
				return true, nil
			}
			startMarker, endMarker = timelineMarkers(page)
		}
		pressTimelineLeft(page, startMarker, endMarker)
		if waitTimelineMoved(page, currentYear, yearMoveTimeout) {
			return true, nil
		}
		fmt.Println("ERROR the timeline did not move from", currentYear, "attempt", attempt+1)
	}
	return false, fmt.Errorf("the timeline stayed at %q after stepping back: %w", currentYear, errPageNotReady)
}

// pressTimelineLeft focuses each timeline marker and presses ArrowLeft on it.
func pressTimelineLeft(page *rod.Page, startMarker, endMarker *rod.Element) {
	for _, marker := range []*rod.Element{startMarker, endMarker} {
		if marker == nil {
			continue
		}
		time.Sleep(time.Millisecond * 100)
		marker.Focus()
		time.Sleep(time.Millisecond * 100)
		page.Keyboard.Press(input.ArrowLeft)
		time.Sleep(time.Millisecond * 100)
		marker.Blur()
	}
}

// timelineValue is the value the timeline markers show (aria-valuenow, the end marker's when
// both have one), as text.
func timelineValue(startMarker, endMarker *rod.Element) string {
	value := ""
	for _, marker := range []*rod.Element{startMarker, endMarker} {
		if marker == nil {
			continue
		}
		if attr, err := marker.Attribute("aria-valuenow"); err == nil && attr != nil && *attr != "" {
			value = *attr
		}
	}
	return value
}

// readTimelineValue reads the timeline's value from page now, "" when it can't (bounded).
func readTimelineValue(page *rod.Page) string {
	p := page.Timeout(2 * time.Second)
	defer p.CancelTimeout()
	return timelineValue(timelineMarkers(p))
}

// waitTimelineMoved waits up to d for the timeline to show a value other than from.
func waitTimelineMoved(page *rod.Page, from string, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for {
		if v := readTimelineValue(page); v != "" && v != from {
			return true
		}
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func getMapHasCountriesFromPage(page *rod.Page) (bool, []string, map[string]string, error) {
	activeTab, _ := GetActivePageTab(page)
	defer func() {
		if activeTab != nil {
			activeTab.Click(proto.InputMouseButtonLeft, 1)
			time.Sleep(time.Millisecond * 200)
		}
	}()
	countriesList := make([]string, 0)
	countryNames := make(map[string]string)
	hasLines := false

	lineTab, _ := GetTabByLabel(page, "line")
	chartTab, _ := GetTabByLabel(page, "chart")
	if lineTab != nil {
		lineTab.Click(proto.InputMouseButtonLeft, 1)
		time.Sleep(time.Second)
		hasLines = page.MustHas(".entity-selector .entity-section, .EntityPicker .EntityList")
	} else if chartTab != nil {
		chartTab.Click(proto.InputMouseButtonLeft, 1)
		time.Sleep(time.Second)
		hasLines = page.MustHas("svg .LineChart")
	}

	if hasLines {
		var err error
		countriesList, countryNames, err = discoverChartCountries(page.MustInfo().URL, func() ([]string, map[string]string, error) {
			return getCountryNamesFromPage(page)
		})
		if err != nil {
			return false, nil, nil, err
		}
	}

	return hasLines, countriesList, countryNames, nil
}

func getMapStartEndYearTitleFromPage(page *rod.Page) (string, string, string) {
	startYear := ""
	endYear := ""
	title := ""

	marker := page.MustElement(START_MARKER_SELECTOR)
	startYear = *marker.MustAttribute("aria-valuemin")
	// TODO
	endYear = *marker.MustAttribute("aria-valuemax")
	// endYear = "2023"
	title = getMapTitleFromPage(page)
	suffix := ", " + endYear
	if strings.HasSuffix(title, suffix) {
		title = strings.ReplaceAll(title, suffix, "")
	}

	return startYear, endYear, title
}

func getMapTitleFromPage(page *rod.Page) string {
	for _, selector := range strings.Split(TITLE_SELECTOR, ",") {
		selector := strings.TrimSpace(selector)
		if el, err := page.Element(selector); err == nil && el != nil {
			title := el.MustText()
			title = strings.TrimSpace(title)
			return title
		}

	}

	return ""
}

func getMapSourceFromPage(page *rod.Page) string {
	// The "Source" entry in the key-data section below the chart
	if titles, err := page.Elements(SOURCE_KEY_DATA_TITLE_SELECTOR); err == nil {
		for _, titleEl := range titles {
			if text, err := titleEl.Text(); err != nil || strings.TrimSpace(text) != "Source" {
				continue
			}
			if sibling, err := titleEl.Next(); err == nil && sibling != nil {
				if source, err := sibling.Text(); err == nil && strings.TrimSpace(source) != "" {
					return strings.TrimSpace(source)
				}
			}
		}
	}

	// Fallback: the "Data source:" line in the chart footer
	if lines, err := page.Elements(SOURCE_FOOTER_LINE_SELECTOR); err == nil {
		for _, line := range lines {
			text, err := line.Text()
			if err != nil {
				continue
			}
			text = strings.TrimSpace(text)
			if !strings.HasPrefix(text, "Data source:") {
				continue
			}
			if source := strings.TrimSpace(strings.TrimPrefix(text, "Data source:")); source != "" {
				return source
			}
		}
	}

	return ""
}

func processRegion(user *models.User, task *models.Task, chartName string, region, downloadPath string, chartParamsMap map[string]string, title string, data StartData) error {
	token := ""
	done := false

	defer func() {
		done = true
	}()

	go func() {
		for !done {
			tokenResponse, err := utils.DoApiReq[TokenResponse](user, map[string]string{
				"action": "query",
				"meta":   "tokens",
				"format": "json",
			}, nil)
			if err != nil {
				fmt.Println("Error fetching edit token", err)
			} else if tokenResponse.Query.Tokens.CsrfToken != "" {
				token = tokenResponse.Query.Tokens.CsrfToken
			}

			time.Sleep(time.Second * 20)
		}
	}()

	for token == "" {
		time.Sleep(time.Second)
		// fmt.Println("Waiting for token")
	}

	// Get start and end years
	// get chart title
	// Process each year
	// utils.SendWSMessage(session, "debug", fmt.Sprintf("%s:processing", region))
	fmt.Println("Processing region: ", region, downloadPath)
	_, errExists := os.Stat(downloadPath)
	if os.IsNotExist(errExists) {
		os.Mkdir(downloadPath, 0755)
	}

	url := data.Url
	url = utils.AttachQueryParamToUrl(url, fmt.Sprintf("tab=map&region=%s", region))

	if task.ChartParameters != "" {
		url = utils.AttachQueryParamToUrl(url, task.ChartParameters)
	}

	// TODO
	url = utils.AttachQueryParamToUrl(url, "time=latest")
	// url = utils.AttachQueryParamToUrl(url, "time=2023")

	err := traverseDownloadRegion(task, data, user, chartParamsMap, &token, chartName, title, region, url, downloadPath)
	task.Reload()

	return err
}

type CountryFillWithYear struct {
	Country    string
	Fill       string
	Year       string
	ActualYear string
}

type svgCountryDataMetadata struct {
	XMLName xml.Name             `xml:"metadata"`
	ID      string               `xml:"id,attr"`
	Years   []svgCountryDataYear `xml:"years>year"`
}

type svgCountryDataYear struct {
	Value     string                  `xml:"value,attr"`
	Countries []svgCountryDataCountry `xml:"country"`
}

type svgCountryDataCountry struct {
	Name string `xml:"name,attr"`
	Fill string `xml:"fill,attr"`
}

func getRegionFills(task *models.Task, region string) (*[]CountryFillWithYear, error) {
	url := utils.AttachQueryParamToUrl(task.URL, "tab=map")

	if task.ChartParameters != "" {
		url = utils.AttachQueryParamToUrl(url, task.ChartParameters)
	}

	data, err := GetChartCountryYearsDataList(url)
	if err != nil {
		fmt.Println("Error getting chart country year data list: ", err)
	}

	taskProcesses, err := models.FindTaskProcessesByTaskIdAndRegion(task.ID, region)
	if err != nil {
		return nil, err
	}

	fills := make([]CountryFillWithYear, 0)
	for _, tp := range taskProcesses {
		if tp.FillData != "" {
			countriesData, err := svgprocessor.ParseJSONString(tp.FillData)
			if err != nil {
				fmt.Println("Error parsing countries fillData", tp, err)
				continue
			}
			for _, countryData := range countriesData {
				fill := CountryFillWithYear{
					Country: countryData.Country,
					Fill:    countryData.Fill,
					Year:    tp.Date,
				}
				if !strings.Contains(strings.ToLower(countryData.Fill), "nodata") {
					if countryCode, hasCode := constants.COUNTRY_CODES[countryData.Country]; hasCode {
						if dates, ok := data[countryCode]; ok {
							countryDate, err := utils.ParseDate(tp.Date)
							if err != nil {
								fmt.Println("Error parsing countryDate: ", err, tp.Date)
							} else if idx := utils.NearestIndex(dates, countryDate, 10); idx >= 0 && !dates[idx].Equal(countryDate) {
								// Substituted: the displayer showed a neighboring date's value.
								fill.ActualYear = utils.FormatDateLike(dates[idx], tp.Date)
							}
						}
					}
				}
				fills = append(fills, fill)
			}
		}
	}

	return &fills, nil
}

func getRegionFileMetadata(task *models.Task, region string) (string, error) {
	fills, err := getRegionFills(task, region)
	if err != nil {
		return "", err
	}

	// Convert metadata to SVG metadata element
	svgContent := generateSVGMetadataFromFills(*fills)
	return svgContent, nil
}

// Generate SVG metadata with data grouped by years
func generateSVGMetadataFromFills(metadata []CountryFillWithYear) string {
	var svgBuilder strings.Builder

	// Group data by year
	dataByYear := make(map[string][]CountryFillWithYear)
	for _, data := range metadata {
		dataByYear[data.Year] = append(dataByYear[data.Year], data)
	}

	// Create a metadata group
	svgBuilder.WriteString(`<metadata id="country-data">`)
	svgBuilder.WriteString("\n")

	// Embed data grouped by years
	svgBuilder.WriteString(`  <years>`)
	svgBuilder.WriteString("\n")

	// Get all years and sort them
	years := make([]string, 0, len(dataByYear))
	for year := range dataByYear {
		years = append(years, year)
	}
	// sort.Ints(years) // Sort years for consistent output
	sort.SliceStable(years, func(i, j int) bool {
		date1, err := utils.ParseDate(years[i])
		if err != nil {
			return false
		}
		date2, err := utils.ParseDate(years[j])
		if err != nil {
			return false
		}

		return date1.UnixMilli() < date2.UnixMilli()
	})

	// Create elements for each year with its countries
	for _, year := range years {
		countriesForYear := dataByYear[year]

		svgBuilder.WriteString(fmt.Sprintf(`    <year value="%s">`, year))
		svgBuilder.WriteString("\n")

		// Add countries for this year
		for _, data := range countriesForYear {
			if data.ActualYear == "" {
				svgBuilder.WriteString(fmt.Sprintf(`      <country name="%s" fill="%s" />`,
					html.EscapeString(data.Country),
					html.EscapeString(data.Fill)))
			} else {
				svgBuilder.WriteString(fmt.Sprintf(`      <country name="%s" fill="%s" actual="%s" />`,
					html.EscapeString(data.Country),
					html.EscapeString(data.Fill),
					html.EscapeString(data.ActualYear),
				),
				)

			}
			svgBuilder.WriteString("\n")
		}

		svgBuilder.WriteString("    </year>")
		svgBuilder.WriteString("\n")
	}

	svgBuilder.WriteString("  </years>")
	svgBuilder.WriteString("\n")

	svgBuilder.WriteString("</metadata>")
	svgBuilder.WriteString("\n")

	return svgBuilder.String()
}

func parseSVGMetadata(content string) ([]CountryFillWithYear, error) {
	reMetadata := regexp.MustCompile(`(?s)<metadata\b[^>]*>.*?</metadata>`)
	metadataBlocks := reMetadata.FindAllString(content, -1)

	if len(metadataBlocks) == 0 {
		return nil, fmt.Errorf("metadata tag not found")
	}

	var parseErrors []error

	for _, metadataBlock := range metadataBlocks {
		metadata, err := parseSVGMetadataBlock(metadataBlock)
		if err != nil {
			parseErrors = append(parseErrors, err)
			continue
		}

		if metadata.ID != "country-data" {
			continue
		}

		parsedData := flattenSVGMetadata(metadata)
		if len(parsedData) == 0 {
			return nil, fmt.Errorf("country-data metadata is empty")
		}

		return parsedData, nil
	}

	if len(parseErrors) > 0 {
		return nil, fmt.Errorf("metadata found, but country-data could not be parsed: %w", parseErrors[len(parseErrors)-1])
	}

	return nil, fmt.Errorf("metadata tag does not contain country-data")
}

// func parseSVGMetadata(content string) ([]CountryFillWithYear, error) {
// 	reMetadata := regexp.MustCompile(`(?is)<metadata\b[^>]*>.*?</metadata>`)
// 	metadataBlocks := reMetadata.FindAllString(content, -1)
// 	if len(metadataBlocks) == 0 {
// 		return nil, fmt.Errorf("metadata tag not found")
// 	}

// 	var fallback []CountryFillWithYear
// 	var lastParseErr error

// 	for _, metadataBlock := range metadataBlocks {
// 		metadata, err := parseSVGMetadataBlock(metadataBlock)
// 		if err != nil {
// 			lastParseErr = err
// 			continue
// 		}

// 		parsedData := flattenSVGMetadata(metadata)
// 		if metadata.ID == "country-data" {
// 			return parsedData, nil
// 		}

// 		if len(parsedData) > 0 && fallback == nil {
// 			fallback = parsedData
// 		}
// 	}

// 	if fallback != nil {
// 		return fallback, nil
// 	}

// 	if lastParseErr != nil {
// 		return nil, fmt.Errorf("error parsing metadata: %w", lastParseErr)
// 	}

// 	return nil, fmt.Errorf("metadata tag does not contain country data")
// }

func parseSVGMetadataBlock(metadataBlock string) (svgCountryDataMetadata, error) {
	var metadata svgCountryDataMetadata
	if err := xml.Unmarshal([]byte(metadataBlock), &metadata); err != nil {
		return metadata, err
	}

	return metadata, nil
}

func flattenSVGMetadata(metadata svgCountryDataMetadata) []CountryFillWithYear {
	result := make([]CountryFillWithYear, 0)
	for _, year := range metadata.Years {
		for _, country := range year.Countries {
			result = append(result, CountryFillWithYear{
				Country: country.Name,
				Fill:    country.Fill,
				Year:    year.Value,
			})
		}
	}

	return result
}

// InjectMetadataIntoSVGSameFile injects metadata into an SVG file by modifying and overwriting it
// Places metadata at the end of the file, just before the closing </svg> tag
func InjectMetadataIntoSVGSameFile(filePath string, metadataString string) error {
	// Read the original SVG file
	svgData, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("error reading SVG file: %w", err)
	}

	// Convert to string for manipulation
	svgContent := string(svgData)

	// Check if SVG already has a metadata element
	reMetadata := regexp.MustCompile(`<metadata[^>]*>[\s\S]*?</metadata>`)
	if reMetadata.MatchString(svgContent) {
		// Replace existing metadata
		svgContent = reMetadata.ReplaceAllString(svgContent, metadataString)
	} else {
		// Add metadata just before the closing </svg> tag
		closingSvgTag := "</svg>"
		lastIndex := strings.LastIndex(svgContent, closingSvgTag)

		if lastIndex == -1 {
			return fmt.Errorf("could not find closing </svg> tag in file")
		}

		// Insert metadata before closing tag
		svgContent = svgContent[:lastIndex] + "\n" + metadataString + "\n" + svgContent[lastIndex:]
	}

	// Write the modified content back to the same file
	err = os.WriteFile(filePath, []byte(svgContent), 0644)
	if err != nil {
		return fmt.Errorf("error writing to file: %w", err)
	}

	return nil
}
