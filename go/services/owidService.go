package services

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/wpmed-videowiki/OWIDImporter-go/constants"
	"github.com/wpmed-videowiki/OWIDImporter-go/env"
	"github.com/wpmed-videowiki/OWIDImporter-go/sessions"
	"github.com/wpmed-videowiki/OWIDImporter-go/utils"
	"golang.org/x/sync/errgroup"
)

type StartData struct {
	Url         string `json:"url"`
	FileName    string `json:"fileName"`
	Description string `json:"desc"`
}

func GetChartNameFromUrl(url string) (string, error) {
	re := regexp.MustCompile(`^https://ourworldindata.org/grapher/([-a-z_0-9]+)(\?.*)?$`)
	matches := re.FindStringSubmatch(url)
	if matches == nil {
		return "", fmt.Errorf("invalid url")
	}
	return matches[1], nil
}

var getBrowserMutex = sync.Mutex{}

func ValidateParameters(data StartData) error {
	if data.Url == "" || data.FileName == "" || data.Description == "" {
		return fmt.Errorf("missing information")
	}
	if !strings.HasPrefix(data.Url, "https://ourworldindata.org/grapher/") {
		return fmt.Errorf("invalid url")
	}
	chartName, err := GetChartNameFromUrl(data.Url)
	if err != nil || chartName == "" {
		return fmt.Errorf("invalid url")
	}

	return nil
}

type TokenResponse struct {
	BatchComplete string `json:"batchcomplete"`
	Query         struct {
		Tokens struct {
			CsrfToken string `json:"csrftoken"`
		} `json:"tokens"`
	} `json:"query"`
}

func StartMap(session *sessions.Session, data StartData) error {
	err := ValidateParameters(data)
	if err != nil {
		return err
	}
	chartName, err := GetChartNameFromUrl(data.Url)
	if err != nil || chartName == "" {
		return fmt.Errorf("invalid url")
	}

	fmt.Println("Chart Name:", chartName)
	utils.SendWSMessage(session, "msg", "Starting")
	utils.SendWSMessage(session, "debug", "Fetching Upload token")

	tokenResponse, err := utils.DoApiReq[TokenResponse](session, map[string]string{
		"action": "query",
		"meta":   "tokens",
		"format": "json",
	}, nil)
	if err != nil {
		fmt.Println("Error fetching edit token", err)
		return err
	}
	token := tokenResponse.Query.Tokens.CsrfToken
	fmt.Println("Edit token:", token)

	tmpDir, err := os.MkdirTemp("./tmp", "owid-exporter")
	if err != nil {
		fmt.Println("Error creating temp directory", err)
		return err
	}
	defer os.RemoveAll(tmpDir)

	startTime := time.Now()
	items := make([]TemplateElement, 0)

	for _, region := range constants.REGIONS {
		region := region
		result, err := processRegion(session, token, chartName, region, filepath.Join(tmpDir, region), data)
		if err != nil {
			fmt.Println("Error in processing some of the region", region)
			fmt.Println(err)
		}
		if result != nil {
			items = append(items, TemplateElement{
				Region: region,
				Data:   result,
			})
		}
	}

	utils.SendWSMessage(session, "debug", fmt.Sprintf("Finished in %s", time.Since(startTime).String()))
	SendTemplate(session, items)

	return nil
}

type FileNameAcc struct {
	Year     int64
	FileName string
	Region   string
}

