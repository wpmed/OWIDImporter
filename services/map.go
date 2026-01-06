package services

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
	"github.com/wpmed-videowiki/OWIDImporter/constants"
	"github.com/wpmed-videowiki/OWIDImporter/env"
	"github.com/wpmed-videowiki/OWIDImporter/models"
	"github.com/wpmed-videowiki/OWIDImporter/owidparser"
	svgprocessor "github.com/wpmed-videowiki/OWIDImporter/svg_processor"
	"github.com/wpmed-videowiki/OWIDImporter/utils"
	"golang.org/x/sync/errgroup"
)

func StartMap(taskId string, user *models.User, data StartData) error {
	err := ValidateParameters(data)
	if err != nil {
		return err
	}
	chartName, err := GetChartNameFromUrl(data.Url)
	if err != nil || chartName == "" {
		return fmt.Errorf("invalid url")
	}

	fmt.Println("Chart Name:", chartName)

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
	fmt.Println("Got edit token")

	tmpDir, err := os.MkdirTemp("", "owid-exporter")
	if err != nil {
		fmt.Println("Error creating temp directory", err)
		return err
	}
	defer os.RemoveAll(tmpDir)

	// startTime := time.Now()
	task, err := models.FindTaskById(taskId)
	if err != nil {
		return err
	}

	models.UpdateTaskLastOperationAt(task.ID)

	task.ChartName = chartName
	task.Status = models.TaskStatusProcessing
	if err := task.Update(); err != nil {
		fmt.Println("Error setting task to Processing: ", err)
	}
	utils.SendWSTask(task)

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
			if task.Status == models.TaskStatusDone || task.Status == models.TaskStatusFailed {
				done = true
				break
			}
		}
	}()

	go func() {
		for {
			time.Sleep(time.Minute)
			if done {
				break
			}
			fmt.Println("Gettng new token")
			tokenResponse, err := utils.DoApiReq[TokenResponse](user, map[string]string{
				"action": "query",
				"meta":   "tokens",
				"format": "json",
			}, nil)
			if err != nil {
				fmt.Println("Error fetching edit token", err)
			} else if tokenResponse.Query.Tokens.CsrfToken != "" {
				token = tokenResponse.Query.Tokens.CsrfToken
				fmt.Println("Got new token")
			}
		}
	}()

	url := fmt.Sprintf("%s%s?tab=map", constants.OWID_BASE_URL, chartName)
	if task.ChartParameters != "" {
		url = fmt.Sprintf("%s&%s", url, task.ChartParameters)
	}

	l, browser := GetBrowser()
	_ = browser.MustPage("")
	chartInfo, err := GetChartInfo(browser, url, task.ChartParameters)
	if err != nil {
		fmt.Println("Error getting chart info: ", err)
		task.Status = models.TaskStatusFailed
		task.Update()
		utils.SendWSTask(task)
		browser.Close()
		l.Cleanup()
		return fmt.Errorf("Error getting chart info")
	}

	chartParamsMap := chartInfo.ParamsMap
	templateName := GenerateTemplateCommonsName(data.TemplateNameFormat, task.ChartName, chartParamsMap)
	task.CommonsTemplateName = templateName
	fmt.Println("==================== COMMONS TEMPLATE NAME: ", templateName, "=====================")
	task.Update()

	startYear := chartInfo.StartYear
	endYear := chartInfo.EndYear
	title := chartInfo.Title
	browser.Close()
	l.Cleanup()

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

	startYearInt, err := strconv.ParseInt(startYear, 10, 64)
	if err != nil {
		// utils.SendWSMessage(session, "debug", fmt.Sprintf("%s:failed", region))
		task.Status = models.TaskStatusFailed
		task.Update()
		utils.SendWSTask(task)
		return fmt.Errorf("failed to parse start year: %v", err)
	}
	endYearInt, err := strconv.ParseInt(endYear, 10, 64)
	if err != nil {
		// utils.SendWSMessage(session, "debug", fmt.Sprintf("%s:failed", region))
		task.Status = models.TaskStatusFailed
		task.Update()
		utils.SendWSTask(task)
		return fmt.Errorf("failed to parse end year: %v", err)
	}

	regionGroup, _ := errgroup.WithContext(context.Background())
	regionGroup.SetLimit(constants.CONCURRENT_REQUESTS)

	for _, region := range constants.REGIONS {
		if task.Status == models.TaskStatusFailed {
			break
		}
		region := region
		regionGroup.Go(func(region string) func() error {
			return func() error {
				if task.Status == models.TaskStatusFailed {
					return nil
				}
				l, browser := GetBrowser()
				blankPage := browser.MustPage("")

				defer blankPage.Close()
				defer l.Cleanup()
				defer browser.Close()
				err := processRegion(browser, user, task, &token, chartName, region, filepath.Join(tmpDir, region), chartParamsMap, startYearInt, endYearInt, title, data)
				fmt.Print("============= FINISHED PROCESSING REGION: ", region)
				if err != nil {
					fmt.Println("Error in processing some of the region", region)
					fmt.Println(err)
				}
				return nil
			}
		}(region))
	}
	regionGroup.Wait()

	if task.ImportCountries == 1 && task.Status == models.TaskStatusProcessing && chartInfo.HasCountries {
		fmt.Print("================= STARTED IMPORTING COUNTRIES")
		result := func() error {
			// countriesList, title, startYear, endYear, err := GetCountryList(url)
			// if err != nil {
			// 	fmt.Println("Error fetching country list", err)
			// 	return err
			// }
			fmt.Println("Countries:====================== ", chartInfo.CountriesList)

			startTime := time.Now()
			g, _ := errgroup.WithContext(context.Background())
			g.SetLimit(constants.CONCURRENT_REQUESTS)

			fmt.Println("================================================")
			fmt.Println("================================================")

			fmt.Println(url)
			fmt.Println(chartParamsMap)
			fmt.Println("================================================")
			fmt.Println("================================================")
			fmt.Println("================================================")
			for _, country := range chartInfo.CountriesList {
				country := country
				g.Go(func(country, downloadPath string, token *string) func() error {
					return func() error {
						if task.Status != models.TaskStatusFailed {
							processCountry(user, task, *token, chartName, country, title, startYear, endYear, downloadPath, StartData{
								Url:                           data.Url,
								FileName:                      task.CountryFileName,
								Description:                   task.CountryDescription,
								DescriptionOverwriteBehaviour: task.CountryDescriptionOverwriteBehaviour,
							}, chartParamsMap)
						}
						return nil
					}
				}(country, filepath.Join(tmpDir, country), &token))
			}

			fmt.Println("Started in", time.Since(startTime).String())
			err = g.Wait()
			elapsedTime := time.Since(startTime)
			fmt.Println("Finished in", elapsedTime.String())
			if err != nil {
				fmt.Println("Error processing countries", err)
				return err
			}
			return nil
		}()

		if result != nil {
			fmt.Print("================== ERROR IMPORTING COUNTRIES: ", err)
			task.Status = models.TaskStatusFailed
			task.Update()
			utils.SendWSTask(task)
			return err
		}
	}

	if task.Status == models.TaskStatusProcessing {
		if data.GenerateTemplateCommons {
			// Create template page in commons
			fmt.Print("============= GENERTING COMMONS TEMPLATE")
			wikiText, err := GetMapTemplate(task.ID)
			fmt.Println("GOT WIKITEXT: ", err)
			if err == nil {
				title, err := createCommonsTemplatePage(user, token, task.CommonsTemplateName, wikiText)
				if err == nil {
					task.CommonsTemplateName = title
					fmt.Print("=============== DONE CREATING COMMONS TEMPLATE")
				} else {
					fmt.Println("Error creating commons template page: ", err)
				}
			} else {
				fmt.Println("Error getting task wikitext", task.ID, err)
			}
		}

		task.Status = models.TaskStatusDone
		if err := task.Update(); err != nil {
			fmt.Println("Error saving task staus to done: ", err)
		}
	}

	utils.SendWSTask(task)

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
	HasCountries  bool              `json:"hasCountries"`
	CountriesList []string          `json:"countriesList"`
}

