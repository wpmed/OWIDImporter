package sessions

import (
	"sync"

	"github.com/gorilla/websocket"
)

type Session struct {
	Username                string
	ResourceOwnerKeyTemp    string
	ResourceOwnerSecretTemp string
	ResourceOwnerKey        string
	ResourceOwnerSecret     string
	OauthVerifier           string
	Ws                      *websocket.Conn
	WsMutex                 *sync.Mutex
}

type SubscriptionSession struct {
	Ws      *websocket.Conn
	WsMutex *sync.Mutex
	Topic   string
	Id      string
}

var (
	Sessions             = make(map[string]*Session)
	SubscriptionSessions = make(map[string][]*SubscriptionSession)
)

var assignSubscriptionSessionMutex = sync.Mutex{}

func AddSubscriptionSession(subscriptionId string, session *SubscriptionSession) {
	assignSubscriptionSessionMutex.Lock()

	SubscriptionSessions[subscriptionId] = append(SubscriptionSessions[subscriptionId], session)

	assignSubscriptionSessionMutex.Unlock()
}

func RemoveSubscriptionSession(subscriptionId, id string) {
	assignSubscriptionSessionMutex.Lock()
	index := -1

	for i := 0; i < len(SubscriptionSessions[subscriptionId]); i++ {
		if SubscriptionSessions[subscriptionId][i].Id == id {
			index = i
			break
		}
	}

	if index != -1 {
		SubscriptionSessions[subscriptionId] = append(SubscriptionSessions[subscriptionId][:index], SubscriptionSessions[subscriptionId][index+1:]...)
	}

	assignSubscriptionSessionMutex.Unlock()
}

func RemoveFullSubscription(subscriptionId string) {
	assignSubscriptionSessionMutex.Lock()
	delete(SubscriptionSessions, subscriptionId)

	assignSubscriptionSessionMutex.Unlock()
}

func RemoveUserSession(username string) {
	for id, session := range Sessions {
		if session.Username == username {
			delete(Sessions, id)
		}
	}
}

const (
	SessionCookieName = "owidsession"
)
