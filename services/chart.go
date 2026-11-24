package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
	"github.com/wpmed-videowiki/OWIDImporter/constants"
	"github.com/wpmed-videowiki/OWIDImporter/models"
	"github.com/wpmed-videowiki/OWIDImporter/utils"
)

func uploadCountryChart(user *models.User, token *string, replaceData ReplaceVarsData, countryDownloadPath string, data StartData) (string, string, error) {
	/**
		Check if the country graph was uploaded before with a past year (year != endYear)
		Old files had the format "$NAME, $START_YEAR to $END_YEAR, $REGION.svg"
		We list files starting with "$NAME, $START_YEAR to "
		$END_YEAR is excluded as it might have changed
	**/
	oldNamePrefix := replaceVars("$NAME, $START_YEAR to ", replaceData)
	newFileName := strings.TrimSpace("File:" + replaceVars(data.FileName, replaceData))
	titles, err := SearchPageWithPrefix(user, oldNamePrefix)
	// Some old files used the OWID slug as name ("Number-of-deaths-from-tetanus, 1980 to 2021, IND.svg")
	if slugName := strings.ReplaceAll(replaceData.FileName, " ", "-"); err == nil && slugName != replaceData.FileName {
		var slugTitles []string
		slugTitles, err = SearchPageWithPrefix(user, strings.Replace(oldNamePrefix, replaceData.FileName, slugName, 1))
		titles = append(titles, slugTitles...)
	}
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
			isOldNameOfThisFile := isOldFormatTitleFor(existingTitle, replaceData.FileName, replaceData.StartYear, replaceData.Region)
			if !sameFileName && isOldNameOfThisFile {
				fmt.Println("Move: Not same file name, title is the old name of this chart, start year and region")
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

				// Check the wikitext points at this exact chart (path and chart-defining params)
				if !pageIsForChart(pageText, replaceData.Url) {
					fmt.Println("Move: page is not for chart", replaceData.Url, "skipping")
					continue
				}
				fmt.Println("Move: have chart url", replaceData.Url)

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
	result, popoverErr := DownloadCountryGraphsFromPopover(url, downloadPath)
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

	if popoverErr != nil {
		// The countries it never reached have no rows; the task must not end looking complete
		return fmt.Errorf("country popovers: %v: %w", popoverErr, errIncomplete)
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

	// The worker's own window in the shared browser, released on every path
	w, err := openBrowserWindow()
	if err != nil {
		return err
	}
	defer w.Close()

	// The country being downloaded (a browser crash puts it back, failed, so the rerun retries
	// it) and the countries given up after their retry, which later passes skip
	items := newPassItems("country", func(p *models.TaskProcess) string { return p.Region })
	// Countries already done are skipped, so a rerun in a relaunched browser neither redoes nor re-uploads them
	// onFail fails the country left "processing" by a crash, a stuck window, an error or an early stop
	err = w.runItems("countries from "+countriesCodes[0], func(p *rod.Page) error {
		page := p
		// Not ready (load timeout, controls missing): retried in a fresh window, never a silent stop
		if err := loadChartPage(page, url); err != nil {
			return fmt.Errorf("loading %s: %v: %w", url, err, errPageNotReady)
		}
		// utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:processing", country))
		if err := utils.WaitElementWithTimeout(page, DOWNLOAD_BUTTON_SELECTOR, time.Second*10); err != nil {
			return fmt.Errorf("cannot find download button: %w", errPageNotReady)
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
			return fmt.Errorf("Cannot find line/chart tabs: %w", errPageNotReady)
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
			if items.skip(code) {
				continue // failed after its retry
			}
			// The previous country's download dialog must be gone: behind it the next country
			// can't be picked, and its export could be the previous chart under this name. Checked
			// before this country starts, so a stuck dialog is charged to the country that left it
			// (still the one in progress) and that one is retried, then given up.
			if err := CloseDownloadPopup(page); err != nil {
				return fmt.Errorf("after the country before %s: %w", code, err)
			}
			fmt.Println("Processing code: ", code, name)
			var taskProcess *models.TaskProcess
			// Try to find existing process, otherwise create one
			existingTB, err := models.FindTaskProcessByTaskRegionDate(code, "", task.ID)
			if existingTB != nil {
				// Done, or failed on its own attempt in this run: stepped past
				if existingTB.Status != models.TaskProcessStatusFailed || !items.retryFailed(code) {
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

			items.start(taskProcess)
			utils.SendWSTaskProcess(task.ID, taskProcess)
			models.UpdateTaskLastOperationAt(task.ID)

			if err := selectCountryFromPage(page, countryNames[code]); err != nil {
				FailTaskProcess(taskProcess)
				return fmt.Errorf("selecting chart for %s: %v: %w", code, err, errPageNotReady)
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
			if err := utils.WaitElementWithTimeout(page, DOWNLOAD_BUTTON_SELECTOR, time.Second*5); err != nil {
				fmt.Println(code, "Cannot find download button", err)
				FailTaskProcess(taskProcess)
				// Retried once in a fresh window, then failed while the rest go on
				return fmt.Errorf("download button for %s: %w", code, errPageNotReady)
			}

			downloadBtn := page.MustElement(DOWNLOAD_BUTTON_SELECTOR)
			downloadBtn.MustFocus()
			time.Sleep(time.Millisecond * 200)

			if err := page.Keyboard.Press(input.Enter); err != nil {
				fmt.Println(code, "Error clicking download button", err)
				FailTaskProcess(taskProcess)
				return fmt.Errorf("opening the download dialog for %s: %v: %w", code, err, errPageNotReady)
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
			if _, err := captureOWIDSVGToDir(page, countryDownloadPath, svgCaptureTimeout, clickElement(elements[0])); err != nil {
				// utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:failed", country))
				fmt.Println(code, "Error downloading svg", err)
				FailTaskProcess(taskProcess)
				if !windowResponds(page) {
					// The window or the browser broke: runWithCrashRetry replaces it and reruns
					// the pass, which retries this country (countries done are skipped)
					return fmt.Errorf("downloading %s: %w", code, errWindowStuck)
				}
				CloseDownloadPopup(page)
				continue
			}
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
			if taskProcess.Status != models.TaskProcessStatusFailed {
				// Only a country done in full resets the window's stuck-retry count
				w.itemDone()
			}

			time.Sleep(time.Millisecond * 200)
		}
		fmt.Println("===================== COUNTRIES ALL DONE ==========================")

		return nil
	}, items.onFail, items.giveUp)
	if err != nil {
		// The page could not be worked at all: the countries not done yet get failed rows, which
		// the task shows and can retry, and the other workers and the regions go on
		fmt.Println("ERROR processing countries, failing the ones left: ", countriesCodes, err)
		failUnfinishedCountries(task, countriesCodes)
	}
	return nil
}

// failUnfinishedCountries gives every country of codes that is not done a failed row.
func failUnfinishedCountries(task *models.Task, codes []string) {
	for _, code := range codes {
		existing, _ := models.FindTaskProcessByTaskRegionDate(code, "", task.ID)
		if existing != nil {
			if itemUnfinished(existing) {
				FailTaskProcess(existing)
			}
			continue
		}
		if p, err := models.NewTaskProcess(code, "", "", models.TaskProcessStatusFailed, models.TaskProcessTypeCountry, task.ID); err == nil {
			utils.SendWSTaskProcess(task.ID, p)
		} else {
			fmt.Println("ERROR recording failed country", code, err)
		}
	}
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

// DownloadCountryGraphsFromPopover saves each country's popover chart and returns their dirs by
// country code. The error is for a pass that could not go over the whole map (the countries it
// reached are still returned).
func DownloadCountryGraphsFromPopover(url, outputDir string) (map[string]string, error) {
	fmt.Println("Downloading country graphs from popover", url)

	result := make(map[string]string, 0)
	// Its own window in the shared browser, released on every path
	w, err := openBrowserWindow()
	if err != nil {
		fmt.Println("Error opening browser window: ", err)
		return result, err
	}
	defer w.Close()
	// A rerun in a relaunched browser when the browser dies, or in a fresh window when the page
	// is not ready or the pass runs out of time; countries already saved are kept
	if err := w.runWithCrashRetry("country popovers "+url, func(p *rod.Page) error {
		var passErr error
		err := rod.Try(func() { passErr = popoverPass(p, url, outputDir, result) })
		if err == nil {
			err = passErr
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("country popovers: %v: %w", err, errPageNotReady)
		}
		return err
	}, nil); err != nil {
		fmt.Println("Error downloading country graphs from popover: ", err)
		return result, err
	}
	return result, nil
}

// popoverPass is one pass of DownloadCountryGraphsFromPopover over the map, skipping the
// countries in result already.
func popoverPass(p *rod.Page, url, outputDir string, result map[string]string) error {
	{
		page := p

		// Max 3 minutes for the process to complete
		page = page.Timeout(time.Minute * 3)
		page.MustSetViewport(constants.VIEWPORT_WIDTH, constants.VIEWPORT_HEIGHT, 1, false)
		page.MustNavigate(url)
		page.MustWaitLoad()
		page.MustWaitIdle()
		time.Sleep(time.Second * 2)
		fmt.Println("Url", page.MustInfo().URL)

		if err := utils.WaitElementWithTimeout(page, DOWNLOAD_BUTTON_SELECTOR, time.Second*5); err != nil {
			fmt.Println("Timeout waiting for download btn")
			return fmt.Errorf("country popovers: no download button: %w", errPageNotReady)
		}

		foundCountries := make([]string, 0)
		notFoundCountries := make([]string, 0)
		gotSvg := make([]string, 0)

		for name, code := range constants.COUNTRY_CODES {
			if _, done := result[code]; done {
				continue // saved by a pass before a browser crash
			}
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

			if err := os.MkdirAll(countryDirPath, 0755); err != nil {
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

		return nil
	}
}
