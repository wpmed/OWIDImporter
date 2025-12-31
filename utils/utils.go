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
	"strings"

	"github.com/dghubble/oauth1"
	"github.com/wpmed-videowiki/OWIDImporter/env"
	"github.com/wpmed-videowiki/OWIDImporter/models"
	"github.com/wpmed-videowiki/OWIDImporter/sessions"
)

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

		fmt.Println("SENDING POST REQUEST")
		res, err = client.Post(url, "multipart/form-data; boundary="+writer.Boundary(), &b)
	} else {
		fmt.Println("SENDING GET REQUEST")
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
	fmt.Println("Res body: ", res.StatusCode, string(body))
	var result T
	err = json.Unmarshal(body, &result)
	if err != nil {
		fmt.Println("Error unmarshalling", err)
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
	fmt.Println("User info", result.Query.UserInfo)

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
	return nil
}

func sendWSTaskMessage(taskId string, messageType string, msg string) {
	go func() {
		if len(sessions.TaskSessions[taskId]) > 0 {
			failedSessions := make([]string, 0)
			for _, s := range sessions.TaskSessions[taskId] {
				s.WsMutex.Lock()
				fmt.Println("Sending msg ", messageType)
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
					sessions.RemoveTaskSession(taskId, id)
				}
			}
		}
	}()
}

func SendWSMessage(session *sessions.Session, messageType string, message string) error {
	go func() {
		session.WsMutex.Lock()
		defer session.WsMutex.Unlock()
		fmt.Println("Sending msg", messageType, "-", message)
		err := session.Ws.WriteJSON(map[string]string{
			"type": messageType,
			"msg":  message,
		})
		if err != nil {
			fmt.Println("Error sending msg", messageType, "-", message, ": ", err)
		}
	}()

	return nil
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
	return strings.ToUpper(s[:1]) + s[1:]
}
