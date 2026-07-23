package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dghubble/oauth1"
	"github.com/go-rod/rod"
	"github.com/wpmed-videowiki/OWIDImporter/env"
	"github.com/wpmed-videowiki/OWIDImporter/models"
	"github.com/wpmed-videowiki/OWIDImporter/sessions"
)

const MAX_RES_LOG_LENGTH = 200

func GetOAuthConfig() *oauth1.Config {
	envData := env.GetEnv()
	return &oauth1.Config{
		ConsumerKey:    envData.OWID_OAUTH_TOKEN,
		ConsumerSecret: envData.OWID_OAUTH_SECRET,
		CallbackURL:    "oob",
		Endpoint: oauth1.Endpoint{
			RequestTokenURL: envData.OWID_OAUTH_INITIATE,
			AuthorizeURL:    envData.OWID_OAUTH_AUTH,
			AccessTokenURL:  envData.OWID_OAUTH_TOKEN_URL,
		},
	}
}

func GetOAuthClient(user *models.User) *http.Client {
	return oauth1.NewClient(context.Background(), GetOAuthConfig(), &oauth1.Token{
		Token:       user.ResourceOwnerKey,
		TokenSecret: user.ResourceOwnerSecret,
	})
}

type UploadedFile struct {
	Filename string
	File     string
	Mime     string
}

func DoApiReq[T any](user *models.User, params map[string]string, file *UploadedFile) (*T, error) {
	client := GetOAuthClient(user)
	values := make(url.Values)
	url := env.GetEnv().OWID_MW_API + "?"
	for k, v := range params {
		if k != "token" && k != "text" {
			values.Set(k, v)
		}
	}
	if _, ok := values["format"]; !ok {
		values.Set("format", "json")
		values.Set("formatversion", "2")
	}
	url += values.Encode()

	var res *http.Response
	var err error

	if file != nil {
		var b bytes.Buffer
		writer := multipart.NewWriter(&b)
		defer writer.Close()
		fileWriter, err := writer.CreatePart(textproto.MIMEHeader{
			"Content-Disposition": []string{fmt.Sprintf("form-data; name=\"file\"; filename=\"%s\"", file.Filename)},
			"Content-Type":        []string{file.Mime},
		})
		if err != nil {
			fmt.Println("Error creating form file", err)
			return nil, err
		}
		_, err = fileWriter.Write([]byte(file.File))
		if err != nil {
			fmt.Println("Error writing file", err)
			return nil, err
		}
		// Add other fields to the form
		writer.WriteField("filename", file.Filename)

		for k, v := range params {
			writer.WriteField(k, v)
		}
		// Close the writer to complete the form data
		err = writer.Close()
		if err != nil {
			fmt.Println("Error closing writer", err)
			return nil, err
		}

		res, err = client.Post(url, "multipart/form-data; boundary="+writer.Boundary(), &b)
	} else if params["token"] != "" {
		var b bytes.Buffer
		writer := multipart.NewWriter(&b)
		defer writer.Close()

		for k, v := range params {
			writer.WriteField(k, v)
		}
		// Close the writer to complete the form data
		err = writer.Close()
		if err != nil {
			fmt.Println("Error closing writer", err)
			return nil, err
		}

		res, err = client.Post(url, "multipart/form-data; boundary="+writer.Boundary(), &b)
	} else {
		res, err = client.Get(url)
	}

	if err != nil {
		fmt.Println("Error", err)
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		fmt.Println("Error reading body", err)
		return nil, err
	}
	// strRes := string(body)
	// // if utf8.RuneCountInString(strRes) > MAX_RES_LOG_LENGTH {
	// // 	strRes = strRes[:MAX_RES_LOG_LENGTH] + "..."
	// // }
	// fmt.Println("Res body: ", res.StatusCode, strRes)

	var result T
	err = json.Unmarshal(body, &result)
	if err != nil {
		fmt.Println("Error unmarshalling api response", url, params, err)
		fmt.Println(string(body))
		return nil, err
	}
	return &result, nil
}

type UserInfo struct {
	BatchComplete string `json:"batchcomplete"`
	Query         struct {
		UserInfo struct {
			Name string `json:"name"`
			ID   int    `json:"id"`
		} `json:"userinfo"`
	} `json:"query"`
}