/*
* Chart title
* start/end year
* params // :DONE
* HasCountries
 */

func GetChartInfo(browser *rod.Browser, url, selectedParams string) (*ChartInfo, error) {
	chartInfo := ChartInfo{}
	var err error

	for i := 0; i < 2; i++ {
		err = rod.Try(func() {
			page := browser.MustPage("")
			defer page.Close()
			page.MustSetViewport(1920, 1080, 1, false)
			page.MustSetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: env.GetEnv().OWID_UA})

			fmt.Println("Before navigate")
			page.MustNavigate(url)
			page.MustWaitIdle()
			page.MustWaitLoad()

			page = page.Timeout(constants.CHART_WAIT_TIME_SECONDS * time.Second)
			page.MustWaitElementsMoreThan(DOWNLOAD_BUTTON_SELECTOR, 0)
			fmt.Println("FOUND DOWNLOAD BTN")
			page.MustWaitElementsMoreThan(PLAY_TIMELAPSE_BUTTON_SELECTOR, 0)
			fmt.Println("FOUND PLAY TIMELAPSE BTN")
			time.Sleep(time.Second * 1)

			chartInfo.Params = GetChartParametersFromPage(page)
			fmt.Println("Got chart params")
			chartInfo.ParamsMap = GetChartParametersMapFromPage(page, selectedParams)
			fmt.Println("Got chart params map")

			startYear, endYear, title := getMapStartEndYearTitleFromPage(page)
			fmt.Println("start/end year title", startYear, endYear, title)
			chartInfo.StartYear = startYear
			chartInfo.EndYear = endYear
			chartInfo.Title = title
			chartInfo.HasCountries = getMapHasCountriesFromPage(page)
			fmt.Println("Got has countries")
			if chartInfo.HasCountries {
				chartInfo.CountriesList = GetCountryListFromPage(page)
			}

			fmt.Println("============= CHART INTO ***************************************** ", chartInfo)
			_, err := GetPageCanDownload(page)
			if err != nil {
				panic(err)
			}
		})
		if err == nil {
			break
		}
	}

	return &chartInfo, err
}

func GetPageCanDownload(page *rod.Page) (bool, error) {
	startMarker := page.MustElement(START_MARKER_SELECTOR)
	endMarker := page.MustElement(END_MARKER_SELECTOR)
	if startMarker == nil && endMarker == nil {
		return false, fmt.Errorf("No start & end markers")
	}

	err := page.MustElement(DOWNLOAD_BUTTON_SELECTOR).Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		fmt.Println(err)
		return false, fmt.Errorf("Cannot find/click on download button")
	}

	page.MustWaitElementsMoreThan(DOWNLOAD_SVG_SELECTOR, 0)
	downloadIcon := page.MustElement(DOWNLOAD_SVG_ICON_SELECTOR)
	if downloadIcon == nil {
		return false, fmt.Errorf("SVG Download icon not found")
	}

	return true, nil
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

			fmt.Println("Before navigate")
			page.MustNavigate(url)
			page.MustWaitIdle()
			page.MustWaitLoad()
			page.MustWaitElementsMoreThan(DOWNLOAD_BUTTON_SELECTOR, 0)
			fmt.Println("FOUND ELEMENT")
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

