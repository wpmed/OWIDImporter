package services

import (
	"context"
	"fmt"
	"html"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
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

	for _, region := range constants.REGIONS {
		if task.Status == models.TaskStatusFailed {
			break
		}
		region := region
		err := processRegion(user, task, &token, chartName, region, filepath.Join(tmpDir, region), data)
		if err != nil {
			fmt.Println("Error in processing some of the region", region)
			fmt.Println(err)
		}
	}

	// utils.SendWSMessage(session, "debug", fmt.Sprintf("Finished in %s", time.Since(startTime).String()))
	// SendTemplate(taskId, user)
	if task.Status == models.TaskStatusProcessing {
		task.Status = models.TaskStatusDone
		if err := task.Update(); err != nil {
			fmt.Println("Error saving task staus to done: ", err)
		}
	}

	utils.SendWSTask(task)

	return nil
}

func downloadMapData(url, dataPath, metadataPath, mapPath string) error {
	l := launcher.New()
	defer l.Cleanup()

	control := l.Set("--no-sandbox").HeadlessNew(HEADLESS).MustLaunch()
	browser := rod.New().ControlURL(control).MustConnect()
	defer browser.Close()

	page := browser.MustPage("")

	dataUrl := ""
	metadataUrl := ""

	router := browser.HijackRequests()
	defer router.MustStop()

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
		}
	})
	go router.Run()

	page.MustSetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: env.GetEnv().OWID_UA})
	fmt.Println("Before navigate")
	page.MustNavigate(url)

	time.Sleep(time.Second * 3)
	if dataUrl == "" || metadataUrl == "" {
		return fmt.Errorf("Error getting data/metadata urls")
	}

	fmt.Println("DOWNLOADING DATA")
	if err := utils.DownloadFile(dataUrl, dataPath); err != nil {
		return fmt.Errorf("ERROR DOWNLOADING DATA JSON %v", err)
	}
	fmt.Println("DOWNLOADING METADATA")
	if err := utils.DownloadFile(metadataUrl, metadataPath); err != nil {
		return fmt.Errorf("ERROR DOWNLOADING METADATA JSON %v", err)
	}

	fmt.Println("DOWNLOADING MAP FILE")
	if err := downloadChartFile(url, mapPath); err != nil {
		return err
	}

	return nil
}

func getMapStartEndYearTitle(chartName, region string) (string, string, string) {
	url := ""
	if region == "World" {
		// World chart has no region parameter
		url = fmt.Sprintf("%s%s", constants.OWID_BASE_URL, chartName)
	} else {
		url = fmt.Sprintf("%s%s?region=%s", constants.OWID_BASE_URL, chartName, region)
	}

	l := launcher.New()
	defer l.Cleanup()

	control := l.Set("--no-sandbox").HeadlessNew(HEADLESS).MustLaunch()
	browser := rod.New().ControlURL(control).MustConnect()
	page := browser.MustPage("")

	defer browser.Close()

	startYear := ""
	endYear := ""
	title := ""

	for i := 0; i < constants.RETRY_COUNT; i++ {
		err := rod.Try(func() {
			page = page.Timeout(constants.CHART_WAIT_TIME_SECONDS * time.Second)
			page.MustSetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: env.GetEnv().OWID_UA})
			page.MustNavigate(url)
			page.MustWaitIdle()
			marker := page.MustElement(".handle.startMarker")
			startYear = *marker.MustAttribute("aria-valuemin")
			endYear = *marker.MustAttribute("aria-valuemax")
			title = page.MustElement("h1.header__title").MustText()
		})

		if err != nil {
			// utils.SendWSMessage(session, "debug", fmt.Sprintf("%s:failed", region))
			page.Close()
			page = browser.MustPage("")
		} else {
			break
		}
	}

	return startYear, endYear, title
}

