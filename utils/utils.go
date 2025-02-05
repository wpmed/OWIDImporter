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

	"github.com/dghubble/oauth1"
	"github.com/wpmed-videowiki/OWIDImporter/env"
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

func GetOAuthClient(session *sessions.Session) *http.Client {
	return oauth1.NewClient(context.Background(), GetOAuthConfig(), &oauth1.Token{
		Token:       session.ResourceOwnerKey,
		TokenSecret: session.ResourceOwnerSecret,
	})
}

type UploadedFile struct {
	Filename string
	File     string
	Mime     string
}

func DoApiReq[T any](session *sessions.Session, params map[string]string, file *UploadedFile) (*T, error) {
	client := GetOAuthClient(session)
	values := make(url.Values)
	url := env.GetEnv().OWID_MW_API + "?"
	for k, v := range params {
		if k != "token" {
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
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"userinfo"`
	} `json:"query"`
}

func GetUsername(session *sessions.Session) (string, error) {
	result, err := DoApiReq[UserInfo](session, map[string]string{
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