func traverseDownloadRegion(browser *rod.Browser, task *models.Task, data StartData, user *models.User, chartParams map[string]string, token *string, chartName, title, region, url, downloadPath string) {
	regionStr := region
	if regionStr == "NorthAmerica" {
		regionStr = "North America"
	}
	if regionStr == "SouthAmerica" {
		regionStr = "South America"
	}

	page := browser.MustPage("")
	defer page.Close()
	page.MustSetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: env.GetEnv().OWID_UA})

	fmt.Println("Before navigate")
	page.MustNavigate(url)
	page.MustWaitElementsMoreThan(DOWNLOAD_BUTTON_SELECTOR, 0)
	page.WaitElementsMoreThan(PLAY_TIMELAPSE_BUTTON_SELECTOR, 0)
	fmt.Println("FOUND ELEMENT")

	fmt.Println("DOWNLOADING MAP FILE")
	page.WaitElementsMoreThan(DOWNLOAD_BUTTON_SELECTOR, 0)
	// TODO: REMVOE THIS, just trying to scroll along
	startMarker := page.MustElement(START_MARKER_SELECTOR)
	endMarker := page.MustElement(END_MARKER_SELECTOR)
	if startMarker != nil || endMarker != nil {
		startYear := ""
		endYear := ""
		if startMarker != nil {
			startYear = *startMarker.MustAttribute("aria-valuemin")
			endYear = *startMarker.MustAttribute("aria-valuemax")
		} else if endMarker != nil {
			startYear = *endMarker.MustAttribute("aria-valuemin")
			endYear = *endMarker.MustAttribute("aria-valuemax")
		}

		fmt.Println("00000000000000000000000000 GOT START MARKER 00000000000000000000000000000", startYear, endYear)

		for task.Status != models.TaskStatusFailed {
			models.UpdateTaskLastOperationAt(task.ID)
			startMarker = page.MustElement(".startMarker")
			endMarker = page.MustElement(".endMarker")

			currentYear := ""
			if startMarker != nil {
				currentYear = *startMarker.MustAttribute("aria-valuenow")
			}

			if endMarker != nil {
				currentYear = *endMarker.MustAttribute("aria-valuenow")
			}

			int64Year, err := strconv.ParseInt(currentYear, 10, 32)
			if err != nil {
				// TODO: Check breaking here
				fmt.Println("Error converting current year to int", currentYear, err)
				break
			}

			year := int(int64Year)

			var taskProcess *models.TaskProcess

			existingTB, err := models.FindTaskProcessByTaskRegionYear(region, year, task.ID)
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
					// TODO: Check this
					break
				}
			}

			mapPath := filepath.Join(downloadPath, currentYear)
			err = page.MustElement(DOWNLOAD_BUTTON_SELECTOR).Click(proto.InputMouseButtonLeft, 1)
			if err != nil {
				panic(fmt.Sprintf("%s %s %v", url, "Error clicking download button", err))
			}
			// TODO:  Check if need to remove
			wait := page.Browser().WaitDownload(mapPath)

			downloadSelector := "div.download-modal__tab-content:nth-child(1) button.download-modal__download-button:nth-child(2)"
			page.MustWaitElementsMoreThan(downloadSelector, 0)
			err = page.MustElements(downloadSelector)[0].Click(proto.InputMouseButtonLeft, 1)
			if err != nil {
				// utils.SendWSProgress(session, taskProcess)
				fmt.Printf("%s, %s, %v", url, "Error clicking download svg button", err)
				break
			}

			wait()
			time.Sleep(time.Millisecond * 100)
			if _, err = os.Stat(mapPath); os.IsNotExist(err) {
				// TODO: Check this
				// panic(fmt.Sprintf("%s %s %v", url, "File not found", err))
				break
			}
			fmt.Println("================= DOWNLOADED IN: ", mapPath)

			// Close download modal
			closeBtn := page.MustElement("div.download-modal-content button.close-button")
			if closeBtn != nil {
				closeBtn.Click(proto.InputMouseButtonLeft, 1)
			}

			// TODO: DOWNLOAD FILE AND UPLOAD
			replaceData := ReplaceVarsData{
				Url:      data.Url,
				Title:    title,
				Region:   regionStr,
				Year:     currentYear,
				FileName: GetFileNameFromChartName(chartName),
				Comment:  "Importing from " + data.Url,
				Params:   chartParams,
			}

			fileInfo, err := getFileInfo(mapPath)
			if err != nil {
				if moveToNextYear(page, startMarker, endMarker, currentYear, startYear) {
					continue
				} else {
					break
				}
			}

			lowerCaseContent := strings.ToLower(string(fileInfo.File))
			if strings.Contains(lowerCaseContent, "missing map column") {
				os.Remove(fileInfo.FilePath)
				fmt.Printf("Missing map column %s %s %s, retrying", replaceData.Region, replaceData.Year, replaceData.FileName)
				if moveToNextYear(page, startMarker, endMarker, currentYear, startYear) {
					continue
				} else {
					break
				}
			}

			Filename, status, err := uploadMapFile(user, *token, replaceData, mapPath, data)
			if err != nil {
				fmt.Println("Error processing", region, year)
				taskProcess.Status = models.TaskProcessStatusFailed
				taskProcess.Update()
				utils.SendWSTaskProcess(task.ID, taskProcess)
			} else {
				taskProcess.FileName = Filename
				taskProcess.Update()

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

				switch status {
				case "skipped":
					taskProcess.Status = models.TaskProcessStatusSkipped
					taskProcess.Update()
					utils.SendWSTaskProcess(task.ID, taskProcess)
				case "description_updated":
					taskProcess.Status = models.TaskProcessStatusDescriptionUpdated
					taskProcess.Update()
					utils.SendWSTaskProcess(task.ID, taskProcess)
				case "overwritten":
					taskProcess.Status = models.TaskProcessStatusOverwritten
					taskProcess.Update()
					utils.SendWSTaskProcess(task.ID, taskProcess)
				case "uploaded":
					taskProcess.Status = models.TaskProcessStatusUploaded
					taskProcess.Update()
					utils.SendWSTaskProcess(task.ID, taskProcess)
				}
			}

			if !moveToNextYear(page, startMarker, endMarker, currentYear, startYear) {
				break
			}
		}
	}
}