func processRegion(user *models.User, task *models.Task, token *string, chartName, region, downloadPath string, data StartData) error {
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
		url = fmt.Sprintf("%s%s", constants.OWID_BASE_URL, chartName)
	} else {
		url = fmt.Sprintf("%s%s?region=%s", constants.OWID_BASE_URL, chartName, region)
	}

	startYear, endYear, title := getMapStartEndYearTitle(chartName, region)

	if startYear == "" || endYear == "" {
		// utils.SendWSMessage(session, "debug", fmt.Sprintf("%s:failed", region))
		return fmt.Errorf("failed to get start and end years")
	}

	startYearInt, err := strconv.ParseInt(startYear, 10, 64)
	if err != nil {
		// utils.SendWSMessage(session, "debug", fmt.Sprintf("%s:failed", region))
		return fmt.Errorf("failed to parse start year: %v", err)
	}
	endYearInt, err := strconv.ParseInt(endYear, 10, 64)
	if err != nil {
		// utils.SendWSMessage(session, "debug", fmt.Sprintf("%s:failed", region))
		return fmt.Errorf("failed to parse end year: %v", err)
	}

	// try to get JSON data
	dataPath := path.Join(downloadPath, "data.json")
	metadataPath := path.Join(downloadPath, "metadata.json")
	mapPath := path.Join(downloadPath, "map")
	err = downloadMapData(url, dataPath, metadataPath, mapPath)

	// Fallback to the old flow of getting each image individually
	if err != nil {
		fmt.Println("ERROR GETTING CHART JSON DATA", err)

		g, _ := errgroup.WithContext(context.Background())
		g.SetLimit(constants.CONCURRENT_REQUESTS)

		// var filename string
		for year := startYearInt; year <= endYearInt; year++ {
			year := year
			g.Go(func(region string, year int, downloadPath string) func() error {
				return func() error {
					if task.Status != models.TaskStatusFailed {
						err := processRegionYear(user, task, *token, chartName, title, region, downloadPath, year, data, "")
						return err
					}
					return nil
				}
			}(region, int(year), filepath.Join(downloadPath, strconv.FormatInt(year, 10))))
		}

		if err := g.Wait(); err != nil {
			return err
		}

		task.Reload()
		if task.Status != models.TaskStatusFailed {
			// Attach country data to the first file metadata on commons
			metadata, err := getRegionFileMetadata(task, region)
			if err != nil {
				fmt.Println("Error generating metadata: ", err)
			} else if metadata != "" {
				// fmt.Println("GOT METADATA, YAAAAAAAAAAY: ", metadata)
				fmt.Println("GOT METADATA, YAAAAAAAAAAY")
				err := processRegionYear(user, task, *token, chartName, title, region, filepath.Join(downloadPath, strconv.FormatInt(startYearInt, 10)+"_final"), int(startYearInt), data, metadata)
				if err != nil {
					fmt.Println("Error uploading with metadata: ", err)
				}
			}
		}
	} else {
		fmt.Println("DOWNLOADED CHART DATA", dataPath, downloadPath)
		mapInfo, err := getFileInfo(mapPath)
		if err != nil {
			return err
		}
		fmt.Println("Map info: ", mapInfo.FilePath)
		err = generateAndprocessRegionYearsNewFlow(user, task, *token, chartName, title, region, int(startYearInt), int(endYearInt), data, dataPath, metadataPath, mapPath)
		if err != nil {
			return err
		}
	}

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

