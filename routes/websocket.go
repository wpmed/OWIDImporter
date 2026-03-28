package routes

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/wpmed-videowiki/OWIDImporter/models"
	"github.com/wpmed-videowiki/OWIDImporter/sessions"
)

func Websocket(c *gin.Context) {
	queryParams := c.Request.URL.Query()

	sessionId := queryParams.Get("sessionId")
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
		log.Println(err)
	}
	session.Ws = ws
	session.WsMutex = &sync.Mutex{}

	go func() {
		for {
			fmt.Println("Reading message")
			mt, message, err := ws.ReadMessage()
			fmt.Println("MT: ", mt)
			if err != nil {
				log.Println("Error reading message", err)
				if websocket.IsCloseError(err, websocket.CloseGoingAway) {
					sessions.RemoveFullSubscription(sessionId)
				}
				break
			}
			if mt == websocket.CloseMessage {
				log.Println("Received close message", mt)
				sessions.RemoveFullSubscription(sessionId)
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
				case "subscribe_task":
					fmt.Println("Action message", actionMessage.Action, ": ", actionMessage)
					taskSession := sessions.SubscriptionSession{
						Id:      sessionId,
						Ws:      ws,
						WsMutex: &sync.Mutex{},
					}
					sessions.AddSubscriptionSession(actionMessage.Content, &taskSession)
				case "unsubscribe_task":
					fmt.Println("Action message", actionMessage.Action, ": ", actionMessage)
					sessions.RemoveSubscriptionSession(actionMessage.Content, sessionId)
				case "subscribe_task_list":
					fmt.Println("Action message", actionMessage.Action, ": ", actionMessage)
					user, err := models.FindUserByUsername(session.Username)
					if err != nil || user == nil {
						continue
					}
					taskSession := sessions.SubscriptionSession{
						Id:      sessionId,
						Ws:      ws,
						WsMutex: &sync.Mutex{},
					}
					sessions.AddSubscriptionSession(fmt.Sprintf("%s_task_list", user.ID), &taskSession)
				case "unsubscribe_task_list":
					fmt.Println("Action message", actionMessage.Action, ": ", actionMessage)
					user, err := models.FindUserByUsername(session.Username)
					if err != nil || user == nil {
						continue
					}
					sessions.RemoveSubscriptionSession(fmt.Sprintf("%s_task_list", user.ID), sessionId)
				}
				break
			}
		}
	}()
}