func moveToNextYear(page *rod.Page, startMarker, endMarker *rod.Element, currentYear, startYear string) bool {
	if (startMarker != nil && *startMarker.MustAttribute("aria-valuenow") == startYear) || (endMarker != nil && *endMarker.MustAttribute("aria-valuenow") == startYear) {
		fmt.Println("REACHING END YEAR")
		return false
	}
	if startMarker != nil {
		fmt.Println("Start marker value now: ", currentYear)
		time.Sleep(time.Millisecond * 100)
		startMarker.Focus()
		fmt.Println("CLicking left on start")
		time.Sleep(time.Millisecond * 100)
		page.Keyboard.Press(input.ArrowLeft)
		fmt.Println("Clicked left on start")
		time.Sleep(time.Millisecond * 100)
		startMarker.Blur()
	}

	if endMarker != nil {
		fmt.Println("End marker value now: ", currentYear)
		time.Sleep(time.Millisecond * 100)
		fmt.Println("CLicking left on end")
		endMarker.Focus()
		time.Sleep(time.Millisecond * 100)
		page.Keyboard.Press(input.ArrowLeft)
		time.Sleep(time.Millisecond * 100)
		endMarker.Blur()
		fmt.Println("Clicked left on start")
		fmt.Println("============ STEP DONE ===============")
	}

	return true
}

func downloadMapData(browser *rod.Browser, url, dataPath, metadataPath, mapPath string) (*owidparser.OWIDGrapherConfig, error) {
	var config owidparser.OWIDGrapherConfig
	var err error
	configJSON := ""
	dataUrl := ""
	metadataUrl := ""
	configUrl := ""
	pageHtml := ""

	fmt.Println("DOWNLOADING MAP DATA: ", url)
	for trials := 0; trials < 2; trials++ {
		err = rod.Try(func() {
			page := browser.MustPage("")
			defer page.Close()
			page.MustSetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: env.GetEnv().OWID_UA})
			page = page.Timeout(constants.CHART_WAIT_TIME_SECONDS * time.Second)

			router := browser.HijackRequests()
			defer router.MustStop()

			fmt.Println("Getting map data: ", url)
			router.MustAdd("*.json", func(ctx *rod.Hijack) {
				fmt.Println("Got req: ", ctx.Request.URL().String())
				if ctx.Request.Method() == "GET" {
					url := ctx.Request.URL().String()
					if strings.Contains(url, ".data.json") && dataUrl == "" {
						dataUrl = url
					}
					if strings.Contains(url, ".metadata.json") && metadataUrl == "" {
						metadataUrl = url
					}
					if strings.Contains(url, ".config.json") && configUrl == "" {
						configUrl = url
					}
				}

				fmt.Println("Finished req: ", ctx.Request.URL().String())
				ctx.MustLoadResponse()
			})
			go router.Run()

			fmt.Println("Before navigate")
			page.MustNavigate(url)
			page.MustWaitElementsMoreThan(DOWNLOAD_BUTTON_SELECTOR, 0)
			fmt.Println("FOUND ELEMENT")
			configJSON = page.MustEval(`() => {
				return JSON.stringify(window._OWID_GRAPHER_CONFIG);
			}`).String()

			fmt.Println("DOWNLOADING MAP FILE")
			page.WaitElementsMoreThan(DOWNLOAD_BUTTON_SELECTOR, 0)
			err := page.MustElement(DOWNLOAD_BUTTON_SELECTOR).Click(proto.InputMouseButtonLeft, 1)
			if err != nil {
				panic(fmt.Sprintf("%s %s %v", url, "Error clicking download button", err))
			}
			// TODO:  Check if need to remove
			time.Sleep(time.Second * 1)
			wait := page.Browser().WaitDownload(mapPath)

			downloadSelector := "div.download-modal__tab-content:nth-child(1) button.download-modal__download-button:nth-child(2)"
			page.MustWaitElementsMoreThan(downloadSelector, 0)
			err = page.MustElements(downloadSelector)[0].Click(proto.InputMouseButtonLeft, 1)
			if err != nil {
				// utils.SendWSProgress(session, taskProcess)
				panic(fmt.Sprintf("%s, %s, %v", url, "Error clicking download svg button", err))
			}

			wait()
			time.Sleep(time.Millisecond * 500)
			if _, err = os.Stat(mapPath); os.IsNotExist(err) {
				panic(fmt.Sprintf("%s %s %v", url, "File not found", err))
			}
			fmt.Println("Finished", url)
			pageHtml = page.MustHTML()
		})

		if err == nil {
			break
		}
	}

	fmt.Println("Data URL: ", dataUrl)
	fmt.Println("Metadata URL: ", metadataUrl)
	fmt.Println("Config URL: ", configUrl)

	if dataUrl == "" || metadataUrl == "" {
		return nil, fmt.Errorf("Error getting data/metadata urls")
	}

	fmt.Println("DOWNLOADING DATA")
	if err := utils.DownloadFile(dataUrl, dataPath); err != nil {
		return nil, fmt.Errorf("ERROR DOWNLOADING DATA JSON %v", err)
	}
	fmt.Println("DOWNLOADING METADATA")
	if err := utils.DownloadFile(metadataUrl, metadataPath); err != nil {
		return nil, fmt.Errorf("ERROR DOWNLOADING METADATA JSON %v", err)
	}

	// Extract the window._OWID_GRAPHER_CONFIG object
	err = json.Unmarshal([]byte(configJSON), &config)
	if err != nil {
		fmt.Println("Failed to parse config, trying from config url: ", err)

		if configUrl != "" {
			client := &http.Client{
				Timeout: 10 * time.Second,
			}

			resp, err := client.Get(configUrl)
			if err != nil {
				fmt.Printf("Error fetching URL: %v\n", err)
				return nil, err
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				fmt.Printf("Error reading response: %v\n", err)
				return nil, err
			}

			err = json.Unmarshal(body, &config)
			if err != nil {
				fmt.Printf("Error parsing JSON: %v\n", err)
				return nil, err
			}
			fmt.Println("******************************************** GOT CONFIG FROM CONFIGURL: ==== ", config)

			return &config, nil
		} else if pageHtml != "" {
			// Try regex matching
			re := regexp.MustCompile(`//EMBEDDED_JSON\s*([\s\S]*?)\s*//EMBEDDED_JSON`)

			// Find the match
			matches := re.FindStringSubmatch(pageHtml)
			if len(matches) > 1 {
				jsonString := matches[1] // The captured group
				fmt.Println("===================== ************************* ")
				fmt.Println("Captured JSON:")
				fmt.Println("===================== ************************* ")
				fmt.Println(jsonString)
				err = json.Unmarshal([]byte(jsonString), &config)
				if err != nil {
					fmt.Println("Failed to parse config from page HTML", err)
					return nil, err
				}
				return &config, nil
			} else {
				fmt.Println("No match found")
				return nil, err
			}

		} else {
			return nil, err
		}
	}

	return &config, nil
}

