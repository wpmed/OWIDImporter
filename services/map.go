package services

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
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

	l, browser := GetBrowser()
	_ = browser.MustPage("")
	chartInfo, err := GetChartInfo(browser, url, data.TemplateNameFormat, task.ChartParameters)
	if err != nil {
		fmt.Println("Error getting chart info: ", err)
		task.Status = models.TaskStatusFailed
		task.Update()
		utils.SendWSTask(task)
		browser.Close()
		l.Cleanup()
		return fmt.Errorf("Error getting chart info")
	}

	browser.Close()
	l.Cleanup()

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

	if task.ImportCountries == 1 && task.Status == models.TaskStatusProcessing {
		fmt.Print("================= STARTED IMPORTING COUNTRIES")
		if err := processCountries(chartInfo, user, task, data.Url, tmpDir, title, startYear, endYear, chartParamsMap); err != nil {
			task.Status = models.TaskStatusFailed
			task.Update()
			utils.SendWSTask(task)
			return err
		}
	}

	if task.Status == models.TaskStatusProcessing {
		// Process regions
		processRegions(task, user, tmpDir, title, chartParamsMap, data)
	}

	if task.Status == models.TaskStatusProcessing {
		if data.GenerateTemplateCommons {
			processCommonsTemplate(task, user)
		}

		task.Status = models.TaskStatusDone
		if err := task.Update(); err != nil {
			fmt.Println("Error saving task staus to done: ", err)
		}
	}

	utils.SendWSTask(task)

	return nil
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

	l, browser := GetBrowser()
	blankPage := browser.MustPage("")

	defer blankPage.Close()
	defer l.Cleanup()
	defer browser.Close()

	page := browser.MustPage("")
	page.MustSetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: env.GetEnv().OWID_UA})
	page.MustSetViewport(constants.VIEWPORT_WIDTH, constants.VIEWPORT_HEIGHT, 1, false)
	page.MustNavigate(task.URL)
	// utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:processing", country))
	page.MustWaitLoad()
	page.MustWaitIdle()
	if err := utils.WaitElementWithTimeout(page, DOWNLOAD_BUTTON_SELECTOR, time.Second*10); err != nil {
		return fmt.Errorf("Cannot find download button in page")
	}

	wait := page.Browser().WaitDownload(downloadPath)
	downloadBtn := page.MustElement(DOWNLOAD_BUTTON_SELECTOR)
	downloadBtn.MustFocus()
	time.Sleep(time.Millisecond * 200)

	if err := page.Keyboard.Press(input.Enter); err != nil {
		FailTaskProcess(taskProcess)
		return fmt.Errorf("Error clicking download button")
	}

	fmt.Println("GOT DOWNLOAD BTN SELECTOR, WAITING FOR SVG")
	if err := utils.WaitElementWithTimeout(page, DOWNLOAD_SVG_ICON_SELECTOR, time.Second*10); err != nil {
		FailTaskProcess(taskProcess)
		CloseDownloadPopup(page)
		return fmt.Errorf("Can't find DOWNLOAD_SVG_SELECTOR")
	}

	elements := page.MustElements(DOWNLOAD_SVG_ICON_SELECTOR)

	if err := elements[0].Click(proto.InputMouseButtonLeft, 1); err != nil {
		// utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:failed", country))
		FailTaskProcess(taskProcess)
		CloseDownloadPopup(page)
		return fmt.Errorf("Error clicking download svg button")
	}

	wait()
	fmt.Println("============= DOWNLOAD DONE =============")

	CloseDownloadPopup(page)
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

