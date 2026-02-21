package services

import (
	"context"
	"fmt"
	"os"
	"path"
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

	startTime := time.Now()
	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(constants.CONCURRENT_REQUESTS)

	for _, country := range countriesList {
		country := country
		g.Go(func(country, downloadPath string, token *string) func() error {
			return func() error {
				if task.Status == models.TaskStatusProcessing {
					params := make(map[string]string, 0)
					processCountry(user, task, *token, chartName, country, title, startYear, endYear, downloadPath, data, params)
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

func ProcessCountriesFromPopover(user *models.User, task *models.Task, token, chartName, title, startYear, endYear, downloadPath string, data StartData, chartParams map[string]string) error {
	url := utils.AttachQueryParamToUrl(task.URL, fmt.Sprintf("tab=map"))
	if task.ChartParameters != "" {
		url = utils.AttachQueryParamToUrl(url, task.ChartParameters)
	}

	models.UpdateTaskLastOperationAt(task.ID)
	result := DownloadCountryGraphsFromPopover(url, downloadPath)

	for country, path := range result {
		models.UpdateTaskLastOperationAt(task.ID)
		// Throttle for api usage limit
		time.Sleep(time.Second)

		var taskProcess *models.TaskProcess
		// Try to find existing process, otherwise create one
		existingTB, err := models.FindTaskProcessByTaskRegionDate(country, "", task.ID)
		if existingTB != nil {
			if existingTB.Status != models.TaskProcessStatusFailed {
				// Not in a retry, skip
				// existingTB.Status = models.TaskProcessStatusSkipped
				// existingTB.Update()
				continue
			}
			existingTB.Status = models.TaskProcessStatusProcessing
			if err := existingTB.Update(); err != nil {
				fmt.Println("Error updating task process to processing")
			}
			taskProcess = existingTB
		} else {
			taskProcess, err = models.NewTaskProcess(country, "", "", models.TaskProcessStatusProcessing, models.TaskProcessTypeCountry, task.ID)
			if err != nil {
				continue
			}
		}

		utils.SendWSTaskProcess(task.ID, taskProcess)

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

		filename, status, err := uploadMapFile(user, token, replaceData, path, data)
		if err != nil {
			fmt.Println("Error country first upload", country, err)
			// utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:failed", country))
			taskProcess.Status = models.TaskProcessStatusRetrying
			taskProcess.Update()
			utils.SendWSTaskProcess(task.ID, taskProcess)
			time.Sleep(time.Second * 2)
			filename, status, err = uploadMapFile(user, token, replaceData, downloadPath, data)
			if err != nil {
				fmt.Println("Error retrying for second time: ", country, err)
				taskProcess.Status = models.TaskProcessStatusFailed
				taskProcess.Update()
				utils.SendWSTaskProcess(task.ID, taskProcess)
				continue
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
		// utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:done:%s", country, status))

		taskProcess.FileName = filename
		taskProcess.Update()
		utils.SendWSTaskProcess(task.ID, taskProcess)

	}

	return nil
}

func processCountry(user *models.User, task *models.Task, token, chartName, country, title, startYear, endYear, downloadPath string, data StartData, chartParams map[string]string) error {
	l, browser := GetBrowser()
	blankPage := browser.MustPage("")

	defer blankPage.Close()
	defer l.Cleanup()
	defer browser.Close()

	var err error
	var taskProcess *models.TaskProcess
	// Try to find existing process, otherwise create one
	existingTB, err := models.FindTaskProcessByTaskRegionDate(country, "", task.ID)
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
		taskProcess, err = models.NewTaskProcess(country, "", "", models.TaskProcessStatusProcessing, models.TaskProcessTypeCountry, task.ID)
		if err != nil {
			return err
		}
	}

	utils.SendWSTaskProcess(task.ID, taskProcess)

	url := utils.AttachQueryParamToUrl(task.URL, fmt.Sprintf("tab=chart&country=~%s", country))
	if task.ChartParameters != "" {
		url = utils.AttachQueryParamToUrl(url, task.ChartParameters)
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
			page.Timeout(timeoutDuration).MustElement(START_MARKER_SELECTOR)
			// utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:processing", country))
			fmt.Println("After timeline startMarker")
			page.MustWaitElementsMoreThan(DOWNLOAD_BUTTON_SELECTOR, 0)
			fmt.Println("After download labesls")

			// title := page.MustElement(TITLE_SELECTOR).MustText()
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

func DownloadCountryGraphsFromPopover(url, outputDir string) map[string]string {
	fmt.Println("Downloading country graphs from popover")

	l, browser := GetBrowser()
	page := browser.MustPage("")
	defer page.Close()
	defer l.Cleanup()
	defer browser.Close()

	// Max 3 minutes for the process to complete
	page = page.Timeout(time.Minute * 3)
	page.MustSetViewport(1920, 1080, 1, false)
	page.MustNavigate(url)
	page.MustWaitLoad()
	page.MustWaitIdle()
	time.Sleep(time.Second * 2)
	fmt.Println("Url", page.MustInfo().URL)

	countries := make([]string, 0)
	countries = append(countries, "Russia")
	countries = append(countries, "China")
	countries = append(countries, "Brazil")

	countries = append(countries, "New-Zealand")
	countries = append(countries, "Chile")
	page.MustWaitElementsMoreThan(DOWNLOAD_BUTTON_SELECTOR, 0)

	foundCountries := make([]string, 0)
	notFoundCountries := make([]string, 0)
	gotSvg := make([]string, 0)
	result := make(map[string]string, 0)

	for name, code := range constants.COUNTRY_CODES {
		time.Sleep(time.Millisecond * 200)

		id := strings.ReplaceAll(name, " ", "-")
		has, _, err := page.Has(fmt.Sprintf("#%s", id))
		if err != nil {
			fmt.Println("Error finding: ", name, id)
			continue
		}

		if !has {
			notFoundCountries = append(notFoundCountries, name)
			continue

		}
		foundCountries = append(foundCountries, name)

		time.Sleep(time.Millisecond * 200)

		fmt.Println("Key: ", name, " Code: ", code, " ID: ", id)
		el, err := page.Element(fmt.Sprintf("#%s", id))
		if err != nil {
			fmt.Println("Error finding element: ", err)
			continue
		}
		shape := el.MustShape()

		page.Mouse.MustMoveTo(shape.OnePointInside().X, shape.OnePointInside().Y)
		time.Sleep(time.Millisecond * 200)
		if !page.MustHas("#mapTooltip svg") {
			fmt.Println("Country doesn't have chart to download: ", name)
			continue
		}

		countrySvg := page.MustElement("#mapTooltip svg")
		_, err = countrySvg.Eval(`(styles) => {
			// Attach style
			const styleEl = document.createElementNS('http://www.w3.org/2000/svg', 'style');
			styleEl.textContent = styles;
			this.insertBefore(styleEl, this.firstChild);
			// Add xmlns attribute for standalone SVG file compatibility
			this.setAttribute('xmlns', 'http://www.w3.org/2000/svg')
		}`, constants.COUNTRY_CHART_POPUP_STYLES)
		if err != nil {
			fmt.Println("Failed to inject styles: ", err)
		}

		html, err := countrySvg.HTML()
		if err != nil {
			fmt.Println("Error getting country svg html", name, err)
			continue
		}

		countryDirPath := path.Join(outputDir, code)

		if err := os.Mkdir(countryDirPath, 0755); err != nil {
			fmt.Println("Error creating output country dir: ", err)
			continue
		}
		outputPath := path.Join(countryDirPath, fmt.Sprintf("%s.svg", code))
		err = os.WriteFile(outputPath, []byte(html), 0644)
		if err != nil {
			fmt.Println("Error writing file: ", name, err)
			continue
		}

		gotSvg = append(gotSvg, name)
		result[code] = countryDirPath
	}

	fmt.Println("===========================")
	fmt.Println("Found countries", len(foundCountries))
	fmt.Println(foundCountries)
	fmt.Println("===========================")
	fmt.Println("Not Found countries", len(notFoundCountries))
	fmt.Println(notFoundCountries)
	fmt.Println("===========================")
	fmt.Println("Got SVGs", len(gotSvg))
	fmt.Println(gotSvg)

	return result
}

func GetCountryListFromPage(page *rod.Page) []string {
	countries := []string{}

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

	return countries
}

func GetCountryList(url string) ([]string, string, string, string, error) {
	// url := fmt.Sprintf("%s%s?tab=chart", constants.OWID_BASE_URL, chartName)
	l, browser := GetBrowser()
	blankPage := browser.MustPage("")

	defer blankPage.Close()
	defer l.Cleanup()
	defer browser.Close()

	countries := []string{}
	startYear := ""
	endYear := ""
	title := ""

	err := rod.Try(func() {
		page := browser.MustPage("")
		fmt.Println("Getting  country list")
		page.MustSetViewport(1920, 1080, 1, false)
		page.MustNavigate(url)
		page.MustWaitIdle()
		page.MustWaitLoad()

		page = page.Timeout(time.Second * constants.CHART_WAIT_TIME_SECONDS)
		page.MustWaitElementsMoreThan(DOWNLOAD_BUTTON_SELECTOR, 0)
		page.MustWaitElementsMoreThan(PLAY_TIMELAPSE_BUTTON_SELECTOR, 0)
		fmt.Println("waiting for entity selector")
		page.MustElement(".entity-selector__content")
		fmt.Println("found entity selector")
		title = page.MustElement(TITLE_SELECTOR).MustText()
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
		marker := page.MustElement(START_MARKER_SELECTOR)
		fmt.Println("Got marker", marker)
		startYear = *marker.MustAttribute("aria-valuemin")
		endYear = *marker.MustAttribute("aria-valuemax")
	})

	return countries, title, startYear, endYear, err
}