func getMapHasCountriesFromPage(page *rod.Page) bool {
	fmt.Println("Getting Has Countries From Page: ", page.MustInfo().URL)

	lineElements := page.MustElements(".ContentSwitchers__Container div.Tabs div.Tabs__Tab .label")
	hasLines := false

	for _, el := range lineElements {
		text, err := el.Text()
		if err != nil {
			fmt.Println("Error parsing line text", err)
			continue
		}

		fmt.Println("Tab item: ", text)
		text = strings.ToLower(text)
		if text == "line" {
			hasLines = true
			break
		}
	}

	return hasLines
}

func getMapStartEndYearTitleFromPage(page *rod.Page) (string, string, string) {
	fmt.Println("Getting map start/end year + title: ", page.MustInfo().URL)

	startYear := ""
	endYear := ""
	title := ""

	marker := page.MustElement(START_MARKER_SELECTOR)
	fmt.Println("Got marker", marker)
	startYear = *marker.MustAttribute("aria-valuemin")
	endYear = *marker.MustAttribute("aria-valuemax")
	fmt.Println("Got start/end")
	title = page.MustElement("h1.header__title, .HeaderHTML h1").MustText()
	title = strings.TrimSpace(title)
	fmt.Println("Got start/end title")
	suffix := ", " + endYear
	if strings.HasSuffix(title, suffix) {
		title = strings.ReplaceAll(title, suffix, "")
	}

	fmt.Println("Start: ", startYear, " End: ", endYear, " Title: ", title)
	return startYear, endYear, title
}

func getMapStartEndYearTitle(browser *rod.Browser, url string) (string, string, string) {
	fmt.Println("Getting map start/end year + title: ", url)

	startYear := ""
	endYear := ""
	title := ""

	for i := 0; i < constants.RETRY_COUNT; i++ {
		err := rod.Try(func() {
			page := browser.MustPage("")
			defer page.Close()
			page = page.Timeout(constants.CHART_WAIT_TIME_SECONDS * time.Second)
			page.MustSetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: env.GetEnv().OWID_UA})
			page.MustNavigate(url)
			fmt.Println("Before idle")
			page.MustWaitIdle()
			fmt.Println("After idle")
			marker := page.MustElement(START_MARKER_SELECTOR)
			fmt.Println("Got marker", marker)
			startYear = *marker.MustAttribute("aria-valuemin")
			endYear = *marker.MustAttribute("aria-valuemax")
			fmt.Println("Got start/end")
			title = page.MustElement("h1.header__title, .HeaderHTML h1").MustText()
			title = strings.TrimSpace(title)
			fmt.Println("Got start/end title")
			suffix := ", " + endYear
			if strings.HasSuffix(title, suffix) {
				title = strings.ReplaceAll(title, suffix, "")
			}
		})

		if err == nil {
			break
		}
	}

	fmt.Println("Start: ", startYear, " End: ", endYear, " Title: ", title)
	return startYear, endYear, title
}