func processRegions(task *models.Task, user *models.User, tmpDir, title string, chartParamsMap map[string]string, data StartData) {
	regionGroup, _ := errgroup.WithContext(context.Background())
	regionGroup.SetLimit(constants.CONCURRENT_REQUESTS)

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
				time.Sleep(time.Second * time.Duration(index*3))
				err := processRegion(user, task, task.ChartName, region, filepath.Join(tmpDir, region), chartParamsMap, title, data)
				fmt.Print("============= FINISHED PROCESSING REGION: ", region)
				if err != nil {
					fmt.Println("Error in processing some of the region", region)
					fmt.Println(err)
				}
				return nil
			}
		}(region, index))
	}

	regionGroup.Wait()
	fmt.Println("================= FINISHED PROCESSING ALL REGIONS ==================")
}

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
					err = TraverseDownloadCountriesList(user, task, &token, task.ChartName, title, startYear, endYear, tmpDir, countriesStartData, chartParamsMap, countryList)

					if err != nil {
						fmt.Println("Error processing countries", err)
						return err
					}
					return nil
				}
			}(countryList))
		}
		countryGroup.Wait()

		elapsedTime := time.Since(startTime)
		fmt.Println("Started in", time.Since(startTime).String())
		fmt.Println("Finished in", elapsedTime.String())
	} else {
		fmt.Println("============= Doesn't have countries, downloading popup chart instead ===============")
		countriesDir := path.Join(tmpDir, "countries")
		err := os.Mkdir(countriesDir, 0755)
		if err == nil {
			ProcessCountriesFromPopover(user, task, task.ChartName, title, startYear, endYear, countriesDir, StartData{
				Url:                           url,
				FileName:                      task.CountryFileName,
				Description:                   task.CountryDescription,
				DescriptionOverwriteBehaviour: task.CountryDescriptionOverwriteBehaviour,
			}, chartParamsMap)
		} else {
			fmt.Println("Error creating countries directory: ", err)
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
	StableUrl     string            `json:"stableUrl"`
	SingleImage   bool              `json:"singleImage"`
}

/*
* Chart title
* start/end year
* params // :DONE
* HasCountries
 */

func GetChartInfo(browser *rod.Browser, url, chartFormat, selectedParams string) (*ChartInfo, error) {
	chartInfo := ChartInfo{}
	var err error

	for i := 0; i < 2; i++ {
		err = rod.Try(func() {
			page := browser.MustPage("")
			time.Sleep(time.Second * time.Duration(i*5))

			defer page.Close()
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
			hasCountries, countriesList := getMapHasCountriesFromPage(page)
			chartInfo.HasCountries = hasCountries
			chartInfo.CountriesList = countriesList
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
	downloadIcon := page.MustElement(DOWNLOAD_SVG_ICON_SELECTOR)
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
			page := browser.MustPage("")
			defer page.Close()
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

func traverseDownloadRegion(task *models.Task, data StartData, user *models.User, chartParams map[string]string, token *string, chartName, title, region, url, downloadPath string) {
	regionStr := region
	if regionStr == "NorthAmerica" {
		regionStr = "North America"
	}
	if regionStr == "SouthAmerica" {
		regionStr = "South America"
	}

	l, browser := GetBrowser()
	blankPage := browser.MustPage("")

	page := browser.MustPage("")
	page.MustSetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: env.GetEnv().OWID_UA})
	page.MustSetViewport(constants.VIEWPORT_WIDTH, constants.VIEWPORT_HEIGHT, 1, false)

	fmt.Println("==================== Traversing to: ", url)
	page.MustNavigate(url)
	page.MustWaitLoad()
	page.MustWaitIdle()

	if err := utils.WaitElementWithTimeout(page, DOWNLOAD_BUTTON_SELECTOR, time.Second*10); err != nil {
		fmt.Println("ERROR waiting for DOWNLOAD_BUTTON_SELECTOR for region: ", region)
		return
	}
	if err := utils.WaitElementWithTimeout(page, PLAY_TIMELAPSE_BUTTON_SELECTOR, time.Second*5); err != nil {
		fmt.Println("ERROR waiting for PLAY_TIMELAPSE_BUTTON_SELECTOR for region: ", region)
		return
	}
	if err := utils.WaitElementWithTimeout(page, fmt.Sprintf("%s, %s", START_MARKER_SELECTOR, END_MARKER_SELECTOR), time.Second*5); err != nil {
		fmt.Println("ERROR waiting for EITHER START_MARKER_SELECTOR or END_MARKER_SELECTOR for region: ", region)
		return
	}

	startMarker := page.MustElement(START_MARKER_SELECTOR)
	endMarker := page.MustElement(END_MARKER_SELECTOR)
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

		triedUsingCommonsTemplate := false

		for task.Status == models.TaskStatusProcessing {
			counter = counter + 1
			if owidEnv == "development" && counter >= 5 {
				// break
			}

			models.UpdateTaskLastOperationAt(task.ID)
			if counter >= 50 {
				currentUrl := page.MustInfo().URL

				page.Close()
				browser.Close()
				l.Cleanup()

				l, browser = GetBrowser()
				blankPage = browser.MustPage("")

				page = browser.MustPage("")
				page.MustSetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: env.GetEnv().OWID_UA})
				page.MustSetViewport(constants.VIEWPORT_WIDTH, constants.VIEWPORT_HEIGHT, 1, false)

				page.MustNavigate(currentUrl)
				page.MustWaitLoad()
				page.MustWaitIdle()

				if err := utils.WaitElementWithTimeout(page, DOWNLOAD_BUTTON_SELECTOR, time.Second*10); err != nil {
					fmt.Println("ERROR waiting for DOWNLOAD_BUTTON_SELECTOR for region: ", region)
					break
				}
				if err := utils.WaitElementWithTimeout(page, PLAY_TIMELAPSE_BUTTON_SELECTOR, time.Second*5); err != nil {
					fmt.Println("ERROR waiting for PLAY_TIMELAPSE_BUTTON_SELECTOR for region: ", region)
					break
				}
				counter = 0
			}

			if err := utils.WaitElementWithTimeout(page, fmt.Sprintf("%s, %s", START_MARKER_SELECTOR, END_MARKER_SELECTOR), time.Second*5); err != nil {
				fmt.Println("ERROR waiting for EITHER START_MARKER_SELECTOR or END_MARKER_SELECTOR for region: ", region)
				break

			}
			startMarker = page.MustElement(START_MARKER_SELECTOR)
			endMarker = page.MustElement(END_MARKER_SELECTOR)

			currentYear := ""
			if startMarker != nil {
				if attr, err := startMarker.Attribute("aria-valuenow"); err == nil && *attr != "" {
					currentYear = *attr
				}
			}

			if endMarker != nil {
				if attr, err := endMarker.Attribute("aria-valuenow"); err == nil && *attr != "" {
					currentYear = *attr
				}
			}

			year := currentYear

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
				if existingTB.Status != models.TaskProcessStatusFailed {
					if moveToNextYear(page, startMarker, endMarker, currentYear, startYear) {
						continue
					} else {
						break
					}
				}

				existingTB.Status = models.TaskProcessStatusProcessing
				if err := existingTB.Update(); err != nil {
					fmt.Println("Error updating task process to processing")
				}
				taskProcess = existingTB
			} else {
				taskProcess, err = models.NewTaskProcess(region, year, "", models.TaskProcessStatusProcessing, models.TaskProcessTypeMap, task.ID)
				if err != nil {
					break
				}
			}

			mapPath := filepath.Join(downloadPath, currentYear)
			if err := utils.WaitElementWithTimeout(page, DOWNLOAD_BUTTON_SELECTOR, time.Second*5); err != nil {
				fmt.Println("ERROR waiting for DOWNLOAD_BUTTON_SELECTOR for region: ", region, currentYear)
				FailTaskProcess(taskProcess)
				break
			}

			downloadBtn := page.MustElement(DOWNLOAD_BUTTON_SELECTOR)
			downloadBtn.MustFocus()
			time.Sleep(time.Millisecond * 200)
			// page.Keyboard.Press(input.Enter)
			err = page.Keyboard.Press(input.Enter)
			if err != nil {
				fmt.Println(fmt.Sprintf("%s %s %v", url, "Error clicking download button", err))
				break
			}
			wait := page.Browser().WaitDownload(mapPath)

			if err := utils.WaitElementWithTimeout(page, DOWNLOAD_SVG_ICON_SELECTOR, time.Second*10); err != nil {
				fmt.Println("ERROR waiting for DOWNLOAD_SVG_ICON_SELECTOR for region: ", region, currentYear)
				FailTaskProcess(taskProcess)
				CloseDownloadPopup(page)
				if moveToNextYear(page, startMarker, endMarker, currentYear, startYear) {
					continue
				} else {
					break
				}
			}
			time.Sleep(time.Millisecond * 200)

			err = page.MustElements(DOWNLOAD_SVG_ICON_SELECTOR)[0].Click(proto.InputMouseButtonLeft, 1)
			if err != nil {
				// utils.SendWSProgress(session, taskProcess)
				fmt.Printf("%s, %s, %v", url, "Error clicking download svg button", err)
				FailTaskProcess(taskProcess)
				CloseDownloadPopup(page)
				if moveToNextYear(page, startMarker, endMarker, currentYear, startYear) {
					continue
				} else {
					break
				}
			}

			wait()
			time.Sleep(time.Millisecond * 100)
			if _, err = os.Stat(mapPath); os.IsNotExist(err) {
				FailTaskProcess(taskProcess)
				CloseDownloadPopup(page)
				if moveToNextYear(page, startMarker, endMarker, currentYear, startYear) {
					continue
				} else {
					break
				}
			}

			// Close download modal
			CloseDownloadPopup(page)

			fileInfo, err := getFileInfo(mapPath)
			if err != nil {
				FailTaskProcess(taskProcess)
				if moveToNextYear(page, startMarker, endMarker, currentYear, startYear) {
					continue
				} else {
					break
				}
			}

			lowerCaseContent := strings.ToLower(string(fileInfo.File))
			if strings.Contains(lowerCaseContent, "missing map column") {
				os.Remove(fileInfo.FilePath)
				fmt.Printf("Missing map column %s %s %s, retrying", regionStr, currentYear, GetFileNameFromChartName(chartName))
				FailTaskProcess(taskProcess)
				if moveToNextYear(page, startMarker, endMarker, currentYear, startYear) {
					continue
				} else {
					break
				}
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
				/**
					We need to check if the file is already uploaded.
					If it is, download that file and upload it instead if it have translations
					Make sure to inject metadata again as some new year data might be available
				**/
				filename := replaceVars(data.FileName, replaceData)
				existingMapPath := filepath.Join(downloadPath, currentYear+"_existing")
				existingMapFilePath := path.Join(existingMapPath, "image.svg")
				if err := os.Mkdir(existingMapPath, 0755); err == nil {
					err := downloadCommonsFile(filename, existingMapFilePath, user)
					if err == nil {
						// Check if file has translation switch
						if SVGHasSwitchElement(existingMapFilePath) {
							newFileInfo, err := getFileInfo(existingMapPath)
							if err == nil {
								fileInfo = newFileInfo
								mapPath = existingMapPath
								fmt.Println("============= NEW FILE INFO: ", fileInfo.FilePath)
							} else {
								fmt.Println("============== ERROR Getting existing file info: ", err)
							}
						}
					} else {
						fmt.Println("============ ERROR DOWNLOADING commons file: ", err)
					}
				}

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
			}

			if !moveToNextYear(page, startMarker, endMarker, currentYear, startYear) {
				break
			}
		}
	}

	blankPage.Close()
	browser.Close()
	l.Cleanup()
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
		return fmt.Errorf("Error injecting metadata into svg: ", err)
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

