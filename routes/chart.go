package routes

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/wpmed-videowiki/OWIDImporter/models"
	"github.com/wpmed-videowiki/OWIDImporter/services"
	"github.com/wpmed-videowiki/OWIDImporter/sessions"
)

type ChartParametersData struct {
	Url string `json:"url"`
}

type MultiChartParametersData struct {
	Urls []string `json:"urls"`
}

type MultiChartParametersResponse struct {
	Url    string                    `json:"url"`
	Info   services.ChartInfo        `json:"info"`
	Params []services.ChartParameter `json:"params"`
}

func GetChartParameters(c *gin.Context) {
	sessionId := c.Request.Header.Get("sessionId")

	if sessionId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid session"})
		return
	}

	session, ok := sessions.Sessions[sessionId]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown session"})
		return
	}
	user, err := models.FindUserByUsername(session.Username)
	if err != nil || user == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown user"})
		return
	}

	var data ChartParametersData
	if err := c.BindJSON(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid data"})
		return
	}

	url := data.Url
	if url == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid url param"})
		return
	}

	fmt.Println("================================== Incoming url: ", url)
	if !strings.Contains(url, "tab=") {
		if strings.Contains(url, "&") {
			url = fmt.Sprintf("%s%s", url, "&tab=map")
		} else if !strings.Contains(url, "?") {
			url = fmt.Sprintf("%s%s", url, "?tab=map")
		}
	}
	fmt.Println("Final url: ", url)

	l, browser := services.GetBrowser()
	defer l.Cleanup()
	defer browser.Close()
	info, err := services.GetChartInfo(browser, url, "")

	c.JSON(http.StatusOK, gin.H{"params": info.Params, "info": info})
}

func GetMultiChartParameters(c *gin.Context) {
	sessionId := c.Request.Header.Get("sessionId")

	if sessionId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid session"})
		return
	}

	session, ok := sessions.Sessions[sessionId]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown session"})
		return
	}
	user, err := models.FindUserByUsername(session.Username)
	if err != nil || user == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown user"})
		return
	}

	var data MultiChartParametersData
	if err := c.BindJSON(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid data"})
		return
	}

	response := make([]MultiChartParametersResponse, 0)
	l, browser := services.GetBrowser()
	defer l.Cleanup()
	defer browser.Close()

	for _, url := range data.Urls {
		fmt.Println("================================== Incoming url: ", url)
		if !strings.Contains(url, "tab=") {
			if strings.Contains(url, "&") {
				url = fmt.Sprintf("%s%s", url, "&tab=map")
			} else if !strings.Contains(url, "?") {
				url = fmt.Sprintf("%s%s", url, "?tab=map")
			}
		}
		fmt.Println("Final url: ", url)

		info, err := services.GetChartInfo(browser, url, "")
		if err == nil {
			response = append(response, MultiChartParametersResponse{Url: url, Params: *info.Params, Info: *info})
		}
	}

	c.JSON(http.StatusOK, gin.H{"data": response})
}
