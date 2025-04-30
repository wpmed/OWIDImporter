package routes

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/wpmed-videowiki/OWIDImporter/env"
	"github.com/wpmed-videowiki/OWIDImporter/models"
	"github.com/wpmed-videowiki/OWIDImporter/sessions"
	"github.com/wpmed-videowiki/OWIDImporter/utils"
)

type ReplaceSessionData struct {
	SessionId string `json:"sessionId"`
}

func Login(c *gin.Context) {
	config := utils.GetOAuthConfig()

	requestToken, requestSecret, err := config.RequestToken()
	if err != nil {
		log.Println(err)
		c.String(http.StatusInternalServerError, "Failed to get request token")
		return
	}
	sessionId := uuid.New().String()
	sessions.Sessions[sessionId] = &sessions.Session{
		ResourceOwnerKeyTemp:    requestToken,
		ResourceOwnerSecretTemp: requestSecret,
	}

	authorizationURL, err := config.AuthorizationURL(requestToken)
	if err != nil {
		log.Println(err)
		c.String(http.StatusInternalServerError, "Failed to get authorization URL")
		return
	}
	c.SetCookie(sessions.SessionCookieName, sessionId, 60*60*24*7, "/", "*", env.GetEnv().OWID_DEBUG, true)

	c.Redirect(http.StatusTemporaryRedirect, authorizationURL.String())
}

func Callback(c *gin.Context) {
	sessionId, _ := c.Cookie(sessions.SessionCookieName)
	if sessionId == "" {
		c.HTML(http.StatusOK, "/", nil)
		return
	}
	session, ok := sessions.Sessions[sessionId]
	if !ok {
		c.HTML(http.StatusOK, "/", nil)
		return
	}

	oauthVerifier := c.Query("oauth_verifier")
	oauthToken := c.Query("oauth_token")

	oauth1Config := utils.GetOAuthConfig()

	accessToken, accessSecret, err := oauth1Config.AccessToken(oauthToken, session.ResourceOwnerSecretTemp, oauthVerifier)
	if err != nil {
		log.Println(err)
		c.String(http.StatusInternalServerError, "Failed to get access token")
		return
	}
	// Temp user model
	user := &models.User{
		ResourceOwnerSecret: accessSecret,
		ResourceOwnerKey:    accessToken,
	}

	username, err := utils.GetUsername(user)
	if err != nil {
		log.Println(err)
		c.String(http.StatusInternalServerError, "Failed to get username")
		return
	}
	user, err = models.FindUserByUsername(username)
	if err != nil || user == nil {
		_, err := models.NewUser(username, accessToken, accessSecret)
		if err != nil {
			c.String(http.StatusInternalServerError, "Failed to create user record")
			fmt.Println("Error creating user", err)
			return
		}
	}

	fmt.Println("User info", username)
	session.Username = username
	session.ResourceOwnerKey = accessToken
	session.ResourceOwnerSecret = accessSecret
	sessions.Sessions[sessionId] = session

	if env.GetEnv().OWID_ENV == "development" {
		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("http://localhost:5173/?sessionId=%s", sessionId))
		return
	}
	c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("/?sessionId=%s", sessionId))
}

func Logout(c *gin.Context) {
	// Redirect session
	c.SetCookie(sessions.SessionCookieName, "", -1, "/", "*", env.GetEnv().OWID_DEBUG, true)
	sessionId, _ := c.Cookie(sessions.SessionCookieName)
	if sessionId != "" {
		delete(sessions.Sessions, sessionId)
	}
	// Active SPA session
	sessionId = c.Request.Header.Get("sessionId")
	if sessionId != "" {
		delete(sessions.Sessions, sessionId)
	}

	if env.GetEnv().OWID_ENV == "development" {
		c.Redirect(http.StatusTemporaryRedirect, "http://localhost:5173/")
		return
	}
	c.Redirect(http.StatusTemporaryRedirect, "/")
}

func ReplaceSession(c *gin.Context) {
	var data ReplaceSessionData
	if err := c.BindJSON(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid data"})
		return
	}

	session, ok := sessions.Sessions[data.SessionId]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown session"})
		return
	}

	newSessionId := uuid.New().String()
	sessions.Sessions[newSessionId] = session

	delete(sessions.Sessions, data.SessionId)

	user, err := models.FindUserByUsername(session.Username)
	if err != nil {
		fmt.Println("Error getting user", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot get user info"})
		return
	}

	username, err := utils.GetUsername(user)
	if err != nil {
		log.Println(err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot get username"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"sessionId": newSessionId, "username": username})
}

func VerifySession(c *gin.Context) {
	var data ReplaceSessionData
	if err := c.BindJSON(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid data"})
		return
	}

	session, ok := sessions.Sessions[data.SessionId]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown session"})
		return
	}

	user, err := models.FindUserByUsername(session.Username)
	if err != nil {
		fmt.Println("Error getting user", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot get user info"})
		return
	}

	username, err := utils.GetUsername(user)
	if err != nil {
		log.Println(err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Session expired, please login again"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"username": username})
}