func GetUsername(user *models.User) (string, error) {
	result, err := DoApiReq[UserInfo](user, map[string]string{
		"action": "query",
		"meta":   "userinfo",
		"format": "json",
	}, nil)
	if err != nil {
		fmt.Println("Error doing API request", err)
		return "", err
	}

	return result.Query.UserInfo.Name, nil
}

func SendWSTaskProcess(taskId string, taskProcess *models.TaskProcess) error {
	msgJson, err := json.Marshal(taskProcess)
	if err != nil {
		fmt.Println("Error marshling json", err, taskProcess)
		return err
	}
	sendWSTaskMessage(taskId, "task_process", string(msgJson))

	return nil
}

func SendWSTask(task *models.Task) error {
	msgJson, err := json.Marshal(task)
	if err != nil {
		fmt.Println("Error marshling json", err, task)
		return err
	}
	sendWSTaskMessage(task.ID, "task", string(msgJson))
	sendWSTaskMessage(fmt.Sprintf("%s_task_list", task.UserId), "task", string(msgJson))
	return nil
}

func sendWSTaskMessage(taskId string, messageType string, msg string) {
	go func() {
		if len(sessions.SubscriptionSessions[taskId]) > 0 {
			failedSessions := make([]string, 0)
			for _, s := range sessions.SubscriptionSessions[taskId] {
				s.WsMutex.Lock()
				err := s.Ws.WriteJSON(map[string]string{
					"type": messageType,
					"msg":  msg,
				})
				if err != nil {
					fmt.Println("Error sending msg ", messageType, "-", msg, ": ", err)
					failedSessions = append(failedSessions, s.Id)
				}
				s.WsMutex.Unlock()
			}

			// Remove failed receives, most probably disconnected
			if len(failedSessions) > 0 {
				for _, id := range failedSessions {
					sessions.RemoveSubscriptionSession(taskId, id)
				}
			}
		}
	}()
}

func Contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func DownloadFile(url, filepath string) (err error) {
	// Create the file
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Get the data
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check server response
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	// Writer the body to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	return nil
}

func ToTitle(s string) string {
	return strings.ReplaceAll(strings.ToUpper(s[:1])+s[1:], "-", " ")
}

func ParseDate(dateStr string) (time.Time, error) {
	// Trim whitespace
	dateStr = strings.TrimSpace(dateStr)
	// Remove commas
	dateStr = strings.ReplaceAll(dateStr, ",", "")

	// Special handling for BCE/BC/AD/CE year notations
	// Matches patterns like "480 BCE", "480 BC", "2024 CE", "2024 AD"
	bcPattern := regexp.MustCompile(`^(\d{1,4})\s*(BCE?|AD|CE)$`)
	if matches := bcPattern.FindStringSubmatch(strings.ToUpper(dateStr)); matches != nil {
		year, err := strconv.Atoi(matches[1])
		if err == nil && year >= 0 && year <= 9999 {
			era := matches[2]
			// BCE/BC years are represented as negative in Go
			if era == "BCE" || era == "BC" {
				year = -year + 1 // Year 1 BCE is year 0, 2 BCE is year -1, etc.
			}
			return time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC), nil
		}
	}

	// Special handling for year-only strings (including years < 1000)
	if matched, _ := regexp.MatchString(`^\d{1,4}$`, dateStr); matched {
		year, err := strconv.Atoi(dateStr)
		if err == nil && year >= 0 && year <= 9999 {
			return time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC), nil
		}
	}

	// Common date formats to try
	formats := []string{
		// Year only (4 digits)
		"2006", // Just year (e.g., "2007")

		// Year and month
		"2006-01",      // YYYY-MM
		"2006/01",      // YYYY/MM
		"January 2006", // Month Year
		"Jan 2006",     // Short month Year
		"01/2006",      // MM/YYYY
		"01-2006",      // MM-YYYY

		// Full dates
		time.RFC3339,                // "2006-01-02T15:04:05Z07:00"
		time.RFC3339Nano,            // "2006-01-02T15:04:05.999999999Z07:00"
		time.RFC1123,                // "Mon, 02 Jan 2006 15:04:05 MST"
		time.RFC1123Z,               // "Mon, 02 Jan 2006 15:04:05 -0700"
		time.RFC822,                 // "02 Jan 06 15:04 MST"
		time.RFC822Z,                // "02 Jan 06 15:04 -0700"
		time.RFC850,                 // "Monday, 02-Jan-06 15:04:05 MST"
		"2006-01-02",                // ISO 8601 date only
		"2006-01-02 15:04:05",       // Common datetime format
		"2006-01-02T15:04:05",       // ISO 8601 without timezone
		"01/02/2006",                // US format (MM/DD/YYYY)
		"02/01/2006",                // European format (DD/MM/YYYY)
		"01-02-2006",                // US format with dashes
		"02-01-2006",                // European format with dashes
		"2006/01/02",                // Asian format (YYYY/MM/DD)
		"January 2, 2006",           // Long month name
		"Jan 2, 2006",               // Short month name
		"January 2 2006",            // Long month name, no comma
		"Jan 2 2006",                // Short month name, no comma
		"2 January 2006",            // Day first with long month
		"2 Jan 2006",                // Day first with short month
		"2006-01-02 15:04:05 MST",   // With timezone abbreviation
		"2006-01-02 15:04:05 -0700", // With timezone offset
	}

	// Try each format
	for _, format := range formats {
		if parsedTime, err := time.Parse(format, dateStr); err == nil {
			return parsedTime, nil
		}
	}

	// If all formats fail, return error
	return time.Time{}, fmt.Errorf("unable to parse date string: %s", dateStr)
}

