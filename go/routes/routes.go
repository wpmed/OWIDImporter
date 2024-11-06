package routes

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/wpmed-videowiki/OWIDImporter-go/env"
	"github.com/wpmed-videowiki/OWIDImporter-go/services"
	"github.com/wpmed-videowiki/OWIDImporter-go/sessions"
	"github.com/wpmed-videowiki/OWIDImporter-go/utils"
)

type WebsocketActionMessage struct {
	Action      string `json:"action"`
	Url         string `json:"url"`
	FileName    string `json:"fileName"`
	Description string `json:"desc"`
}

var upgrader = websocket.Upgrader{} // use default options

func BuildRoutes() *gin.Engine {
	router := gin.Default()
	router.LoadHTMLGlob("routes/templates/*")

	router.GET("/", Home)
	router.GET("/login", Login)
	router.GET("/callback", Callback)
	router.GET("/logout", Logout)
	router.GET("/ws", Websocket)

	return router
}

func Home(c *gin.Context) {
	// get OWID_ID from cookies
	sessionId, _ := c.Cookie(sessions.SessionCookieName)
	if sessionId == "" {
		c.HTML(http.StatusOK, "login.html", nil)
		return
	}
	session, ok := sessions.Sessions[sessionId]
	if !ok {
		c.HTML(http.StatusOK, "login.html", nil)
		return
	}

	username, err := utils.GetUsername(session)
	if err != nil {
		log.Fatal(err)
		c.String(http.StatusInternalServerError, "Failed to get username")
		return
	}
	c.HTML(http.StatusOK, "home.html", gin.H{
		"Username": username,
	})
}

func Login(c *gin.Context) {
	config := utils.GetOAuthConfig()

	requestToken, requestSecret, err := config.RequestToken()
	if err != nil {
		log.Fatal(err)
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
		log.Fatal(err)
		c.String(http.StatusInternalServerError, "Failed to get authorization URL")
		return
	}
	c.SetCookie(sessions.SessionCookieName, sessionId, 60*60*24*7, "/", "*", env.GetEnv().OWID_DEBUG, true)

	c.Redirect(http.StatusTemporaryRedirect, authorizationURL.String())
}

func Callback(c *gin.Context) {
	sessionId, _ := c.Cookie(sessions.SessionCookieName)
	if sessionId == "" {
		c.HTML(http.StatusOK, "login.html", nil)
		return
	}
	session, ok := sessions.Sessions[sessionId]
	if !ok {
		c.HTML(http.StatusOK, "login.html", nil)
		return
	}

	oauthVerifier := c.Query("oauth_verifier")
	oauthToken := c.Query("oauth_token")

	oauth1Config := utils.GetOAuthConfig()

	accessToken, accessSecret, err := oauth1Config.AccessToken(oauthToken, session.ResourceOwnerSecretTemp, oauthVerifier)
	fmt.Println("accessToken", accessToken)
	fmt.Println("accessSecret", accessSecret)
	if err != nil {
		log.Fatal(err)
		c.String(http.StatusInternalServerError, "Failed to get access token")
		return
	}
	session.ResourceOwnerKey = accessToken
	session.ResourceOwnerSecret = accessSecret
	sessions.Sessions[sessionId] = session
	c.Redirect(http.StatusTemporaryRedirect, "/")
}

func Logout(c *gin.Context) {
	c.SetCookie(sessions.SessionCookieName, "", -1, "/", "*", env.GetEnv().OWID_DEBUG, true)
	sessionId, _ := c.Cookie(sessions.SessionCookieName)
	if sessionId != "" {
		delete(sessions.Sessions, sessionId)
	}
	c.Redirect(http.StatusTemporaryRedirect, "/")
}

func Websocket(c *gin.Context) {
	sessionId, _ := c.Cookie(sessions.SessionCookieName)
	if sessionId == "" {
		return
	}
	session, ok := sessions.Sessions[sessionId]
	if !ok {
		return
	}
	upgrader.CheckOrigin = func(r *http.Request) bool { return true }
	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Fatal(err)
	}
	session.Ws = ws
	session.WsMutex = &sync.Mutex{}

	go func() {
		for {
			fmt.Println("Reading message")
			mt, message, err := ws.ReadMessage()
			if err != nil {
				log.Println("Error reading message", err)
				break
			}
			if mt == websocket.TextMessage {
				var actionMessage WebsocketActionMessage
				err := json.Unmarshal(message, &actionMessage)
				if err != nil {
					log.Println("Error unmarshalling message", err)
					continue
				}
				switch actionMessage.Action {
				case "start":
					fmt.Println("Action message", actionMessage.Action, ": ", actionMessage)
					fmt.Println("Action message", actionMessage.Action, ": ", actionMessage)
					err := services.StartMap(session, services.StartData{
						Url:         actionMessage.Url,
						FileName:    actionMessage.FileName,
						Description: actionMessage.Description,
					})
					if err != nil {
						log.Println("Error starting map", err)
					}
					ws.Close()
				case "startMap":
					fmt.Println("Action message", actionMessage.Action, ": ", actionMessage)
					err := services.StartChart(session, services.StartData{
						Url:         actionMessage.Url,
						FileName:    actionMessage.FileName,
						Description: actionMessage.Description,
					})
					if err != nil {
						log.Println("Error starting map", err)
					}
					ws.Close()
				}
			}
		}
	}()
}
