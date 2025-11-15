package routes

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/wpmed-videowiki/OWIDImporter/models"
	"github.com/wpmed-videowiki/OWIDImporter/services"
	"github.com/wpmed-videowiki/OWIDImporter/sessions"
)

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

	url := c.Query("url")
	if url == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid url param"})
		return
	}

	l, browser := services.GetBrowser()
	defer l.Cleanup()
	defer browser.Close()
	params := services.GetChartParameters(browser, url)

	c.JSON(http.StatusOK, gin.H{"params": params})
}
