package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/wpmed-videowiki/OWIDImporter/constants"
	"github.com/wpmed-videowiki/OWIDImporter/env"
	"github.com/wpmed-videowiki/OWIDImporter/models"
	"github.com/wpmed-videowiki/OWIDImporter/utils"
	"golang.org/x/sync/errgroup"
)

func StartChart(taskId string, user *models.User, data StartData) error {
	err := ValidateParameters(data)
	if err != nil {
		return err
	}
	chartName, err := GetChartNameFromUrl(data.Url)
	if err != nil || chartName == "" {
		return fmt.Errorf("invalid url")
	}

	fmt.Println("Chart Name:", chartName)

	// utils.SendWSMessage(session, "msg", "Starting")
	// utils.SendWSMessage(session, "debug", "Fetching Upload token")

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

	task, err := models.FindTaskById(taskId)
	if err != nil {
		return err
	}

	task.ChartName = chartName
	task.Status = models.TaskStatusProcessing
	if err := task.Update(); err != nil {
		fmt.Println("Error setting task to Processing: ", err)
	}
	utils.SendWSTask(task)

	tmpDir, err := os.MkdirTemp("", "owid-exporter")
	if err != nil {
		fmt.Println("Error creating temp directory", err)
		return err
	}
	defer os.RemoveAll(tmpDir)

	// utils.SendWSMessage(session, "debug", "Fetching country list")
	countriesList, title, startYear, endYear, err := GetCountryList(data.Url)
	// utils.SendWSMessage(session, "debug", fmt.Sprintf("Fetched %d countries. Countries are %s", len(countriesList), countriesList))
	if err != nil {
		fmt.Println("Error fetching country list", err)
		return err
	}
	fmt.Println("Countries:====================== ", countriesList)

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

	l, browser := GetBrowser()
	blankPage := browser.MustPage("")

	defer blankPage.Close()
	defer l.Cleanup()
	defer browser.Close()

	startTime := time.Now()
	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(constants.CONCURRENT_REQUESTS)

	for _, country := range countriesList {
		country := country
		g.Go(func(country, downloadPath string, token *string) func() error {
			return func() error {
				if task.Status != models.TaskStatusFailed {
					params := make(map[string]string, 0)
					processCountry(browser, user, task, *token, chartName, country, title, startYear, endYear, downloadPath, data, params)
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

	// utils.SendWSMessage(session, "debug", fmt.Sprintf("Finished in %s", elapsedTime.String()))
	// SendCountriesTemplate(user, filenamesArray)
	if task.Status == models.TaskStatusProcessing {
		task.Status = models.TaskStatusDone
		if err := task.Update(); err != nil {
			fmt.Println("Error saving task staus to done: ", err)
		}
	}

	utils.SendWSTask(task)

	return nil
}

func processCountry(browser *rod.Browser, user *models.User, task *models.Task, token, chartName, country, title, startYear, endYear, downloadPath string, data StartData, chartParams map[string]string) error {
	var err error
	var taskProcess *models.TaskProcess
	// Try to find existing process, otherwise create one
	existingTB, err := models.FindTaskProcessByTaskRegionYear(country, 0, task.ID)
	if existingTB != nil {
		if existingTB.Status != models.TaskProcessStatusFailed {
			// Not in a retry, skip
			// existingTB.Status = models.TaskProcessStatusSkipped
			// existingTB.Update()
			return nil
		}
		existingTB.Status = models.TaskProcessStatusProcessing
		if err := existingTB.Update(); err != nil {
			fmt.Println("Error updating task process to processing")
		}
		taskProcess = existingTB
	} else {
		taskProcess, err = models.NewTaskProcess(country, 0, "", models.TaskProcessStatusProcessing, models.TaskProcessTypeCountry, task.ID)
		if err != nil {
			return err
		}
	}

	utils.SendWSTaskProcess(task.ID, taskProcess)

	url := fmt.Sprintf("%s%s?tab=chart&time=%s..%s&country=~%s", constants.OWID_BASE_URL, chartName, startYear, endYear, country)
	if task.ChartParameters != "" {
		url = fmt.Sprintf("%s&%s", url, task.ChartParameters)
	}

	fmt.Println("================== Processing country: ", url)
	// utils.SendWSTaskProcess(taskId string, taskProcess *models.TaskProcess)
	// utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:processing", country))
	fmt.Println("Processing", url)

	var page *rod.Page
	var FileName string
	// Retry 2 times
	for i := 1; i <= constants.RETRY_COUNT; i++ {
		timeoutDuration := time.Duration(i*constants.CHART_WAIT_TIME_SECONDS) * time.Second
		err = rod.Try(func() {
			page = browser.MustPage("")
			defer page.Close()

			page.MustSetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: env.GetEnv().OWID_UA})
			page.MustNavigate(url)
			fmt.Println("Navigated to url", url)
			fmt.Println("Before timeline startMarker")
			page.Timeout(timeoutDuration).MustElement(".timeline-component .startMarker")
			// utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:processing", country))
			fmt.Println("After timeline startMarker")
			page.MustWaitElementsMoreThan(DOWNLOAD_BUTTON_SELECTOR, 0)
			fmt.Println("After download labesls")

			// title := page.MustElement("h1.header__title").MustText()
			// startYear := page.MustElement(".slider.clickable .handle.startMarker").MustAttribute("aria-valuenow")
			// endYear := page.MustElement(".slider.clickable .handle.endMarker").MustAttribute("aria-valuenow")
			// fmt.Println("After getting title/start/end years", title, *startYear, *endYear)

			// TODO: Check if need to remove
			time.Sleep(time.Second * 1)
			wait := page.Browser().WaitDownload(downloadPath)
			err = page.MustElement(DOWNLOAD_BUTTON_SELECTOR).Click(proto.InputMouseButtonLeft, 1)
			fmt.Println("Clicked download button")
			if err != nil {
				fmt.Println(country, "Error clicking download button", err)
				// utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:failed", country))
				return
			}
			downloadSelector := "div.download-modal__tab-content:nth-child(1) button.download-modal__download-button:nth-child(2)"
			page.MustWaitElementsMoreThan(downloadSelector, 0)
			fmt.Println("Found elements")
			elements := page.MustElements(downloadSelector)
			fmt.Println("Got elements", elements)
			err = elements[0].Click(proto.InputMouseButtonLeft, 1)
			fmt.Println("Clicked Chart download button")
			if err != nil {
				// utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:failed", country))
				fmt.Println(country, "Error clicking download svg button", err)
				return
			}

			fmt.Println("Waiting for download")
			wait()
			fmt.Println("After waiting for download")
			if _, err := os.Stat(downloadPath); os.IsNotExist(err) {
				// utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:failed", country))
				taskProcess.Status = models.TaskProcessStatusFailed
				taskProcess.Update()
				utils.SendWSTaskProcess(task.ID, taskProcess)
				fmt.Println(country, "File not found", err)
				return
			}

			replaceData := ReplaceVarsData{
				Url:       data.Url,
				Title:     title,
				Region:    country,
				StartYear: startYear,
				EndYear:   endYear,
				FileName:  GetFileNameFromChartName(chartName),
				Comment:   "Importing from " + data.Url,
				Params:    chartParams,
			}
			filename, status, err := uploadMapFile(user, token, replaceData, downloadPath, data)
			if err != nil {
				// utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:failed", country))
				taskProcess.Status = models.TaskProcessStatusRetrying
				taskProcess.Update()
				utils.SendWSTaskProcess(task.ID, taskProcess)
				return
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
			// utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:done:%s", country, status))

			FileName = filename
		})

		if err != nil {
			// utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:failed", country))
			taskProcess.Status = models.TaskProcessStatusRetrying
			taskProcess.Update()
			utils.SendWSTaskProcess(task.ID, taskProcess)
			fmt.Println(country, "timeout waiting for start marker", err)
			// utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:retrying", country))
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
		taskProcess.Update()
		utils.SendWSTaskProcess(task.ID, taskProcess)
		// utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:failed", country))
		return err
	}
	// taskProcess.Status = models.TaskStatusDone
	taskProcess.FileName = FileName
	taskProcess.Update()
	utils.SendWSTaskProcess(task.ID, taskProcess)

	return nil
}

func GetCountryList(url string) ([]string, string, string, string, error) {
	// url := fmt.Sprintf("%s%s?tab=chart", constants.OWID_BASE_URL, chartName)
	l, browser := GetBrowser()
	blankPage := browser.MustPage("")

	defer blankPage.Close()
	defer l.Cleanup()
	defer browser.Close()

	page := browser.MustPage("")
	fmt.Println("Getting  country list")
	page.MustSetViewport(1920, 1080, 1, false)

	countries := []string{}
	startYear := ""
	endYear := ""
	title := ""

	err := rod.Try(func() {
		page = page.Timeout(time.Second * constants.CHART_WAIT_TIME_SECONDS)
		page.MustNavigate(url)
		fmt.Println("waiting for entity selector")
		page.MustElement(".entity-selector__content")
		fmt.Println("found entity selector")
		title = page.MustElement("h1.header__title, .HeaderHTML h1").MustText()
		title = strings.TrimSpace(title)
		elements := page.MustElements(".entity-selector__content li")
		for _, element := range elements {
			label := element.MustElement(".label")
			value := element.MustElement(".value")
			if value != nil && value.MustText() != "" && strings.ToLower(value.MustText()) == "no data" {
				fmt.Println("No data for country: ", element.MustText())
				continue
			}
			country := strings.TrimSpace(label.MustText())
			countryCode, ok := constants.COUNTRY_CODES[country]
			if !ok {
				fmt.Println("Country not found", country)
				continue
			}
			// check if country is not already in list
			if !utils.Contains(countries, countryCode) {
				countries = append(countries, countryCode)
			}
		}

		// Get start/end years
		marker := page.MustElement(".handle.startMarker")
		fmt.Println("Got marker", marker)
		startYear = *marker.MustAttribute("aria-valuemin")
		endYear = *marker.MustAttribute("aria-valuemax")
	})

	return countries, title, startYear, endYear, err
}