func getRegionStartEndYears(session *sessions.Session, chartName, region string) (string, string) {
	l := launcher.New()
	control := l.Set("--no-sandbox").Headless(false).MustLaunch()
	browser := rod.New().ControlURL(control).MustConnect()

	defer l.Cleanup()
	defer browser.Close()

	utils.SendWSMessage(session, "debug", fmt.Sprintf("%s:processing", region))
	url := ""
	if region == "World" {
		// World chart has no region parameter
		url = fmt.Sprintf("%s%s", constants.OWID_BASE_URL, chartName)
	} else {
		url = fmt.Sprintf("%s%s?region=%s", constants.OWID_BASE_URL, chartName, region)
	}
	fmt.Println("Getting region start/end years", region, chartName, url)

	page := browser.MustPage("")
	page = page.Timeout(constants.CHART_WAIT_TIME_SECONDS * time.Second)

	startYear := ""
	endYear := ""

	for i := 0; i < constants.RETRY_COUNT; i++ {
		err := rod.Try(func() {
			page.MustSetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: env.GetEnv().OWID_UA})
			page.MustNavigate(url)
			fmt.Println("Navigated to ", url)
			marker := page.MustElement(".handle.startMarker")
			fmt.Println("Got marker")
			startYear = *marker.MustAttribute("aria-valuemin")
			endYear = *marker.MustAttribute("aria-valuemax")
			fmt.Println("Got start/end years", startYear, endYear)
		})
		page.Close()
		if err != nil {
			utils.SendWSMessage(session, "debug", fmt.Sprintf("%s:failed", region))
			page = browser.MustPage("")
			page = page.Timeout(constants.CHART_WAIT_TIME_SECONDS * time.Second)
		} else {
			break
		}
	}
	fmt.Println("Finished getting start/end year for region", region)
	return startYear, endYear
}
func processRegion(session *sessions.Session, token, chartName, region, downloadPath string, data StartData) ([]FileNameAcc, error) {
	// Get start and end years
	// get chart title
	// Process each year
	fmt.Println("Processing region", region)
	startYear, endYear := getRegionStartEndYears(session, chartName, region)
	fmt.Println("Got start and end years", startYear, endYear)

	if startYear == "" || endYear == "" {
		utils.SendWSMessage(session, "debug", fmt.Sprintf("%s:failed", region))
		return nil, fmt.Errorf("failed to get start and end years")
	}

	startYearInt, err := strconv.ParseInt(startYear, 10, 64)
	if err != nil {
		utils.SendWSMessage(session, "debug", fmt.Sprintf("%s:failed", region))
		return nil, fmt.Errorf("failed to parse start year: %v", err)
	}
	endYearInt, err := strconv.ParseInt(endYear, 10, 64)
	if err != nil {
		utils.SendWSMessage(session, "debug", fmt.Sprintf("%s:failed", region))
		return nil, fmt.Errorf("failed to parse end year: %v", err)
	}

	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(constants.CONCURRENT_REQUESTS)

	filenameAccumilator := make([]FileNameAcc, 0)
	accMutex := sync.Mutex{}

	var filename string
	browsers, l := createBrowserGroups()
	fmt.Println("Created Browser group", len(browsers))
	defer func() {
		l.Cleanup()
		for _, b := range browsers {
			b.Browser.Close()
		}
	}()
	for year := startYearInt; year <= endYearInt; year++ {
		year := year
		g.Go(func(region string, year int64, downloadPath string) func() error {
			return func() error {
				browser := getBrowser(browsers)
				fmt.Println("Got Browser")
				filename, err = processRegionYear(session, browser.Browser, token, chartName, region, downloadPath, strconv.FormatInt(year, 10), data)
				fmt.Println("Done region year", region, year)
				markBrowserAvailable(browser)
				fmt.Println("Marked browser as available")

				accMutex.Lock()
				filenameAccumilator = append(filenameAccumilator, FileNameAcc{Year: year, FileName: filename, Region: region})
				accMutex.Unlock()
				return err
			}
		}(region, year, filepath.Join(downloadPath, strconv.FormatInt(year, 10))))
	}

	if err := g.Wait(); err != nil {
		return filenameAccumilator, err
	}

	return filenameAccumilator, nil
}