func processRegion(browser *rod.Browser, user *models.User, task *models.Task, token *string, chartName string, region, downloadPath string, chartParamsMap map[string]string, startYearInt, endYearInt int64, title string, data StartData) error {
	// Get start and end years
	// get chart title
	// Process each year
	// utils.SendWSMessage(session, "debug", fmt.Sprintf("%s:processing", region))
	fmt.Println("Processing region: ", region, downloadPath)
	_, errExists := os.Stat(downloadPath)
	if os.IsNotExist(errExists) {
		os.Mkdir(downloadPath, 0755)
	}

	url := ""
	if region == "World" {
		// World chart has no region parameter
		url = fmt.Sprintf("%s%s?tab=map", constants.OWID_BASE_URL, chartName)
	} else {
		url = fmt.Sprintf("%s%s?tab=map&region=%s", constants.OWID_BASE_URL, chartName, region)
	}
	// TODO:

	if task.ChartParameters != "" {
		url = fmt.Sprintf("%s&%s", url, task.ChartParameters)
	}
	url = fmt.Sprintf("%s&time=latest", url)
	traverseDownloadRegion(browser, task, data, user, chartParamsMap, token, chartName, title, region, url, downloadPath)
	task.Reload()
	// Sleep for 10 seconds to avoid API complains of reuploading
	// Attach country data to the first file metadata on commons
	time.Sleep(time.Second * 10)
	if task.Status != models.TaskStatusFailed {
		metadata, err := getRegionFileMetadata(task, region)
		if err != nil {
			fmt.Println("Error generating metadata: ", err)
		} else if metadata != "" {
			// fmt.Println("GOT METADATA, YAAAAAAAAAAY: ", metadata)
			fmt.Println("GOT METADATA, YAAAAAAAAAAY")
			err := processRegionYear(browser, user, task, *token, chartName, title, region, chartParamsMap, filepath.Join(downloadPath, strconv.FormatInt(startYearInt, 10)+"_final"), int(startYearInt), data, metadata)
			if err != nil {
				fmt.Println("Error uploading with metadata: ", err)
			}
		}
	}

	// START OLD FLOW

	// // try to get JSON data
	// dataPath := path.Join(downloadPath, "data.json")
	// metadataPath := path.Join(downloadPath, "metadata.json")
	// mapPath := path.Join(downloadPath, "map")
	//
	// var config *owidparser.OWIDGrapherConfig
	// var err error
	// for i := 0; i < constants.RETRY_COUNT; i++ {
	// 	config, err = downloadMapData(browser, url, dataPath, metadataPath, mapPath)
	// 	if err == nil && config != nil {
	// 		break
	// 	}
	// }
	//
	// // fmt.Println("CONFIG IS: ==================", config)
	// // Fallback to the old flow of getting each image individually
	// if err != nil {
	// 	fmt.Println("ERROR GETTING CHART JSON DATA", err)
	//
	// 	g, _ := errgroup.WithContext(context.Background())
	// 	g.SetLimit(constants.CONCURRENT_REQUESTS)
	//
	// 	// var filename string
	// 	for year := startYearInt; year <= endYearInt; year++ {
	// 		year := year
	// 		g.Go(func(region string, year int, downloadPath string) func() error {
	// 			return func() error {
	// 				if task.Status != models.TaskStatusFailed {
	// 					err := processRegionYear(browser, user, task, *token, chartName, title, region, downloadPath, year, data, "")
	// 					return err
	// 				}
	// 				return nil
	// 			}
	// 		}(region, int(year), filepath.Join(downloadPath, strconv.FormatInt(year, 10))))
	// 	}
	//
	// 	if err := g.Wait(); err != nil {
	// 		return err
	// 	}
	//
	// 	task.Reload()
	// 	if task.Status != models.TaskStatusFailed {
	// 		// Attach country data to the first file metadata on commons
	// 		metadata, err := getRegionFileMetadata(task, region)
	// 		if err != nil {
	// 			fmt.Println("Error generating metadata: ", err)
	// 		} else if metadata != "" {
	// 			// fmt.Println("GOT METADATA, YAAAAAAAAAAY: ", metadata)
	// 			fmt.Println("GOT METADATA, YAAAAAAAAAAY")
	// 			err := processRegionYear(browser, user, task, *token, chartName, title, region, filepath.Join(downloadPath, strconv.FormatInt(startYearInt, 10)+"_final"), int(startYearInt), data, metadata)
	// 			if err != nil {
	// 				fmt.Println("Error uploading with metadata: ", err)
	// 			}
	// 		}
	// 	}
	// } else {
	// 	fmt.Println("DOWNLOADED CHART DATA", dataPath, downloadPath)
	// 	mapInfo, err := getFileInfo(mapPath)
	// 	if err != nil {
	// 		return err
	// 	}
	// 	fmt.Println("Map info: ", mapInfo.FilePath)
	// 	err = generateAndprocessRegionYearsNewFlow(user, task, config, *token, chartName, title, region, int(startYearInt), int(endYearInt), data, dataPath, metadataPath, mapPath, chartParamsMap)
	// 	if err != nil {
	// 		return err
	// 	}
	// }

	// END OLD FLOW

	return nil
}

type CountryFillWithYear struct {
	Country string
	Fill    string
	Year    int
}

func getRegionFileMetadata(task *models.Task, region string) (string, error) {
	taskProcesses, err := models.FindTaskProcessesByTaskIdAndRegion(task.ID, region)
	if err != nil {
		return "", err
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
					Year:    tp.Year,
				})
			}
		}
	}

	// Convert metadata to SVG metadata element
	svgContent := generateSVGMetadata(metadata)
	return svgContent, nil
}

