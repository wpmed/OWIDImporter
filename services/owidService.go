package services

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/wpmed-videowiki/OWIDImporter/models"
	"github.com/wpmed-videowiki/OWIDImporter/utils"
)

type StartData struct {
	Url                           string                               `json:"url"`
	FileName                      string                               `json:"fileName"`
	Description                   string                               `json:"desc"`
	DescriptionOverwriteBehaviour models.DescriptionOverwriteBehaviour `json:"description_overwrite_behaviour"`
}

type CountryTemplateDataItem struct {
	FileName string
	Country  string
}

type TokenResponse struct {
	BatchComplete string `json:"batchcomplete"`
	Query         struct {
		Tokens struct {
			CsrfToken string `json:"csrftoken"`
		} `json:"tokens"`
	} `json:"query"`
}

type FileNameAcc struct {
	FileName string
	Region   string
	Year     int64
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

type FileRevisionsResponse struct {
	BatchComplete bool `json:"batchcomplete"`
	Query         struct {
		Normalized []struct {
			FromEncoded bool   `json:"fromencoded"`
			From        string `json:"from"`
			To          string `json:"to"`
		} `json:"normalized"`
		Pages []FileRevisionsPage `json:"pages"`
	} `json:"query"`
}

type FileRevisionsPage struct {
	PageID    int        `json:"pageid"`
	Namespace int        `json:"ns"`
	Title     string     `json:"title"`
	Revisions []Revision `json:"revisions,omitempty"`
}

type Revision struct {
	Slots map[string]ContentSlot `json:"slots"`
}

type ContentSlot struct {
	ContentModel  string `json:"contentmodel"`
	ContentFormat string `json:"contentformat"`
	Content       string `json:"content"`
}

const (
	DOWNLOAD_BUTTON_SELECTOR = `figure div[data-track-note="chart_click_download"] button`
	HEADLESS                 = true
)

func GetChartNameFromUrl(url string) (string, error) {
	re := regexp.MustCompile(`^https://ourworldindata.org/grapher/([-a-z_0-9]+)(\?.*)?$`)
	matches := re.FindStringSubmatch(url)
	if matches == nil {
		return "", fmt.Errorf("invalid url")
	}
	return matches[1], nil
}

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

func getFileWikiText(user *models.User, filename string) (string, error) {
	res, err := utils.DoApiReq[FileRevisionsResponse](user, map[string]string{
		"action":  "query",
		"titles":  "File:" + filename,
		"prop":    "revisions",
		"rvprop":  "content",
		"rvlimit": "1",
		"rvslots": "main",
	}, nil)
	if err != nil {
		return "", err
	}

	fmt.Println("Response: ", res)
	for _, page := range res.Query.Pages {
		if len(page.Revisions) > 0 {
			// Assuming the main slot contains the wikitext
			if mainSlot, ok := page.Revisions[0].Slots["main"]; ok {
				return mainSlot.Content, nil
			}
		}
	}

	return "", nil
}

func extractCategories(wikitext string) []string {
	// Create a regular expression to match full MediaWiki category tags
	// Pattern: [[Category:Any text that doesn't include closing brackets]]
	re := regexp.MustCompile(`\[\[Category:[^\]]+\]\]`)

	// Find all matches (full category tags)
	matches := re.FindAllString(wikitext, -1)

	return matches
}

func uploadMapFile(user *models.User, token string, replaceData ReplaceVarsData, downloadPath string, data StartData) (string, string, error) {
	filedesc := replaceVars(data.Description, replaceData)
	filename := replaceVars(data.FileName, replaceData)

	fileInfo, err := getFileInfo(downloadPath)
	if err != nil {
		return filename, "", err
	}
	fmt.Println("Uploading file: ", fileInfo.FilePath)

	res, err := utils.DoApiReq[QueryResponse](user, map[string]string{
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

	fmt.Println("Got page info", page.ImageInfo)
	// Doesn't exist, upload and update description directly
	if len(page.ImageInfo) == 0 {
		fmt.Println("Uploading", filename)
		// Do upload
		res, err := utils.DoApiReq[UploadResponse](user, map[string]string{
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
	}

	// Page already exists
	var wikiText string
	newFileDesc := strings.TrimSpace(filedesc)
	switch data.DescriptionOverwriteBehaviour {

	case models.DescriptionOverwriteBehaviourExceptCategories:
		wikiText, err = getFileWikiText(user, filename)
		if err == nil {
			// Remove all user incoming categories
			incomingCategories := extractCategories(filedesc)
			for _, text := range incomingCategories {
				newFileDesc = strings.ReplaceAll(newFileDesc, text, "")
			}

			newFileDesc = strings.TrimSpace(newFileDesc)
			// Apply existing file categories
			existingCategories := extractCategories(wikiText)
			for _, text := range existingCategories {
				newFileDesc = newFileDesc + "\n" + text
			}

		} else {
			fmt.Println("ERROR GETTING WIKITEXT: ", err)
			return filename, "", fmt.Errorf("Error getting wikitext for except-category overwrite")
		}

	case models.DescriptionOverwriteBehaviourOnlyFile:
		wikiText, err = getFileWikiText(user, filename)
		if err != nil {
			fmt.Println(" ", err)
			return filename, "", fmt.Errorf("Error getting wikitext for file-only overwrite")
		}

		newFileDesc = wikiText
	}

	// Already uploaded, just update the description if changed
	if len(page.ImageInfo) > 0 && page.ImageInfo[0].SHA1 == fileInfo.Sha1 {
		// We ddin't fetch the wikitext or it failed, try again
		if wikiText == "" {
			wikiText, err = getFileWikiText(user, filename)
		}

		params := map[string]string{
			"action":         "edit",
			"comment":        "Updating description from " + data.Url,
			"text":           newFileDesc,
			"title":          "File:" + filename,
			"ignorewarnings": "1",
			"token":          token,
		}

		if wikiText != "" && strings.Compare(strings.TrimSpace(wikiText), strings.TrimSpace(newFileDesc)) != 0 {
			fmt.Println("Updating description", filename)
			fmt.Println("Old Desc:\n", strings.TrimSpace(wikiText))
			fmt.Println("New Desc:\n", strings.TrimSpace(newFileDesc))

			res, err := utils.DoApiReq[interface{}](user, params, nil)
			if err != nil {
				fmt.Println("Error updating description: ", err, res)
			} else {
				fmt.Println("Description updated", *res)
				return filename, "description_updated", nil
			}
		}
		fmt.Println("Skipping", filename)
		return filename, "skipped", nil
	} else {
		// Image changed, Overwrite the file
		fmt.Println("Overwriting", filename, page.ImageInfo[0].SHA1, fileInfo.Sha1)
		res, err := utils.DoApiReq[UploadResponse](user, map[string]string{
			"action":         "upload",
			"comment":        "Re-importing from " + data.Url,
			"text":           newFileDesc,
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
		fmt.Println("Error uploading file", res)
		return filename, "", fmt.Errorf("%s", res.Upload.Result)
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
		return nil, fmt.Errorf("expected exactly 1 SVG file, found %d, %s", len(files), fileDirectory)
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

type TemplateElement struct {
	Region string
	Data   []FileNameAcc
}

func GetMapTemplate(taskId string) (string, error) {
	taskProcesses, err := models.FindTaskProcessesByTaskId(taskId)
	if err != nil {
		fmt.Println("Error getting task processes to send template ", taskId, err)
		return "", err
	}

	data := make([]TemplateElement, 0)

	// Accumilate regions
	regions := make(map[string]bool)
	for _, el := range taskProcesses {
		if !regions[el.Region] {
			regions[el.Region] = true
		}
	}

	for key := range regions {
		items := make([]FileNameAcc, 0)
		for _, tp := range taskProcesses {
			if tp.Region == key {
				items = append(items, FileNameAcc{
					Year:     int64(tp.Year),
					Region:   tp.Region,
					FileName: tp.FileName,
				})
			}
		}
		data = append(data, TemplateElement{
			Region: key,
			Data:   items,
		})
	}

	wikiText := strings.Builder{}
	wikiText.WriteString("{{owidslidersrcs|id=gallery|widths=640|heights=640\n")
	for _, el := range data {
		sort.SliceStable(el.Data, func(i, j int) bool {
			return el.Data[i].Year < el.Data[j].Year
		})

		wikiText.WriteString(fmt.Sprintf("|gallery-%s=\n", el.Region))
		for _, item := range el.Data {
			wikiText.WriteString(fmt.Sprintf("File:%s!year=%d\n", item.FileName, item.Year))
		}
	}

	wikiText.WriteString("}}\n")
	// utils.SendWSMessage(session, "wikitext", wikiText.String())
	return wikiText.String(), nil
}

func GetChartTemplate(taskId string) (string, error) {
	taskProcesses, err := models.FindTaskProcessesByTaskId(taskId)
	if err != nil {
		fmt.Println("Error getting task processes to send template ", taskId, err)
		return "", err
	}

	data := make([]CountryTemplateDataItem, 0)
	for _, p := range taskProcesses {
		data = append(data, CountryTemplateDataItem{
			Country:  p.Region,
			FileName: p.FileName,
		})
	}

	wikiText := strings.Builder{}
	wikiText.WriteString("|gallery-AllCountries=\n")

	for _, el := range data {
		wikiText.WriteString(fmt.Sprintf("File:%s!country=%s\n", el.FileName, el.Country))
	}

	wikiText.WriteString("\n")
	// utils.SendWSMessage(session, "wikitext_countries", wikiText.String())
	return wikiText.String(), nil
}