// FormatDateLike formats t using the same format that dateStr was written in.
// dateStr should be a string that was successfully parsed by ParseDate.
// If the format can't be detected, it falls back to ISO 8601 (2006-01-02).
func FormatDateLike(t time.Time, dateStr string) string {
	trimmed := strings.TrimSpace(dateStr)

	// Special handling for BCE/BC/AD/CE year notations.
	// Preserve the era token and spacing exactly as the user wrote it.
	eraPattern := regexp.MustCompile(`^(\d{1,4})(\s*)(BCE?|AD|CE)$`)
	if matches := eraPattern.FindStringSubmatch(strings.ToUpper(trimmed)); matches != nil {
		year := t.Year()
		if era := matches[3]; era == "BCE" || era == "BC" {
			// Reverse of: year = -year + 1
			year = 1 - year
		}
		// Recover the era text with its original casing/spacing from the input
		origEra := trimmed[len(matches[1]):]
		// Preserve zero-padding width of the original year, if any
		return fmt.Sprintf("%0*d%s", len(matches[1]), year, origEra)
	}

	// Special handling for year-only strings (including years < 1000)
	if matched, _ := regexp.MatchString(`^\d{1,4}$`, trimmed); matched {
		return fmt.Sprintf("%0*d", len(trimmed), t.Year())
	}

	// Same layout list as ParseDate, plus the comma variants so that
	// inputs like "January 2, 2006" keep their comma in the output.
	formats := []string{
		"2006",
		"2006-01", "2006/01", "January 2006", "Jan 2006", "01/2006", "01-2006",
		time.RFC3339, time.RFC3339Nano, time.RFC1123, time.RFC1123Z,
		time.RFC822, time.RFC822Z, time.RFC850,
		"2006-01-02", "2006-01-02 15:04:05", "2006-01-02T15:04:05",
		"01/02/2006", "02/01/2006", "01-02-2006", "02-01-2006", "2006/01/02",
		"January 2, 2006", "Jan 2, 2006",
		"January 2 2006", "Jan 2 2006",
		"2 January 2006", "2 Jan 2006",
		"2006-01-02 15:04:05 MST", "2006-01-02 15:04:05 -0700",
	}

	// First try the string as-is (so comma formats match directly),
	// then the comma-stripped version, mirroring ParseDate's normalization.
	candidates := []string{trimmed, strings.ReplaceAll(trimmed, ",", "")}
	for _, candidate := range candidates {
		for _, format := range formats {
			if _, err := time.Parse(format, candidate); err == nil {
				return t.Format(format)
			}
		}
	}

	// Couldn't detect the format; fall back to ISO 8601
	return t.Format("2006-01-02")
}

