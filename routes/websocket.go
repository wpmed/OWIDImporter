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

	// A connection without a known session is anonymous: it may only follow
	// single tasks (public import pages), never lists or operations.
	sessionId := queryParams.Get("sessionId")
	session, ok := sessions.Sessions[sessionId]
	if sessionId == "" || !ok || session == nil {
		session = nil
		sessionId = ""
	}
	upgrader.CheckOrigin = func(r *http.Request) bool { return true }
	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println(err)
		return
	}
	// One write mutex per connection, shared by every subscription on it:
	// gorilla/websocket allows only one concurrent writer per connection.
	wsMutex := &sync.Mutex{}
	if session != nil {
		session.Ws = ws
		session.WsMutex = wsMutex
	}
	newSubscription := func() *sessions.SubscriptionSession {
		return &sessions.SubscriptionSession{
			Id:      sessionId,
			Ws:      ws,
			WsMutex: wsMutex,
		}
	}

	go func() {
		// Any exit (close frame, read error) drops this connection's subscriptions.
		defer func() {
			sessions.RemoveConnSubscriptions(ws)
			ws.Close()
		}()
		for {
			fmt.Println("Reading message")
			mt, message, err := ws.ReadMessage()
			fmt.Println("MT: ", mt)
			if err != nil {
				log.Println("Error reading message", err)
				break
			}
			if mt == websocket.CloseMessage {
				log.Println("Received close message", mt)
				break
			}
			if mt == websocket.TextMessage {
				var actionMessage WebsocketActionMessage
				err := json.Unmarshal(message, &actionMessage)
				if err != nil {
					log.Println("Error unmarshalling message", err)
					continue
				}
				isTaskAction := actionMessage.Action == "subscribe_task" || actionMessage.Action == "unsubscribe_task"
				if session == nil && !isTaskAction {
					// List and operation subscriptions require a session
					continue
				}
				switch actionMessage.Action {
				case "subscribe_task":
					fmt.Println("Action message", actionMessage.Action, ": ", actionMessage)
					// Only ids of existing tasks (tasks are public). Topics share one
					// namespace, so this also keeps anyone off another user's
					// "<userId>_task_list" / operation topics.
					task, err := models.FindTaskById(actionMessage.Content)
					if err != nil || task == nil {
						continue
					}
					sessions.AddSubscriptionSession(task.ID, newSubscription())
				case "unsubscribe_task":
					fmt.Println("Action message", actionMessage.Action, ": ", actionMessage)
					sessions.RemoveSubscriptionConn(actionMessage.Content, ws)
				case "subscribe_task_list":
					fmt.Println("Action message", actionMessage.Action, ": ", actionMessage)
					user, err := models.FindUserByUsername(session.Username)
					if err != nil || user == nil {
						continue
					}
					sessions.AddSubscriptionSession(fmt.Sprintf("%s_task_list", user.ID), newSubscription())
				case "unsubscribe_task_list":
					fmt.Println("Action message", actionMessage.Action, ": ", actionMessage)
					user, err := models.FindUserByUsername(session.Username)
					if err != nil || user == nil {
						continue
					}
					sessions.RemoveSubscriptionConn(fmt.Sprintf("%s_task_list", user.ID), ws)
				case "subscribe_operation":
					fmt.Println("Action message", actionMessage.Action, ": ", actionMessage)
					// Operations are private: only the session user's own
					user, err := models.FindUserByUsername(session.Username)
					if err != nil || user == nil {
						continue
					}
					operation, err := models.FindOperationById(actionMessage.Content)
					if err != nil || operation == nil || operation.UserId != user.ID {
						continue
					}
					sessions.AddSubscriptionSession(operation.ID, newSubscription())
				case "unsubscribe_operation":
					fmt.Println("Action message", actionMessage.Action, ": ", actionMessage)
					sessions.RemoveSubscriptionConn(actionMessage.Content, ws)
				case "subscribe_operation_list":
					fmt.Println("Action message", actionMessage.Action, ": ", actionMessage)
					user, err := models.FindUserByUsername(session.Username)
					if err != nil || user == nil {
						continue
					}
					sessions.AddSubscriptionSession(fmt.Sprintf("%s_operation_list", user.ID), newSubscription())
				case "unsubscribe_operation_list":
					fmt.Println("Action message", actionMessage.Action, ": ", actionMessage)
					user, err := models.FindUserByUsername(session.Username)
					if err != nil || user == nil {
						continue
					}
					sessions.RemoveSubscriptionConn(fmt.Sprintf("%s_operation_list", user.ID), ws)
				}
			}
		}
	}()
}