func downloadChartFile(url, downloadPath string) error {
	l := launcher.New()
	defer l.Cleanup()

	// control := l.Set("--no-sandbox").Headless(false).MustLaunch()
	control := l.Set("--no-sandbox").HeadlessNew(HEADLESS).MustLaunch()
	browser := rod.New().ControlURL(control).MustConnect()
	defer browser.Close()
	timeoutDuration := time.Duration(constants.CHART_WAIT_TIME_SECONDS) * time.Second
	page := browser.MustPage("")

	err := rod.Try(func() {
		page = page.Timeout(timeoutDuration)
		page.MustNavigate(url)
		page.MustWaitIdle()

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

func processRegionYearNewFlow(user *models.User, task *models.Task, data StartData, token, region, title, chartName, mapDir, fileMetadata string, year int) error {
	var err error
	var taskProcess *models.TaskProcess
	mapPath := path.Join(mapDir, strconv.Itoa(year))
	fmt.Println("Map path: ", mapPath)

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
		taskProcess, err = models.NewTaskProcess(region, year, "", models.TaskProcessStatusProcessing, task.ID)
		if err != nil {
			fmt.Println("Error creating new task process: ", region, year, err)
			return err
		}
	}

	utils.SendWSTaskProcess(task.ID, taskProcess)

	// control := l.Set("--no-sandbox").Headless(false).MustLaunch()
	url := ""
	if region == "World" {
		// World chart has no region parameter
		url = fmt.Sprintf("%s%s?time=%d&tab=map", constants.OWID_BASE_URL, chartName, year)
	} else {
		url = fmt.Sprintf("%s%s?region=%s&time=%d&tab=map", constants.OWID_BASE_URL, chartName, region, year)
	}
	fmt.Println(url)
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
		FileName: chartName,
	}

	fileInfo, err := getFileInfo(mapPath)
	if err != nil {
		fmt.Println("Error getting file info: ", err)
		return err
	}

	if fileMetadata != "" {
		if err := InjectMetadataIntoSVGSameFile(fileInfo.FilePath, fileMetadata); err != nil {
			fmt.Println("Error injecting metadata into svg: ", err)
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

	return nil
}

func generateAndprocessRegionYearsNewFlow(user *models.User, task *models.Task, token, chartName, title, region string, startYear, endYear int, data StartData, dataPath, metadataPath, mapDir string) error {
	fileinfo, err := getFileInfo(mapDir)
	if err != nil {
		return err
	}

	_, err = owidparser.GenerateImages(title, dataPath, metadataPath, fileinfo.FilePath, mapDir)
	if err != nil {
		return err
	}

	for year := startYear; year <= endYear; year++ {
		if task.Status == models.TaskStatusFailed {
			break
		}
		processRegionYearNewFlow(user, task, data, token, region, title, chartName, mapDir, "", year)

		time.Sleep(time.Millisecond * 2000)
	}

	task.Reload()
	if task.Status != models.TaskStatusFailed {
		// Attach country data to the first file metadata on commons
		metadata, err := getRegionFileMetadata(task, region)
		if err != nil {
			fmt.Println("Error generating metadata: ", err)
		} else if metadata != "" {
			// fmt.Println("GOT METADATA, YAAAAAAAAAAY: ", metadata)
			fmt.Println("GOT METADATA, YAAAAAAAAAAY")
			err := processRegionYearNewFlow(user, task, data, token, region, title, chartName, mapDir, metadata, startYear)
			if err != nil {
				fmt.Println("Error uploading with metadata: ", err)
			}
		}
	}

	return nil
}

func processRegionYear(user *models.User, task *models.Task, token, chartName, title, region, downloadPath string, year int, data StartData, fileMetadata string) error {
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
		taskProcess, err = models.NewTaskProcess(region, year, "", models.TaskProcessStatusProcessing, task.ID)
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
	fmt.Println(url)
	regionStr := region
	if regionStr == "NorthAmerica" {
		regionStr = "North America"
	}
	if regionStr == "SouthAmerica" {
		regionStr = "South America"
	}

	var filename string
	for i := 1; i <= constants.RETRY_COUNT; i++ {
		err = rod.Try(func() {
			if err = downloadChartFile(url, downloadPath); err != nil {
				panic(err)
			}
			fmt.Println("Finished", year, title)

			replaceData := ReplaceVarsData{
				Url:      data.Url,
				Title:    title,
				Region:   regionStr,
				Year:     strconv.Itoa(year),
				FileName: chartName,
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
