package services

import (
	"fmt"
	"os"
	"path"
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

func uploadCountryChart(user *models.User, token *string, replaceData ReplaceVarsData, countryDownloadPath string, data StartData) (string, string, error) {
	oldFileNameFormatMatcher := "$NAME, $START_YEAR $REGION.svg"
	/**
		Check if the country graph was uploaded before with a past year (year != endYear)
		Old files had the format "$NAME, $START_YEAR to $END_YEAR, $REGION.svg"
		We will search by "$NAME, $START_YEAR $REGION.svg"
		$END_YEAR is excluded as it might have changed
	**/
	searchFileName := strings.TrimSpace("File:" + replaceVars(oldFileNameFormatMatcher, replaceData))
	newFileName := strings.TrimSpace("File:" + replaceVars(data.FileName, replaceData))
	titles, err := SearchPageWithPrefix(user, searchFileName)
	if err == nil && len(titles) > 0 {
		fmt.Println("============ MATCHED FILES WITH OLD PREFIX: ", titles)
		for _, title := range titles {
			existingTitle := title
			newFileNameTrimmed := strings.ToLower(strings.TrimSpace(newFileName))
			existingTitleTrimmed := strings.ToLower(strings.TrimSpace(existingTitle))
			/**
				Check if not the same file
			**/
			sameFileName := strings.EqualFold(newFileNameTrimmed, existingTitleTrimmed)
			hasRegionSuffix := strings.HasSuffix(existingTitleTrimmed, fmt.Sprintf("%s.svg", strings.ToLower(replaceData.Region)))
			hasSameChartTitle := strings.Contains(existingTitleTrimmed, strings.ToLower(replaceData.FileName))
			hasStartingYear := strings.Contains(existingTitleTrimmed, strings.ToLower(replaceData.StartYear))
			if !sameFileName && hasRegionSuffix && hasSameChartTitle && hasStartingYear {
				fmt.Println("Move: Not same file name, has region suffix, has same chart title, has starting year")
				// Check if it's an OWID file by checking for "Uploaded by OWID Importer tool" category
				pageText, err := getPageWikiText(user, title)
				if err != nil {
					fmt.Println("Move: Error getting move title wikitext", title, err)
					continue
				}

				lowercasePageText := strings.ToLower(pageText)
				hasOwidCategory := strings.Contains(lowercasePageText, "category:uploaded by owid importer tool")
				if !hasOwidCategory {
					fmt.Println("Move: doesn't have owid category, skipping")
					continue
				}
				fmt.Println("Move: have owid category Category:uploaded by owid importer")

				// Check if it has the chart url in the wikitext
				chartUrlParts := strings.Split(replaceData.Url, "?")
				if len(chartUrlParts) == 0 {
					fmt.Println("Move: can't get chart url, skipping for safety")
					continue
				}

				hasChartUrl := strings.Contains(lowercasePageText, strings.ToLower(chartUrlParts[0]))
				if !hasChartUrl {
					fmt.Println("Move: doesn't have owid chart url, skipping")
					continue
				}
				fmt.Println("Move: have chart url", chartUrlParts[0])

				// Move the page to the newFileName
				fmt.Println("============== MOVING TO THE NEW NAME", newFileName)
				if err := MovePage(user, *token, existingTitle, newFileName); err != nil {
					fmt.Println("Error moving country page from old title to new title", existingTitle, newFileName, err)
				} else {
					fmt.Println("============ Moved From: ", existingTitle, " to: ", newFileName)
				}
				time.Sleep(time.Second * 2)
				break
			}
		}
	}

	filename, status, err := uploadMapFile(user, *token, replaceData, countryDownloadPath, data)
	return filename, status, err
}

func ProcessCountriesFromPopover(user *models.User, task *models.Task, chartName, title, startYear, endYear, downloadPath string, data StartData, chartParams map[string]string) error {
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

	url := utils.AttachQueryParamToUrl(task.URL, fmt.Sprintf("tab=map"))
	if task.ChartParameters != "" {
		url = utils.AttachQueryParamToUrl(url, task.ChartParameters)
	}
	if !strings.Contains(url, "time=") {
		url = utils.AttachQueryParamToUrl(url, "time=latest")
	}

	models.UpdateTaskLastOperationAt(task.ID)
	result := DownloadCountryGraphsFromPopover(url, downloadPath)
	models.UpdateTaskLastOperationAt(task.ID)
	countryCodeNameMap := constants.GetCountryCodeNameMap()

	for country, path := range result {
		if task.Status != models.TaskStatusProcessing {
			break
		}

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

		if name, exists := countryCodeNameMap[country]; exists {
			replaceData.RegionName = name
		} else {
			replaceData.RegionName = country
		}

		filename, status, err := uploadCountryChart(user, &token, replaceData, path, data)
		if err != nil {
			fmt.Println("Error country first upload", country, err)
			// utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:failed", country))
			taskProcess.Status = models.TaskProcessStatusRetrying
			taskProcess.Update()
			utils.SendWSTaskProcess(task.ID, taskProcess)
			time.Sleep(time.Second * 2)
			filename, status, err = uploadCountryChart(user, &token, replaceData, path, data)
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

func TraverseDownloadCountriesList(user *models.User, task *models.Task, token *string, chartName, title, startYear, endYear, downloadPath string, data StartData, chartParams map[string]string, countriesCodes []string, countryNames map[string]string) error {
	if len(countriesCodes) == 0 {
		return nil
	}

	if task.Status != models.TaskStatusProcessing {
		return fmt.Errorf("Task is not processing")
	}

	url, err := countryChartURL(task.URL, task.ChartParameters, countriesCodes[0])
	if err != nil {
		return err
	}

	fmt.Println("================== Processing country: ", url)

	l, browser := GetBrowser()
	blankPage := browser.MustPage("")

	defer blankPage.Close()
	defer l.Cleanup()
	defer browser.Close()

	page := browser.MustPage("")
	page.MustSetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: env.GetEnv().OWID_UA})
	page.MustSetViewport(constants.VIEWPORT_WIDTH, constants.VIEWPORT_HEIGHT, 1, false)
	page.MustNavigate(url)
	// utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:processing", country))
	page.MustWaitLoad()
	page.MustWaitIdle()
	if err := utils.WaitElementWithTimeout(page, DOWNLOAD_BUTTON_SELECTOR, time.Second*10); err != nil {
		return fmt.Errorf("cannot find download button: %w", err)
	}

	lineTab, _ := GetTabByLabel(page, "line")
	chartTab, _ := GetTabByLabel(page, "chart")

	if lineTab != nil {
		lineTab.Click(proto.InputMouseButtonLeft, 1)
		time.Sleep(time.Second)
	} else if chartTab != nil {
		chartTab.Click(proto.InputMouseButtonLeft, 1)
		time.Sleep(time.Second)
	} else {
		return fmt.Errorf("Cannot find line/chart tabs")
	}

	countriesCodeNameMap := constants.GetCountryCodeNameMap()

	for _, code := range countriesCodes {
		if task.Status != models.TaskStatusProcessing {
			break
		}

		name, found := countriesCodeNameMap[code]
		if !found {
			fmt.Println("Not found")
			continue
		}
		fmt.Println("Processing code: ", code, name)
		var taskProcess *models.TaskProcess
		// Try to find existing process, otherwise create one
		existingTB, err := models.FindTaskProcessByTaskRegionDate(code, "", task.ID)
		if existingTB != nil {
			if existingTB.Status != models.TaskProcessStatusFailed {
				continue
			}
			existingTB.Status = models.TaskProcessStatusProcessing
			if err := existingTB.Update(); err != nil {
				fmt.Println("Error updating task process to processing")
			}
			taskProcess = existingTB
		} else {
			taskProcess, err = models.NewTaskProcess(code, "", "", models.TaskProcessStatusProcessing, models.TaskProcessTypeCountry, task.ID)
			if err != nil {
				fmt.Println("ERROR finding task process for country code", code, err)
				continue
			}
		}

		utils.SendWSTaskProcess(task.ID, taskProcess)
		models.UpdateTaskLastOperationAt(task.ID)

		if err := selectCountryFromPage(page, countryNames[code]); err != nil {
			FailTaskProcess(taskProcess)
			return fmt.Errorf("selecting chart for %s: %w", code, err)
		}

		countryDownloadPath := path.Join(downloadPath, code)

		if _, err := os.Stat(countryDownloadPath); err == nil {
			os.RemoveAll(countryDownloadPath)
		}

		if err := os.Mkdir(countryDownloadPath, 0755); err != nil {
			fmt.Println("Error creating download directory: ", code, err)
			FailTaskProcess(taskProcess)
			continue
		}
		wait := page.Browser().WaitDownload(countryDownloadPath)

		if err := utils.WaitElementWithTimeout(page, DOWNLOAD_BUTTON_SELECTOR, time.Second*5); err != nil {
			fmt.Println(code, "Cannot find download button", err)
			FailTaskProcess(taskProcess)
			continue
		}

		downloadBtn := page.MustElement(DOWNLOAD_BUTTON_SELECTOR)
		downloadBtn.MustFocus()
		time.Sleep(time.Millisecond * 200)

		if err := page.Keyboard.Press(input.Enter); err != nil {
			fmt.Println(code, "Error clicking download button", err)
			FailTaskProcess(taskProcess)
			continue
		}

		fmt.Println("GOT DOWNLOAD BTN SELECTOR, WAITING FOR SVG")
		if err := utils.WaitElementWithTimeout(page, DOWNLOAD_SVG_ICON_SELECTOR, time.Second*10); err != nil {
			fmt.Println("Can't find DOWNLOAD_SVG_SELECTOR")
			FailTaskProcess(taskProcess)
			CloseDownloadPopup(page)
			continue
		}
		fmt.Println("DONE WAITING FOR DOWNLOAD ICON")

		elements := page.MustElements(DOWNLOAD_SVG_BUTTON_SELECTOR)

		fmt.Println("GOT DOWNLOAD ICON")
		if err := elements[0].Click(proto.InputMouseButtonLeft, 1); err != nil {
			// utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:failed", country))
			fmt.Println(code, "Error clicking download svg button", err)
			FailTaskProcess(taskProcess)
			CloseDownloadPopup(page)
			continue
		}

		wait()
		fmt.Println("============= DOWNLOAD DONE =============")

		CloseDownloadPopup(page)

		if _, err := os.Stat(countryDownloadPath); os.IsNotExist(err) {
			// utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:failed", country))
			FailTaskProcess(taskProcess)
			fmt.Println(code, "File not found", err)
			continue
		}

		replaceData := ReplaceVarsData{
			Url:       data.Url,
			Title:     title,
			Region:    code,
			StartYear: startYear,
			EndYear:   endYear,
			FileName:  GetFileNameFromChartName(chartName),
			Comment:   "Importing from " + data.Url,
			Params:    chartParams,
		}

		if name != "" {
			replaceData.RegionName = name
		} else {
			replaceData.RegionName = code
		}

		filename, status, err := uploadCountryChart(user, token, replaceData, countryDownloadPath, data)
		if err != nil {
			FailTaskProcess(taskProcess)
			continue
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

		time.Sleep(time.Millisecond * 200)
	}
	fmt.Println("===================== COUNTRIES ALL DONE ==========================")

	return nil
}

// getCountryNamesFromPage is the compatibility fallback when historical CSV
// discovery is unavailable. It reads names without filtering selected-year values.
func getCountryNamesFromPage(page *rod.Page) ([]string, map[string]string, error) {
	page = page.Timeout(10 * time.Second)
	search, err := page.Element(COUNTRY_SEARCH_INPUT)
	if err != nil {
		return nil, nil, fmt.Errorf("finding sidebar search: %w", err)
	}
	if err := search.SelectAllText(); err != nil {
		return nil, nil, err
	}
	if err := search.Input(""); err != nil {
		return nil, nil, err
	}
	// Allow both the regular sidebar and explorer search to restore their lists.
	time.Sleep(200 * time.Millisecond)
	items, err := page.Elements(COUNTRY_SEARCH_RESULT_LIST)
	if err != nil {
		return nil, nil, err
	}
	labels := make([]string, 0, len(items))
	for _, item := range items {
		text, err := item.Text()
		if err != nil {
			return nil, nil, err
		}
		labels = append(labels, text)
	}
	return countryNamesFromLabels(labels)
}

// selectCountryFromPage switches entities in the existing page using OWID's
// own name. The selected-year value (including "No data") is irrelevant.
func selectCountryFromPage(page *rod.Page, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("missing OWID entity name")
	}
	selectionPage := page.Timeout(15 * time.Second)
	search, err := selectionPage.Element(COUNTRY_SEARCH_INPUT)
	if err != nil {
		return fmt.Errorf("finding entity search: %w", err)
	}
	// Clear the previous query so it cannot hide selected options in explorers.
	if err := search.SelectAllText(); err != nil {
		return err
	}
	if err := search.Input(""); err != nil {
		return err
	}
	time.Sleep(100 * time.Millisecond)
	for attempts := 0; ; attempts++ {
		selected, err := selectionPage.Elements(COUNTRY_SELECTED_OPTIONS_LIST)
		if err != nil {
			return err
		}
		if len(selected) == 0 {
			break
		}
		if attempts >= 100 {
			return fmt.Errorf("could not clear selected entities")
		}
		if err := selected[0].Click(proto.InputMouseButtonLeft, 1); err != nil {
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := search.Input(name); err != nil {
		return err
	}
	var option *rod.Element
	for attempts := 0; attempts < 50 && option == nil; attempts++ {
		items, err := selectionPage.Elements(COUNTRY_SEARCH_RESULT_LIST)
		if err != nil {
			return err
		}
		for _, item := range items {
			text, err := item.Text()
			if err != nil {
				return err
			}
			if strings.EqualFold(strings.TrimSpace(text), name) {
				option = item
				break
			}
		}
		if option == nil {
			time.Sleep(100 * time.Millisecond)
		}
	}
	if option == nil {
		return fmt.Errorf("cannot find OWID entity %q", name)
	}
	if err := option.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return err
	}
	for attempts := 0; attempts < 50; attempts++ {
		selected, err := selectionPage.Elements(COUNTRY_SELECTED_OPTIONS_LIST)
		if err != nil {
			return err
		}
		if len(selected) == 1 {
			text, err := selected[0].Text()
			if err != nil {
				return err
			}
			if strings.EqualFold(strings.TrimSpace(text), name) {
				time.Sleep(200 * time.Millisecond)
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("could not confirm selection of %q", name)
}

func DownloadCountryGraphsFromPopover(url, outputDir string) map[string]string {
	fmt.Println("Downloading country graphs from popover", url)

	l, browser := GetBrowser()
	page := browser.MustPage("")
	defer page.Close()
	defer l.Cleanup()
	defer browser.Close()

	// Max 3 minutes for the process to complete
	page = page.Timeout(time.Minute * 3)
	page.MustSetViewport(constants.VIEWPORT_WIDTH, constants.VIEWPORT_HEIGHT, 1, false)
	page.MustNavigate(url)
	page.MustWaitLoad()
	page.MustWaitIdle()
	time.Sleep(time.Second * 2)
	fmt.Println("Url", page.MustInfo().URL)

	result := make(map[string]string, 0)
	if err := utils.WaitElementWithTimeout(page, DOWNLOAD_BUTTON_SELECTOR, time.Second*5); err != nil {
		fmt.Println("Timeout waiting for download btn")
		return result
	}

	foundCountries := make([]string, 0)
	notFoundCountries := make([]string, 0)
	gotSvg := make([]string, 0)

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

		el, err := page.Element(fmt.Sprintf("#%s", id))
		if err != nil {
			fmt.Println("Error finding element: ", err)
			continue
		}
		shape := el.MustShape()

		if shape == nil {
			fmt.Println("Doesn't have shape: ", name)
			continue
		}

		pointInside := shape.OnePointInside()
		if pointInside == nil {
			fmt.Println("Doesn't have point inside: ", name)
			continue
		}

		page.Mouse.MustMoveTo(pointInside.X, pointInside.Y)

		if err := utils.WaitElementWithTimeout(page, MAP_TOOLTIP_SELECTOR, time.Millisecond*500); err != nil {
			fmt.Println("Country doesn't have chart to download: ", name)
			continue
		}

		countrySvg := page.MustElement(MAP_TOOLTIP_SELECTOR)
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
