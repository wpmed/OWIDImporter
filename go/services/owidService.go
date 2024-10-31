package services

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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

var COUNTRY_CODES = map[string]string{
	"Afghanistan":                       "AFG",
	"Åland Islands":                     "ALA",
	"Albania":                           "ALB",
	"Algeria":                           "DZA",
	"American Samoa":                    "ASM",
	"Andorra":                           "AND",
	"Angola":                            "AGO",
	"Anguilla":                          "AIA",
	"Antarctica":                        "ATA",
	"Antigua and Barbuda":               "ATG",
	"Argentina":                         "ARG",
	"Armenia":                           "ARM",
	"Aruba":                             "ABW",
	"Australia":                         "AUS",
	"Austria":                           "AUT",
	"Azerbaijan":                        "AZE",
	"Bahamas":                           "BHS",
	"Bahrain":                           "BHR",
	"Bangladesh":                        "BGD",
	"Barbados":                          "BRB",
	"Belarus":                           "BLR",
	"Belgium":                           "BEL",
	"Belize":                            "BLZ",
	"Benin":                             "BEN",
	"Bermuda":                           "BMU",
	"Bhutan":                            "BTN",
	"Bolivia":                           "BOL",
	"Bonaire, Sint Eustatius and Saba":  "BES",
	"Bosnia and Herzegovina":            "BIH",
	"Botswana":                          "BWA",
	"Bouvet Island":                     "BVT",
	"Brazil":                            "BRA",
	"British Indian Ocean Territory":    "IOT",
	"Brunei":                            "BRN",
	"Bulgaria":                          "BGR",
	"Burkina Faso":                      "BFA",
	"Burundi":                           "BDI",
	"Cambodia":                          "KHM",
	"Cameroon":                          "CMR",
	"Canada":                            "CAN",
	"Cape Verde":                        "CPV",
	"Cayman Islands":                    "CYM",
	"Central African Republic":          "CAF",
	"Chad":                              "TCD",
	"Chile":                             "CHL",
	"China":                             "CHN",
	"Christmas Island":                  "CXR",
	"Cocos (Keeling) Islands":           "CCK",
	"Colombia":                          "COL",
	"Comoros":                           "COM",
	"Congo":                             "COG",
	"Democratic Republic of Congo":      "COD",
	"Cook Islands":                      "COK",
	"Costa Rica":                        "CRI",
	"Cote d'Ivoire":                     "CIV",
	"Croatia":                           "HRV",
	"Cuba":                              "CUB",
	"Curaçao":                           "CUW",
	"Cyprus":                            "CYP",
	"Czechia":                           "CZE",
	"Denmark":                           "DNK",
	"Djibouti":                          "DJI",
	"Dominica":                          "DMA",
	"Dominican Republic":                "DOM",
	"Ecuador":                           "ECU",
	"Egypt":                             "EGY",
	"El Salvador":                       "SLV",
	"Equatorial Guinea":                 "GNQ",
	"Eritrea":                           "ERI",
	"Estonia":                           "EST",
	"Ethiopia":                          "ETH",
	"Falkland Islands (Malvinas)":       "FLK",
	"Faroe Islands":                     "FRO",
	"Fiji":                              "FJI",
	"Finland":                           "FIN",
	"France":                            "FRA",
	"French Guiana":                     "GUF",
	"French Polynesia":                  "PYF",
	"French Southern Territories":       "ATF",
	"Gabon":                             "GAB",
	"Gambia":                            "GMB",
	"Georgia":                           "GEO",
	"Germany":                           "DEU",
	"Ghana":                             "GHA",
	"Gibraltar":                         "GIB",
	"Greece":                            "GRC",
	"Greenland":                         "GRL",
	"Grenada":                           "GRD",
	"Guadeloupe":                        "GLP",
	"Guam":                              "GUM",
	"Guatemala":                         "GTM",
	"Guernsey":                          "GGY",
	"Guinea":                            "GIN",
	"Guinea-Bissau":                     "GNB",
	"Guyana":                            "GUY",
	"Haiti":                             "HTI",
	"Heard Island and McDonald Islands": "HMD",
	"Holy See (Vatican City State)":     "VAT",
	"Honduras":                          "HND",
	"Hong Kong":                         "HKG",
	"Hungary":                           "HUN",
	"Iceland":                           "ISL",
	"India":                             "IND",
	"Indonesia":                         "IDN",
	"Iran":                              "IRN",
	"Iraq":                              "IRQ",
	"Ireland":                           "IRL",
	"Isle of Man":                       "IMN",
	"Israel":                            "ISR",
	"Italy":                             "ITA",
	"Jamaica":                           "JAM",
	"Japan":                             "JPN",
	"Jersey":                            "JEY",
	"Jordan":                            "JOR",
	"Kazakhstan":                        "KAZ",
	"Kenya":                             "KEN",
	"Kiribati":                          "KIR",
	"North Korea":                       "PRK",
	"South Korea":                       "KOR",
	"Kuwait":                            "KWT",
	"Kyrgyzstan":                        "KGZ",
	"Laos":                              "LAO",
	"Latvia":                            "LVA",
	"Lebanon":                           "LBN",
	"Lesotho":                           "LSO",
	"Liberia":                           "LBR",
	"Libya":                             "LBY",
	"Liechtenstein":                     "LIE",
	"Lithuania":                         "LTU",
	"Luxembourg":                        "LUX",
	"Macao":                             "MAC",
	"North Macedonia":                   "MKD",
	"Madagascar":                        "MDG",
	"Malawi":                            "MWI",
	"Malaysia":                          "MYS",
	"Maldives":                          "MDV",
	"Mali":                              "MLI",
	"Malta":                             "MLT",
	"Marshall Islands":                  "MHL",
	"Martinique":                        "MTQ",
	"Mauritania":                        "MRT",
	"Mauritius":                         "MUS",
	"Mayotte":                           "MYT",
	"Mexico":                            "MEX",
	"Micronesia (country)":              "FSM",
	"Moldova":                           "MDA",
	"Monaco":                            "MCO",
	"Mongolia":                          "MNG",
	"Montenegro":                        "MNE",
	"Montserrat":                        "MSR",
	"Morocco":                           "MAR",
	"Mozambique":                        "MOZ",
	"Myanmar":                           "MMR",
	"Namibia":                           "NAM",
	"Nauru":                             "NRU",
	"Nepal":                             "NPL",
	"Netherlands":                       "NLD",
	"New Caledonia":                     "NCL",
	"New Zealand":                       "NZL",
	"Nicaragua":                         "NIC",
	"Niger":                             "NER",
	"Nigeria":                           "NGA",
	"Niue":                              "NIU",
	"Norfolk Island":                    "NFK",
	"Northern Mariana Islands":          "MNP",
	"Norway":                            "NOR",
	"Oman":                              "OMN",
	"Pakistan":                          "PAK",
	"Palau":                             "PLW",
	"Palestinian Territory, Occupied":   "PSE",
	"Panama":                            "PAN",
	"Papua New Guinea":                  "PNG",
	"Paraguay":                          "PRY",
	"Peru":                              "PER",
	"Philippines":                       "PHL",
	"Pitcairn":                          "PCN",
	"Poland":                            "POL",
	"Portugal":                          "PRT",
	"Puerto Rico":                       "PRI",
	"Qatar":                             "QAT",
	"Réunion":                           "REU",
	"Romania":                           "ROU",
	"Russia":                            "RUS",
	"Rwanda":                            "RWA",
	"Saint Barthélemy":                  "BLM",
	"Saint Helena, Ascension and Tristan da Cunha": "SHN",
	"Saint Kitts and Nevis":                        "KNA",
	"Saint Lucia":                                  "LCA",
	"Saint Martin (French part)":                   "MAF",
	"Saint Pierre and Miquelon":                    "SPM",
	"Saint Vincent and the Grenadines":             "VCT",
	"Samoa":                                        "WSM",
	"San Marino":                                   "SMR",
	"Sao Tome and Principe":                        "STP",
	"Saudi Arabia":                                 "SAU",
	"Senegal":                                      "SEN",
	"Serbia":                                       "SRB",
	"Seychelles":                                   "SYC",
	"Sierra Leone":                                 "SLE",
	"Singapore":                                    "SGP",
	"Sint Maarten (Dutch part)":                    "SXM",
	"Slovakia":                                     "SVK",
	"Slovenia":                                     "SVN",
	"Solomon Islands":                              "SLB",
	"Somalia":                                      "SOM",
	"South Africa":                                 "ZAF",
	"South Georgia and the South Sandwich Islands": "SGS",
	"South Sudan":                          "SSD",
	"Spain":                                "ESP",
	"Sri Lanka":                            "LKA",
	"Sudan":                                "SDN",
	"Suriname":                             "SUR",
	"Svalbard and Jan Mayen":               "SJM",
	"Eswatini":                             "SWZ",
	"Sweden":                               "SWE",
	"Switzerland":                          "CHE",
	"Syria":                                "SYR",
	"Taiwan, Province of China":            "TWN",
	"Tajikistan":                           "TJK",
	"Tanzania":                             "TZA",
	"Thailand":                             "THA",
	"East Timor":                           "TLS",
	"Togo":                                 "TGO",
	"Tokelau":                              "TKL",
	"Tonga":                                "TON",
	"Trinidad and Tobago":                  "TTO",
	"Tunisia":                              "TUN",
	"Turkey":                               "TUR",
	"Turkmenistan":                         "TKM",
	"Turks and Caicos Islands":             "TCA",
	"Tuvalu":                               "TUV",
	"Uganda":                               "UGA",
	"Ukraine":                              "UKR",
	"United Arab Emirates":                 "ARE",
	"United Kingdom":                       "GBR",
	"United States":                        "USA",
	"United States Minor Outlying Islands": "UMI",
	"Uruguay":                              "URY",
	"Uzbekistan":                           "UZB",
	"Vanuatu":                              "VUT",
	"Venezuela":                            "VEN",
	"Vietnam":                              "VNM",
	"Virgin Islands, British":              "VGB",
	"Virgin Islands, U.S.":                 "VIR",
	"Wallis and Futuna":                    "WLF",
	"Western Sahara":                       "ESH",
	"Yemen":                                "YEM",
	"Zambia":                               "ZMB",
	"Zimbabwe":                             "ZWE",
}

