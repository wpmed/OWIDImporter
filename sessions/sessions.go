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

// AddSubscriptionSession registers session for subscriptionId. A connection that
// is already subscribed to subscriptionId is not added twice.
func AddSubscriptionSession(subscriptionId string, session *SubscriptionSession) {
	assignSubscriptionSessionMutex.Lock()
	defer assignSubscriptionSessionMutex.Unlock()

	for _, s := range SubscriptionSessions[subscriptionId] {
		if session.Ws != nil && s.Ws == session.Ws {
			return
		}
	}
	SubscriptionSessions[subscriptionId] = append(SubscriptionSessions[subscriptionId], session)
}

// GetSubscriptionSessions returns a snapshot of the sessions subscribed to
// subscriptionId, safe to iterate while subscriptions change concurrently.
func GetSubscriptionSessions(subscriptionId string) []*SubscriptionSession {
	assignSubscriptionSessionMutex.Lock()
	defer assignSubscriptionSessionMutex.Unlock()

	subs := SubscriptionSessions[subscriptionId]
	out := make([]*SubscriptionSession, len(subs))
	copy(out, subs)
	return out
}

// RemoveSubscriptionConn removes the subscription of connection ws from
// subscriptionId (other connections of the same session are kept).
func RemoveSubscriptionConn(subscriptionId string, ws *websocket.Conn) {
	assignSubscriptionSessionMutex.Lock()
	defer assignSubscriptionSessionMutex.Unlock()

	removeConnLocked(subscriptionId, ws)
}

// RemoveConnSubscriptions removes every subscription held by connection ws.
func RemoveConnSubscriptions(ws *websocket.Conn) {
	assignSubscriptionSessionMutex.Lock()
	defer assignSubscriptionSessionMutex.Unlock()

	for subscriptionId := range SubscriptionSessions {
		removeConnLocked(subscriptionId, ws)
	}
}

func removeConnLocked(subscriptionId string, ws *websocket.Conn) {
	subs := SubscriptionSessions[subscriptionId]
	kept := make([]*SubscriptionSession, 0, len(subs))
	for _, s := range subs {
		if s.Ws != ws {
			kept = append(kept, s)
		}
	}
	if len(kept) == 0 {
		delete(SubscriptionSessions, subscriptionId)
	} else {
		SubscriptionSessions[subscriptionId] = kept
	}
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