func processRegionYear(session *sessions.Session, browser *rod.Browser, token, chartName, region, downloadPath, year string, data StartData) (string, error) {
	utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:processing:%s", region, year))

	url := ""
	if region == "World" {
		// World chart has no region parameter
		url = fmt.Sprintf("%s%s?time=%s", constants.OWID_BASE_URL, chartName, year)
	} else {
		url = fmt.Sprintf("%s%s?region=%s&time=%s", constants.OWID_BASE_URL, chartName, region, year)
	}
	fmt.Println(url)
	var page *rod.Page
	var err error
	var status string
	var filename string
	for i := 1; i <= constants.RETRY_COUNT; i++ {
		timeoutDuration := time.Duration(constants.CHART_WAIT_TIME_SECONDS*i) * time.Second
		page = browser.MustPage("")
		page = page.Timeout(timeoutDuration)
		page.MustNavigate(url)

		err = rod.Try(func() {
			page = page.Timeout(timeoutDuration)

			title := page.MustElement("h1.header__title").MustText()
			err = page.MustElement(`figure button[data-track-note="chart_click_download"]`).Click(proto.InputMouseButtonLeft, 1)
			if err != nil {
				fmt.Println(year, "Error clicking download button", err)
				utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:failed:%s", region, year))
				return
			}
			wait := page.Browser().WaitDownload(downloadPath)
			err = page.MustElement(`figure button[data-track-note="chart_download_svg"]`).Click(proto.InputMouseButtonLeft, 1)
			if err != nil {
				utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:failed:%s", region, year))
				fmt.Println(year, "Error clicking download svg button", err)
				return
			}

			wait()
			time.Sleep(time.Millisecond * 500)
			if _, err = os.Stat(downloadPath); os.IsNotExist(err) {
				utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:failed:%s", region, year))
				fmt.Println(year, "File not found", err)
				return
			}
			fmt.Println("Finished", year, title)

			replaceData := ReplaceVarsData{
				Url:      data.Url,
				Title:    title,
				Region:   region,
				Year:     year,
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

			filename, status, err = uploadMapFile(session, token, replaceData, downloadPath, data)
			if err != nil {
				utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:failed:%s", region, year))
				return
			}
		})

		page.Close()
		if err != nil {
			utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:failed:%s", region, year))
			fmt.Println(year, "timeout waiting for start marker", err)
			utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:retrying:%s", region, year))
		} else {
			err = nil
			break
		}
	}

	if err != nil {
		utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:failed:%s", region, year))
		return filename, err
	}
	utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:done:%s:%s", region, year, status))

	return filename, nil
}

type BrowserGroup struct {
	Browser   *rod.Browser
	Available bool
}

func getBrowser(browsers []*BrowserGroup) *BrowserGroup {
	for {
		getBrowserMutex.Lock()
		for _, bg := range browsers {
			if bg.Available {
				bg.Available = false
				getBrowserMutex.Unlock()
				return bg
			}
		}
		getBrowserMutex.Unlock()
		time.Sleep(time.Second * 500)
	}
}

func markBrowserAvailable(browser *BrowserGroup) {
	getBrowserMutex.Lock()
	browser.Available = true
	getBrowserMutex.Unlock()
}

func createBrowserGroups() ([]*BrowserGroup, *launcher.Launcher) {
	browsers := make([]*BrowserGroup, 0)
	l := launcher.New()
	control := l.Set("--no-sandbox").Headless(false).MustLaunch()
	//control := l.Set("--no-sandbox").HeadlessNew(true).MustLaunch()

	for i := 0; i < constants.CONCURRENT_REQUESTS; i++ {
		browser := rod.New().ControlURL(control).MustConnect()
		browsers = append(browsers, &BrowserGroup{
			Browser:   browser,
			Available: true,
		})
	}

	return browsers, l
}

func StartChart(session *sessions.Session, data StartData) error {
	err := ValidateParameters(data)
	if err != nil {
		return err
	}
	chartName, err := GetChartNameFromUrl(data.Url)
	if err != nil || chartName == "" {
		return fmt.Errorf("invalid url")
	}

	fmt.Println("Chart Name:", chartName)

	utils.SendWSMessage(session, "msg", "Starting")
	utils.SendWSMessage(session, "debug", "Fetching Upload token")

	tokenResponse, err := utils.DoApiReq[TokenResponse](session, map[string]string{
		"action": "query",
		"meta":   "tokens",
		"format": "json",
	}, nil)
	if err != nil {
		fmt.Println("Error fetching edit token", err)
		return err
	}
	token := tokenResponse.Query.Tokens.CsrfToken
	fmt.Println("Edit token:", token)

	tmpDir, err := os.MkdirTemp("./tmp", "owid-exporter")
	if err != nil {
		fmt.Println("Error creating temp directory", err)
		return err
	}
	defer os.RemoveAll(tmpDir)

	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(constants.CONCURRENT_REQUESTS)

	utils.SendWSMessage(session, "debug", "Fetching country list")
	countriesList, err := GetCountryList(chartName)

	utils.SendWSMessage(session, "debug", fmt.Sprintf("Fetched %d countries. Countries are %s", len(countriesList), countriesList))
	if err != nil {
		fmt.Println("Error fetching country list", err)
		return err
	}
	fmt.Println("Countries:====================== ", countriesList)

	browsers, l := createBrowserGroups()
	defer func() {
		l.Cleanup()
		for _, b := range browsers {
			b.Browser.Close()
		}

	}()
	startTime := time.Now()

	for _, country := range countriesList {
		country := country
		g.Go(func(country, downloadPath string) func() error {
			return func() error {
				bg := getBrowser(browsers)
				processCountry(session, bg.Browser, token, chartName, country, downloadPath, data)
				markBrowserAvailable(bg)
				return nil
			}
		}(country, filepath.Join(tmpDir, country)))
	}
	fmt.Println("Started in", time.Since(startTime).String())
	err = g.Wait()
	elapsedTime := time.Since(startTime)
	fmt.Println("Finished in", elapsedTime.String())
	if err != nil {
		fmt.Println("Error processing countries", err)
		return err
	}
	utils.SendWSMessage(session, "debug", fmt.Sprintf("Finished in %s", elapsedTime.String()))
	return nil
}

func processCountry(session *sessions.Session, browser *rod.Browser, token, chartName, country, downloadPath string, data StartData) error {
	url := fmt.Sprintf("%s%s?tab=chart&country=~%s", constants.OWID_BASE_URL, chartName, country)
	utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:processing", country))

	fmt.Println("Processing", url)

	var page *rod.Page
	var err error
	// Retry 2 times
	for i := 1; i <= constants.RETRY_COUNT; i++ {
		timeoutDuration := time.Duration(i*constants.CHART_WAIT_TIME_SECONDS) * time.Second
		page = browser.MustPage("")
		page.MustSetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: env.GetEnv().OWID_UA})
		page.MustNavigate(url)
		fmt.Println("Navigated to url", url)
		err = rod.Try(func() {
			page.Timeout(timeoutDuration).MustElement(".timeline-component .startMarker")
			utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:processing", country))
			page.MustWaitElementsMoreThan("figure #line-labels", 0)

			title := page.MustElement("h1.header__title").MustText()
			startYear := page.MustElement(".slider.clickable .handle.startMarker").MustAttribute("aria-valuenow")
			endYear := page.MustElement(".slider.clickable .handle.endMarker").MustAttribute("aria-valuenow")

			wait := page.Browser().WaitDownload(downloadPath)
			err = page.MustElement(`button[data-track-note="chart_click_download"]`).Click(proto.InputMouseButtonLeft, 1)
			if err != nil {
				fmt.Println(country, "Error clicking download button", err)
				utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:failed", country))
				return
			}
			err = page.MustElement(`button[data-track-note="chart_download_svg"]`).Click(proto.InputMouseButtonLeft, 1)
			if err != nil {
				utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:failed", country))
				fmt.Println(country, "Error clicking download svg button", err)
				return
			}

			wait()
			if _, err := os.Stat(downloadPath); os.IsNotExist(err) {
				utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:failed", country))
				fmt.Println(country, "File not found", err)
				return
			}

			replaceData := ReplaceVarsData{
				Url:       data.Url,
				Title:     title,
				Region:    country,
				StartYear: *startYear,
				EndYear:   *endYear,
				FileName:  chartName,
			}
			_, status, err := uploadMapFile(session, token, replaceData, downloadPath, data)
			if err != nil {
				utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:failed", country))
				return
			}
			utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:done:%s", country, status))
		})

		page.Close()
		if err != nil {
			utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:failed", country))
			fmt.Println(country, "timeout waiting for start marker", err)
			utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:retrying", country))
		} else {
			err = nil
			break
		}
	}

	if err != nil {
		utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:failed", country))
		return err
	}

	return nil
}

