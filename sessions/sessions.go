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

type TaskSession struct {
	Ws      *websocket.Conn
	WsMutex *sync.Mutex
	Id      string
}

var (
	Sessions     = make(map[string]*Session)
	TaskSessions = make(map[string][]*TaskSession)
)

var assignTaskSessionMutex = sync.Mutex{}

func AddTaskSession(taskId string, session *TaskSession) {
	assignTaskSessionMutex.Lock()

	TaskSessions[taskId] = append(TaskSessions[taskId], session)

	assignTaskSessionMutex.Unlock()
}

func RemoveTaskSession(taskId, id string) {
	assignTaskSessionMutex.Lock()
	index := -1

	for i := 0; i < len(TaskSessions[taskId]); i++ {
		if TaskSessions[taskId][i].Id == id {
			index = i
			break
		}
	}

	if index != -1 {
		TaskSessions[taskId] = append(TaskSessions[taskId][:index], TaskSessions[taskId][index+1:]...)
	}

	assignTaskSessionMutex.Unlock()
}

const (
	SessionCookieName = "owidsession"
)