func moveToNextYear(page *rod.Page, startMarker, endMarker *rod.Element, currentYear, startYear string) bool {
	if didReachStartYear(startMarker, endMarker, startYear) {
		return false
	}

	if startMarker != nil {
		time.Sleep(time.Millisecond * 100)
		startMarker.Focus()
		time.Sleep(time.Millisecond * 100)
		page.Keyboard.Press(input.ArrowLeft)
		time.Sleep(time.Millisecond * 100)
		startMarker.Blur()
	}

	if endMarker != nil {
		time.Sleep(time.Millisecond * 100)
		endMarker.Focus()
		time.Sleep(time.Millisecond * 100)
		page.Keyboard.Press(input.ArrowLeft)
		time.Sleep(time.Millisecond * 100)
		endMarker.Blur()
	}

	return true
}

func getMapHasCountriesFromPage(page *rod.Page) (bool, []string) {
	activeTab, _ := GetActivePageTab(page)
	countriesList := make([]string, 0)
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
		countriesList = getCountryListFromPage(page)
	}

	if activeTab != nil {
		activeTab.Click(proto.InputMouseButtonLeft, 1)
		time.Sleep(time.Millisecond * 200)
	}

	return hasLines, countriesList
}

func getCountryListFromPage(page *rod.Page) []string {
	// activeTab := GetActivePageTab(page)
	// lineTab := GetTabByLabel(page, "line")
	// chartTab := GetTabByLabel(page, "chart")

	// if lineTab != nil {
	// 	lineTab.Click(proto.InputMouseButtonLeft, 1)
	// 	time.Sleep(time.Second)
	// } else if chartTab != nil {
	// 	chartTab.Click(proto.InputMouseButtonLeft, 1)
	// 	time.Sleep(time.Second)
	// }

	countries := []string{}

	elements := page.MustElements(".entity-selector__content li")
	// Is regular graph
	if len(elements) > 0 {
		for _, element := range elements {
			label := element.MustElement(".label")
			value := element.MustElement(".value")
			if value != nil && value.MustText() != "" && strings.ToLower(value.MustText()) == "no data" {
				continue
			}
			country := strings.TrimSpace(label.MustText())
			countryCode, ok := constants.COUNTRY_CODES[country]
			if !ok {
				continue
			}
			// check if country is not already in list
			if !utils.Contains(countries, countryCode) {
				countries = append(countries, countryCode)
			}
		}

		return countries
	}

	// Is explorer graph
	elements = page.MustElements(".EntityList label.EntityPickerOption")
	if len(elements) > 0 {
		for _, element := range elements {
			label := element.MustElement(".name")
			classes := element.MustAttribute("class")

			if strings.Contains(*classes, "MissingData") {
				continue
			}

			country := strings.TrimSpace(label.MustText())
			countryCode, ok := constants.COUNTRY_CODES[country]
			if !ok {
				continue
			}
			// check if country is not already in list
			if !utils.Contains(countries, countryCode) {
				countries = append(countries, countryCode)
			}
		}
	}

	// if activeTab != nil {
	// 	activeTab.Click(proto.InputMouseButtonLeft, 1)
	// 	time.Sleep(time.Second)
	// }

	return countries
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
	title = page.MustElement(TITLE_SELECTOR).MustText()
	title = strings.TrimSpace(title)
	suffix := ", " + endYear
	if strings.HasSuffix(title, suffix) {
		title = strings.ReplaceAll(title, suffix, "")
	}

	return startYear, endYear, title
}