type StartMapData struct {
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

func ValidateParameters(data StartMapData) error {
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

func StartMap(session *sessions.Session, data StartMapData) error {
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
	g.SetLimit(5)

	utils.SendWSMessage(session, "debug", "Fetching country list")
	countriesList, err := GetCountryList(chartName)

	utils.SendWSMessage(session, "debug", fmt.Sprintf("Fetched %d countries. Countries are %s", len(countriesList), countriesList))
	if err != nil {
		fmt.Println("Error fetching country list", err)
		return err
	}
	fmt.Println("Countries:====================== ", countriesList)

	startTime := time.Now()
	for _, country := range countriesList {
		country := country
		g.Go(func(country, downloadPath string) func() error {
			return func() error {
				processCountry(session, token, chartName, country, downloadPath, data)
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

func processCountry(session *sessions.Session, token, chartName, country, downloadPath string, data StartMapData) error {
	url := fmt.Sprintf("%s%s?tab=chart&country=~%s", constants.OWID_BASE_URL, chartName, country)
	utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:processing", country))
	control := launcher.New().Set("--no-sandbox").HeadlessNew(true).MustLaunch()
	browser := rod.New().ControlURL(control).MustConnect()
	defer browser.Close()
	fmt.Println("Processing", url)

	var page *rod.Page
	var err error
	// Retry 2 times
	for i := 0; i < constants.RETRY_COUNT; i++ {
		page = browser.MustPage("")
		page.MustSetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: env.GetEnv().OWID_UA})
		page.MustNavigate(url)
		err = rod.Try(func() {
			page.Timeout(constants.CHART_WAIT_TIME).MustElement(".timeline-component .startMarker")
		})
		if err != nil {
			utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:failed", country))
			fmt.Println(country, "timeout waiting for start marker", err)
			utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:retrying", country))
			page.Close()
		} else {
			err = nil
			break
		}
	}

	if err != nil {
		utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:failed", country))
		return err
	}
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
		return err
	}
	err = page.MustElement(`button[data-track-note="chart_download_svg"]`).Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:failed", country))
		fmt.Println(country, "Error clicking download svg button", err)
		return err
	}

	wait()
	if _, err := os.Stat(downloadPath); os.IsNotExist(err) {
		utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:failed", country))
		fmt.Println(country, "File not found", err)
		return err
	}

	status, err := uploadMapFile(session, token, chartName, country, *startYear, *endYear, title, downloadPath, data)
	if err != nil {
		utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:failed", country))
		return err
	}
	utils.SendWSMessage(session, "progress", fmt.Sprintf("%s:done:%s", country, status))
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

func uploadMapFile(session *sessions.Session, token string, chartName string, country string, startYear, endYear string, title string, downloadPath string, data StartMapData) (string, error) {
	fileInfo, err := getFileInfo(downloadPath)
	if err != nil {
		return "", err
	}
	replaceData := ReplaceVarsData{
		Url:       data.Url,
		Title:     title,
		Region:    country,
		StartYear: startYear,
		EndYear:   endYear,
		FileName:  chartName,
	}
	filedesc := replaceVars(data.Description, replaceData)
	filename := replaceVars(data.FileName, replaceData)

	res, err := utils.DoApiReq[QueryResponse](session, map[string]string{
		"action": "query",
		"prop":   "imageinfo",
		"titles": "File:" + filename,
		"iiprop": "sha1",
	}, nil)
	if err != nil {
		return "", err
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
			return "", err
		}
		if res.Upload.Result == "Success" {
			return "uploaded", nil
		}
		return "", fmt.Errorf("upload failed: %s", res.Upload.Result)

	} else if len(page.ImageInfo) > 0 && page.ImageInfo[0].SHA1 == fileInfo.Sha1 {
		// Already uploaded
		fmt.Println("Skipping", filename)
		return "skipped", nil
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
			return "", err
		}
		if res.Upload.Result == "Success" {
			return "overwritten", nil
		}
		return "", fmt.Errorf("upload failed: %s", res.Upload.Result)
	}
}

type FileInfo struct {
	File []byte
	Name string
	Sha1 string
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
		File: fileContents,
		Name: name,
		Sha1: sha1sum,
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
	control := launcher.New().Set("--no-sandbox").HeadlessNew(true).MustLaunch()
	browser := rod.New().ControlURL(control).MustConnect()
	defer browser.Close()
	page := browser.MustPage(url)

	page = page.Timeout(time.Second * 10)
	countries := []string{}
	err := rod.Try(func() {
		fmt.Println("waiting for entity selector")
		page.MustElement(".entity-selector__content")
		fmt.Println("found entity selector")
		elements := page.MustElements(".entity-selector__content li")
		for _, element := range elements {
			country := element.MustText()
			countryCode, ok := COUNTRY_CODES[country]
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