type QueryResponse struct {
	BatchComplete bool `json:"batchcomplete"`
	Query         struct {
		Normalized []struct {
			FromEncoded bool   `json:"fromencoded"`
			From        string `json:"from"`
			To          string `json:"to"`
		} `json:"normalized"`
		Pages []Page `json:"pages"`
	} `json:"query"`
}

type Page struct {
	PageID          int    `json:"pageid"`
	NS              int    `json:"ns"`
	Title           string `json:"title"`
	ImageRepository string `json:"imagerepository"`
	ImageInfo       []struct {
		SHA1 string `json:"sha1"`
	} `json:"imageinfo"`
}

type UploadResponse struct {
	Warnings struct {
		Main struct {
			Warnings string `json:"warnings"`
		} `json:"main"`
	} `json:"warnings"`
	Upload struct {
		Result   string `json:"result"`
		Filename string `json:"filename"`
		Warnings struct {
			Duplicate []string `json:"duplicate"`
		} `json:"warnings"`
		ImageInfo struct {
			Timestamp      string `json:"timestamp"`
			User           string `json:"user"`
			UserID         int    `json:"userid"`
			Size           int    `json:"size"`
			Width          int    `json:"width"`
			Height         int    `json:"height"`
			ParsedComment  string `json:"parsedcomment"`
			Comment        string `json:"comment"`
			HTML           string `json:"html"`
			CanonicalTitle string `json:"canonicaltitle"`
			URL            string `json:"url"`
			DescriptionURL string `json:"descriptionurl"`
			SHA1           string `json:"sha1"`
			Metadata       []struct {
				Name  string      `json:"name"`
				Value interface{} `json:"value"`
			} `json:"metadata"`
			CommonMetadata []interface{} `json:"commonmetadata"`
			ExtMetadata    struct {
				DateTime struct {
					Value  string `json:"value"`
					Source string `json:"source"`
					Hidden string `json:"hidden"`
				} `json:"DateTime"`
				ObjectName struct {
					Value  string `json:"value"`
					Source string `json:"source"`
				} `json:"ObjectName"`
			} `json:"extmetadata"`
			Mime      string `json:"mime"`
			MediaType string `json:"mediatype"`
			BitDepth  int    `json:"bitdepth"`
		} `json:"imageinfo"`
	} `json:"upload"`
}