// Generate SVG metadata with data grouped by years
func generateSVGMetadata(metadata []CountryFillWithYear) string {
	var svgBuilder strings.Builder

	// Group data by year
	dataByYear := make(map[int][]CountryFillWithYear)
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
	years := make([]int, 0, len(dataByYear))
	for year := range dataByYear {
		years = append(years, year)
	}
	sort.Ints(years) // Sort years for consistent output

	// Create elements for each year with its countries
	for _, year := range years {
		countriesForYear := dataByYear[year]

		svgBuilder.WriteString(fmt.Sprintf(`    <year value="%d">`, year))
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

func downloadChartFile(browser *rod.Browser, url, downloadPath string) error {
	timeoutDuration := time.Duration(constants.CHART_WAIT_TIME_SECONDS) * time.Second

	err := rod.Try(func() {
		page := browser.MustPage("")
		defer page.Close()
		page = page.Timeout(timeoutDuration)
		page.MustNavigate(url)
		page.MustWaitIdle()

		page.WaitElementsMoreThan(DOWNLOAD_BUTTON_SELECTOR, 0)
		err := page.MustElement(DOWNLOAD_BUTTON_SELECTOR).Click(proto.InputMouseButtonLeft, 1)
		if err != nil {
			panic(fmt.Sprintf("%s %s %v", url, "Error clicking download button", err))
		}
		// TODO:  Check if need to remove
		time.Sleep(time.Second * 1)
		wait := page.Browser().WaitDownload(downloadPath)

		downloadSelector := "div.download-modal__tab-content:nth-child(1) button.download-modal__download-button:nth-child(2)"
		page.MustWaitElementsMoreThan(downloadSelector, 0)
		err = page.MustElements(downloadSelector)[0].Click(proto.InputMouseButtonLeft, 1)
		if err != nil {
			// utils.SendWSProgress(session, taskProcess)
			panic(fmt.Sprintf("%s, %s, %v", url, "Error clicking download svg button", err))
		}

		wait()
		time.Sleep(time.Millisecond * 500)
		if _, err = os.Stat(downloadPath); os.IsNotExist(err) {
			panic(fmt.Sprintf("%s %s %v", url, "File not found", err))
		}
		fmt.Println("Finished", url)
	})
	return err
}

func processRegionYearNewFlow(user *models.User, task *models.Task, data StartData, token, region, title, chartName, mapDir, fileMetadata string, year int, chartParams map[string]string) error {
	var err error
	var taskProcess *models.TaskProcess
	mapPath := path.Join(mapDir, strconv.Itoa(year))
	fmt.Println("Map path: ", mapPath)
	if err := models.UpdateTaskLastOperationAt(task.ID); err != nil {
		fmt.Println("Error updating task last operation at ", task.ID, err)
	}

	// Try to find existing process, otherwise create one
	existingTB, err := models.FindTaskProcessByTaskRegionYear(region, year, task.ID)
	if existingTB != nil {
		if existingTB.Status != models.TaskProcessStatusFailed && fileMetadata == "" {
			// Not in a retry, skip
			// existingTB.Status = models.TaskProcessStatusSkipped
			// existingTB.Update()
			// utils.SendWSTaskProcess(task.ID, existingTB)
			return nil
		}
		existingTB.Status = models.TaskProcessStatusProcessing
		if err := existingTB.Update(); err != nil {
			fmt.Println("Error updating task process to processing")
		}
		taskProcess = existingTB
	} else {
		taskProcess, err = models.NewTaskProcess(region, year, "", models.TaskProcessStatusProcessing, models.TaskProcessTypeMap, task.ID)
		if err != nil {
			fmt.Println("Error creating new task process: ", region, year, err)
			return err
		}
	}

	utils.SendWSTaskProcess(task.ID, taskProcess)

	// control := l.Set("--no-sandbox").Headless(false).MustLaunch()
	regionStr := region
	if regionStr == "NorthAmerica" {
		regionStr = "North America"
	}
	if regionStr == "SouthAmerica" {
		regionStr = "South America"
	}

	replaceData := ReplaceVarsData{
		Url:      data.Url,
		Title:    title,
		Region:   regionStr,
		Year:     strconv.Itoa(year),
		FileName: GetFileNameFromChartName(chartName),
		Comment:  "Importing from " + data.Url,
		Params:   chartParams,
	}

	fileInfo, err := getFileInfo(mapPath)
	if err != nil {
		fmt.Println("Error getting file info: ", err)
		return err
	}

	if fileMetadata != "" {
		if err := InjectMetadataIntoSVGSameFile(fileInfo.FilePath, fileMetadata); err != nil {
			fmt.Println("Error injecting metadata into svg: ", err)
		} else {
			replaceData.Comment = "Importing from " + data.Url + " with metadata"
		}
	}

	Filename, status, err := uploadMapFile(user, token, replaceData, mapPath, data)
	if err != nil {
		taskProcess.Status = models.TaskProcessStatusFailed
		taskProcess.Update()
		utils.SendWSTaskProcess(task.ID, taskProcess)
		fmt.Println("Error processing", region, year)
		return err
	}
	taskProcess.FileName = Filename

	// Save the countries fill data
	countryFlls, err := svgprocessor.ExtractCountryFills(fileInfo.FilePath)
	if err != nil {
		fmt.Println("Error extracting country fills ", err)
	} else {
		jsonStr, err := svgprocessor.ConvertToJSON(countryFlls)
		if jsonStr != "" && err == nil {
			taskProcess.FillData = jsonStr
		}
	}
	taskProcess.Update()

	switch status {
	case "skipped":
		taskProcess.Status = models.TaskProcessStatusSkipped
		taskProcess.Update()
		utils.SendWSTaskProcess(task.ID, taskProcess)
	case "description_updated":
		taskProcess.Status = models.TaskProcessStatusDescriptionUpdated
		taskProcess.Update()
		utils.SendWSTaskProcess(task.ID, taskProcess)
	case "overwritten":
		taskProcess.Status = models.TaskProcessStatusOverwritten
		taskProcess.Update()
		utils.SendWSTaskProcess(task.ID, taskProcess)
	case "uploaded":
		taskProcess.Status = models.TaskProcessStatusUploaded
		taskProcess.Update()
		utils.SendWSTaskProcess(task.ID, taskProcess)
	}

	time.Sleep(time.Millisecond * 2000)
	return nil
}

func generateAndprocessRegionYearsNewFlow(user *models.User, task *models.Task, config *owidparser.OWIDGrapherConfig, token, chartName, title, region string, startYear, endYear int, data StartData, dataPath, metadataPath, mapDir string, chartParams map[string]string) error {
	fileinfo, err := getFileInfo(mapDir)
	if err != nil {
		return err
	}

	results, err := owidparser.GenerateImages(config, title, endYear, dataPath, metadataPath, fileinfo.FilePath, mapDir)
	if err != nil {
		return err
	}

	for i := range *results {
		if task.Status == models.TaskStatusFailed {
			break
		}
		processRegionYearNewFlow(user, task, data, token, region, title, chartName, mapDir, "", (*results)[i].Year, chartParams)
	}
	// for year := startYear; year <= endYear; year++ {
	// 	if task.Status == models.TaskStatusFailed {
	// 		break
	// 	}
	// 	processRegionYearNewFlow(user, task, data, token, region, title, chartName, mapDir, "", year)
	// }

	task.Reload()
	if task.Status != models.TaskStatusFailed {
		// Attach country data to the first file metadata on commons
		metadata, err := getRegionFileMetadata(task, region)
		if err != nil {
			fmt.Println("Error generating metadata: ", err)
		} else if metadata != "" {
			// fmt.Println("GOT METADATA, YAAAAAAAAAAY: ", metadata)
			fmt.Println("GOT METADATA, YAAAAAAAAAAY")
			err := processRegionYearNewFlow(user, task, data, token, region, title, chartName, mapDir, metadata, startYear, chartParams)
			if err != nil {
				fmt.Println("Error uploading with metadata: ", err)
			}
		}
	}

	return nil
}

func processRegionYear(browser *rod.Browser, user *models.User, task *models.Task, token, chartName, title, region string, chartParams map[string]string, downloadPath string, year int, data StartData, fileMetadata string) error {
	var err error
	var taskProcess *models.TaskProcess
	// Try to find existing process, otherwise create one
	existingTB, err := models.FindTaskProcessByTaskRegionYear(region, year, task.ID)
	if existingTB != nil {
		if existingTB.Status != models.TaskProcessStatusFailed && fileMetadata == "" {
			// Not in a retry, skip
			// existingTB.Status = models.TaskProcessStatusSkipped
			// existingTB.Update()
			// utils.SendWSTaskProcess(task.ID, existingTB)
			return nil
		}
		existingTB.Status = models.TaskProcessStatusProcessing
		if err := existingTB.Update(); err != nil {
			fmt.Println("Error updating task process to processing")
		}
		taskProcess = existingTB
	} else {
		taskProcess, err = models.NewTaskProcess(region, year, "", models.TaskProcessStatusProcessing, models.TaskProcessTypeMap, task.ID)
		if err != nil {
			return err
		}
	}

	utils.SendWSTaskProcess(task.ID, taskProcess)
	// utils.SendWSProgress(session, taskProcess)

	// control := l.Set("--no-sandbox").Headless(false).MustLaunch()
	url := ""
	if region == "World" {
		// World chart has no region parameter
		url = fmt.Sprintf("%s%s?time=%d&tab=map", constants.OWID_BASE_URL, chartName, year)
	} else {
		url = fmt.Sprintf("%s%s?region=%s&time=%d&tab=map", constants.OWID_BASE_URL, chartName, region, year)
	}
	if task.ChartParameters != "" {
		url = fmt.Sprintf("%s&%s", url, task.ChartParameters)
	}

	fmt.Println(url)
	regionStr := region
	if regionStr == "NorthAmerica" {
		regionStr = "North America"
	}
	if regionStr == "SouthAmerica" {
		regionStr = "South America"
	}

	var filename string
	for i := 0; i <= constants.RETRY_COUNT; i++ {
		err = rod.Try(func() {
			if err = downloadChartFile(browser, url, downloadPath); err != nil {
				panic(err)
			}
			fmt.Println("Finished", year, title)

			replaceData := ReplaceVarsData{
				Url:      data.Url,
				Title:    title,
				Region:   regionStr,
				Year:     strconv.Itoa(year),
				FileName: GetFileNameFromChartName(chartName),
				Comment:  "Importing from " + data.Url,
				Params:   chartParams,
			}

			fileInfo, err := getFileInfo(downloadPath)
			if err != nil {
				panic(err)
			}
			lowerCaseContent := strings.ToLower(string(fileInfo.File))
			if strings.Contains(lowerCaseContent, "missing map column") {
				os.Remove(fileInfo.FilePath)
				panic(fmt.Sprintf("Missing map column %s %s %s, retrying", replaceData.Region, replaceData.Year, replaceData.FileName))
			}

			if fileMetadata != "" {
				if err := InjectMetadataIntoSVGSameFile(fileInfo.FilePath, fileMetadata); err != nil {
					fmt.Println("Error injecting metadata into svg: ", err)
				} else {
					replaceData.Comment = "Importing from " + data.Url + " with metadata"
				}
			}

			Filename, status, err := uploadMapFile(user, token, replaceData, downloadPath, data)
			if err != nil {
				fmt.Println("Error processing", region, year)
				panic(err)
			}
			filename = Filename

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

			switch status {
			case "skipped":
				taskProcess.Status = models.TaskProcessStatusSkipped
				taskProcess.Update()
				utils.SendWSTaskProcess(task.ID, taskProcess)
			case "description_updated":
				taskProcess.Status = models.TaskProcessStatusDescriptionUpdated
				taskProcess.Update()
				utils.SendWSTaskProcess(task.ID, taskProcess)
			case "overwritten":
				taskProcess.Status = models.TaskProcessStatusOverwritten
				taskProcess.Update()
				utils.SendWSTaskProcess(task.ID, taskProcess)
			case "uploaded":
				taskProcess.Status = models.TaskProcessStatusUploaded
				taskProcess.Update()
				utils.SendWSTaskProcess(task.ID, taskProcess)
			}
		})

		if err != nil {
			fmt.Println(year, "timeout waiting for start marker", err)
			if i == constants.RETRY_COUNT {
				taskProcess.Status = models.TaskProcessStatusFailed
			} else {
				taskProcess.Status = models.TaskProcessStatusRetrying
			}
			taskProcess.Update()
			utils.SendWSTaskProcess(task.ID, taskProcess)
		} else {
			err = nil
			break
		}
	}

	if err := models.UpdateTaskLastOperationAt(task.ID); err != nil {
		fmt.Println("Error updating task last operation at ", task.ID, err)
	}

	if err != nil {
		taskProcess.Status = models.TaskProcessStatusFailed
		err2 := taskProcess.Update()
		utils.SendWSTaskProcess(task.ID, taskProcess)
		if err2 != nil {
			fmt.Println("Error saving task process update: ", err)
		}

		return err
	}

	// taskProcess.Status = models.TaskProcessStatusDone
	taskProcess.FileName = filename
	err2 := taskProcess.Update()
	if err2 != nil {
		fmt.Println("Error saving task process update: ", err)
	}

	utils.SendWSTaskProcess(task.ID, taskProcess)
	return nil
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