// NearestIndex returns the index into dates of the record a displayer
// would show for query, or -1 if none lies within ±toleranceYears
// (calendar years, inclusive). dates must be sorted ascending.
// Ties prefer the later date.
func NearestIndex(dates []time.Time, query time.Time, toleranceYears int) int {
	i := sort.Search(len(dates), func(j int) bool {
		return !dates[j].Before(query)
	})

	best := -1
	var bestY int
	var bestRem time.Duration

	// Right neighbor first; left must be *strictly* closer to win,
	// so equal calendar distance keeps the later date.
	if i < len(dates) {
		if y, rem, ok := calendarDist(query, dates[i], toleranceYears); ok {
			best, bestY, bestRem = i, y, rem
		}
	}
	if i > 0 {
		if y, rem, ok := calendarDist(dates[i-1], query, toleranceYears); ok {
			if best < 0 || y < bestY || (y == bestY && rem < bestRem) {
				best = i - 1
			}
		}
	}
	return best
}

// calendarDist measures the distance from a to b (a <= b) as whole
// calendar years plus a sub-year remainder, and reports whether it is
// within toleranceYears (inclusive). (6, 0) means exactly 6 years,
// regardless of how many leap days the span contains.
func calendarDist(a, b time.Time, toleranceYears int) (years int, rem time.Duration, ok bool) {
	years = b.Year() - a.Year()
	if a.AddDate(years, 0, 0).After(b) {
		years--
	}
	rem = b.Sub(a.AddDate(years, 0, 0))
	ok = years < toleranceYears || (years == toleranceYears && rem == 0)
	return years, rem, ok
}

func AttachQueryParamToUrl(url, queryStr string) string {
	if strings.Contains(url, "?") {
		url = fmt.Sprintf("%s&%s", url, queryStr)
	} else {
		url = fmt.Sprintf("%s?%s", url, queryStr)
	}

	return url
}

func CleanupTaskURLQueryParams(url string) string {
	if !strings.Contains(url, "?") {
		return url
	}

	urlPartsInitial := strings.Split(url, "?")
	urlParts := make([]string, 0)

	for _, part := range urlPartsInitial {
		if part != "" {
			urlParts = append(urlParts, part)
		}
	}

	url = urlParts[0]

	if len(urlParts) > 1 {
		// We have query params
		paramsInitial := strings.Split(urlParts[1], "&")
		params := make([]string, 0)
		for _, param := range paramsInitial {
			if param != "" {
				params = append(params, param)
			}
		}

		if len(params) > 0 {
			url = fmt.Sprintf("%s?", url)
			addedParams := false
			for _, param := range params {
				keyVal := strings.Split(param, "=")
				key := strings.ToLower(keyVal[0])
				// Remove region, tab and time as they're being handled via the tool
				if key == "tab" || key == "region" || key == "time" || key == "country" || key == "mapselect" {
					continue
				}

				// Avoid globe parameters
				if strings.HasPrefix(key, "globe") {
					continue
				}

				if !strings.HasSuffix(url, "?") {
					url = fmt.Sprintf("%s&", url)
				}
				url = fmt.Sprintf("%s%s=%s", url, keyVal[0], keyVal[1])
				addedParams = true
			}

			if !addedParams {
				url = strings.Split(url, "?")[0]
			}
		}
	}

	return url
}

func WaitElementWithTimeout(page *rod.Page, selector string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Println("timeout waiting for element after ", selector, timeout)
			return fmt.Errorf("timeout waiting for element %q after %s", selector, timeout)
		case <-ticker.C:
			has, _, err := page.Has(selector)
			if err != nil {
				fmt.Println("error checking for element : ", selector, err)
				return fmt.Errorf("error checking for element %s: %w", selector, err)
			}
			if has {
				return nil
			}
		}
	}
}

func SplitSlice[T any](slice []T, numChunks int) [][]T {
	if numChunks <= 0 {
		return nil
	}

	length := len(slice)
	chunkSize := (length + numChunks - 1) / numChunks
	result := make([][]T, 0, numChunks)

	for i := 0; i < length; i += chunkSize {
		end := i + chunkSize
		if end > length {
			end = length
		}
		result = append(result, slice[i:end])
	}

	return result
}

func DiffSliceBy[A any, K comparable](a []A, b []A, keyFn func(A) K) []A {
	bKeys := make(map[K]struct{}, len(b))

	for _, item := range b {
		bKeys[keyFn(item)] = struct{}{}
	}

	result := make([]A, 0)

	for _, item := range a {
		key := keyFn(item)

		if _, exists := bKeys[key]; !exists {
			result = append(result, item)
		}
	}

	return result
}