func uploadMapFile(session *sessions.Session, token string, replaceData ReplaceVarsData, downloadPath string, data StartData) (string, string, error) {
	filedesc := replaceVars(data.Description, replaceData)
	filename := replaceVars(data.FileName, replaceData)

	fileInfo, err := getFileInfo(downloadPath)
	if err != nil {
		return filename, "", err
	}

	res, err := utils.DoApiReq[QueryResponse](session, map[string]string{
		"action": "query",
		"prop":   "imageinfo",
		"titles": "File:" + filename,
		"iiprop": "sha1",
	}, nil)
	if err != nil {
		return filename, "", err
	}
	pages := res.Query.Pages
	var page Page
	for _, p := range pages {
		page = p
		break
	}

	if len(page.ImageInfo) == 0 {
		fmt.Println("Uploading", filename)
		// Do upload
		res, err := utils.DoApiReq[UploadResponse](session, map[string]string{
			"action":         "upload",
			"comment":        "Importing from " + data.Url,
			"text":           filedesc,
			"filename":       filename,
			"ignorewarnings": "1",
			"token":          token,
		}, &utils.UploadedFile{
			Filename: filename,
			File:     string(fileInfo.File),
			Mime:     "image/svg+xml",
		})
		if err != nil {
			return filename, "", err
		}
		if res.Upload.Result == "Success" {
			return filename, "uploaded", nil
		}
		return filename, "", fmt.Errorf("upload failed: %s", res.Upload.Result)

	} else if len(page.ImageInfo) > 0 && page.ImageInfo[0].SHA1 == fileInfo.Sha1 {
		// Already uploaded
		fmt.Println("Skipping", filename)
		return filename, "skipped", nil
	} else {
		// Overwrite
		fmt.Println("Overwriting", filename)
		res, err := utils.DoApiReq[UploadResponse](session, map[string]string{
			"action":         "upload",
			"comment":        "Re-importing from " + data.Url,
			"text":           filedesc,
			"filename":       filename,
			"ignorewarnings": "1",
			"token":          token,
		}, &utils.UploadedFile{
			Filename: filename,
			File:     string(fileInfo.File),
			Mime:     "image/svg+xml",
		})
		if err != nil {
			return filename, "", err
		}
		if res.Upload.Result == "Success" {
			return filename, "overwritten", nil
		}
		return filename, "", fmt.Errorf("upload failed: %s", res.Upload.Result)
	}
}

