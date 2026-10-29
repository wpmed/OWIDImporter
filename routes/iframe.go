package routes

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/wpmed-videowiki/OWIDImporter/models"
	"github.com/wpmed-videowiki/OWIDImporter/services"
	"github.com/wpmed-videowiki/OWIDImporter/sessions"
	"github.com/wpmed-videowiki/OWIDImporter/utils"
)

type IframeInfoData struct {
	Url string `json:"url"`
}

// GetIframeInfo is a read-only lookup of an OWID grapher/explorer page for the iframe import.
func GetIframeInfo(c *gin.Context) {
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

	var data IframeInfoData
	if err := c.BindJSON(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid data"})
		return
	}

	cannotImport := gin.H{"error": utils.ErrIframeChartCannotBeImported.Error()}
	if _, err := utils.NormalizeIframeChartPath(data.Url); err != nil {
		c.JSON(http.StatusBadRequest, cannotImport)
		return
	}

	l, browser := services.GetBrowser()
	defer l.Cleanup()
	defer browser.Close()
	info, err := services.GetIframeInfo(browser, data.Url)
	if err != nil {
		fmt.Println("Error getting iframe info", data.Url, err)
		c.JSON(http.StatusBadRequest, cannotImport)
		return
	}

	c.JSON(http.StatusOK, gin.H{"info": info})
}