func getMapTitleFromPage(page *rod.Page) string {
	title := page.MustElement(TITLE_SELECTOR).MustText()
	title = strings.TrimSpace(title)

	return title
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

	traverseDownloadRegion(task, data, user, chartParamsMap, &token, chartName, title, region, url, downloadPath)
	task.Reload()

	return nil
}

type CountryFillWithYear struct {
	Country string
	Fill    string
	Year    string
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
	taskProcesses, err := models.FindTaskProcessesByTaskIdAndRegion(task.ID, region)
	if err != nil {
		return nil, err
	}

	metadata := make([]CountryFillWithYear, 0)
	for _, tp := range taskProcesses {
		if tp.FillData != "" {
			countriesData, err := svgprocessor.ParseJSONString(tp.FillData)
			if err != nil {
				fmt.Println("Error parsing countries fillData", tp, err)
				continue
			}
			for _, countryData := range countriesData {
				metadata = append(metadata, CountryFillWithYear{
					Country: countryData.Country,
					Fill:    countryData.Fill,
					Year:    tp.Date,
				})
			}
		}
	}

	return &metadata, nil
}

func getRegionFileMetadata(task *models.Task, region string) (string, error) {
	taskProcesses, err := models.FindTaskProcessesByTaskIdAndRegion(task.ID, region)
	if err != nil {
		return "", err
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
				fills = append(fills, CountryFillWithYear{
					Country: countryData.Country,
					Fill:    countryData.Fill,
					Year:    tp.Date,
				})
			}
		}
	}

	// Convert metadata to SVG metadata element
	svgContent := generateSVGMetadataFromFills(fills)
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
			svgBuilder.WriteString(fmt.Sprintf(`      <country name="%s" fill="%s" />`,
				html.EscapeString(data.Country),
				html.EscapeString(data.Fill)))
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