type FileInfo struct {
	File     []byte
	Name     string
	Sha1     string
	FilePath string
}

func getFileInfo(fileDirectory string) (*FileInfo, error) {
	files, err := filepath.Glob(filepath.Join(fileDirectory, "*"))
	if err != nil {
		return nil, fmt.Errorf("error finding SVG files: %v", err)
	}

	if len(files) != 1 {
		return nil, fmt.Errorf("expected exactly 1 SVG file, found %d", len(files))
	}

	fileContents, err := os.ReadFile(files[0])
	if err != nil {
		return nil, fmt.Errorf("error reading file: %v", err)
	}

	// Remove external font imports that Commons doesn't allow
	re := regexp.MustCompile(`<style>@impo[^<]*</style>`)
	fileContents = re.ReplaceAll(fileContents, []byte(""))

	// Get just the filename without path or extension
	name := filepath.Base(files[0])
	name = strings.TrimSuffix(name, ".svg")

	// Calculate SHA1
	h := sha1.New()
	h.Write(fileContents)
	sha1sum := hex.EncodeToString(h.Sum(nil))

	return &FileInfo{
		File:     fileContents,
		Name:     name,
		Sha1:     sha1sum,
		FilePath: files[0],
	}, nil
}

type ReplaceVarsData struct {
	Url       string
	Title     string
	Year      string
	Region    string
	StartYear string
	EndYear   string
	FileName  string
}

func replaceVars(value string, params ReplaceVarsData) string {
	value = strings.ReplaceAll(value, "$URL", params.Url)
	value = strings.ReplaceAll(value, "$NAME", params.FileName)

	if params.Title != "" {
		value = strings.ReplaceAll(value, "$TITLE", params.Title)
	}
	if params.Year != "" {
		value = strings.ReplaceAll(value, "$YEAR", params.Year)
	}
	if params.Region != "" {
		value = strings.ReplaceAll(value, "$REGION", params.Region)
	}
	if params.StartYear != "" {
		value = strings.ReplaceAll(value, "$START_YEAR", params.StartYear)
	}
	if params.EndYear != "" {
		value = strings.ReplaceAll(value, "$END_YEAR", params.EndYear)
	}

	return value
}

func GetCountryList(chartName string) ([]string, error) {
	url := fmt.Sprintf("%s%s?tab=chart", constants.OWID_BASE_URL, chartName)
	l := launcher.New()
	defer l.Cleanup()

	control := l.Set("--no-sandbox").HeadlessNew(true).MustLaunch()
	browser := rod.New().ControlURL(control).MustConnect()
	defer browser.MustClose()
	page := browser.MustPage("")
	page.MustNavigate(url)
	fmt.Println("Getting  c ountry list")

	page = page.Timeout(time.Duration(constants.CHART_WAIT_TIME_SECONDS))
	countries := []string{}
	err := rod.Try(func() {
		fmt.Println("waiting for entity selector")
		page.MustElement(".entity-selector__content")
		fmt.Println("found entity selector")
		elements := page.MustElements(".entity-selector__content li")
		for _, element := range elements {
			country := element.MustText()
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
	})

	return countries, err
}

type TemplateElement struct {
	Region string
	Data   []FileNameAcc
}

func SendTemplate(session *sessions.Session, data []TemplateElement) {
	wikiText := strings.Builder{}
	wikiText.WriteString(`{{owidslidersrcs|id=gallery|widths=640|heights=640\n`)
	for _, el := range data {
		sort.SliceStable(el.Data, func(i, j int) bool {
			return el.Data[i].Year < el.Data[j].Year
		})

		wikiText.WriteString(fmt.Sprintf("|gallery-%s=\n", el.Region))
		for _, item := range el.Data {
			wikiText.WriteString(fmt.Sprintf("File:%s!year=%d\n", item.FileName, item.Year))
		}
	}

	wikiText.WriteString(`}}\n`)
	utils.SendWSMessage(session, "wikitext", wikiText.String())
	fmt.Println("Sent template", wikiText.String())
}
