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

	// The shared lookup browser stays warm between requests (see services.LookupIframeInfo)
	info, err := services.LookupIframeInfo(c.Request.Context(), data.Url)
	if err != nil {
		if c.Request.Context().Err() != nil {
			// The client went away; rod's panic text in err would only flood the log
			fmt.Println("Iframe info lookup cancelled by the client", data.Url)
			c.JSON(http.StatusBadRequest, cannotImport)
			return
		}
		fmt.Println("Error getting iframe info", data.Url, err)
		c.JSON(http.StatusBadRequest, cannotImport)
		return
	}

	c.JSON(http.StatusOK, gin.H{"info": info})
}
