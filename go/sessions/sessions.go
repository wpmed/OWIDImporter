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

var Sessions = make(map[string]*Session)

const (
	SessionCookieName = "owidsession"
)
