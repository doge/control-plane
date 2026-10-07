package node

import (
	"context"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

func (a *Agent) dispatch(conn *websocket.Conn, message Message) {
	lockValue, _ := a.serverLocks.LoadOrStore(message.ServerID, &sync.Mutex{})
	lock := lockValue.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	if message.Type == "create_server" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
		defer cancel()
		id, err := a.createServer(ctx, message)
		response := Message{Type: "server_created", RequestID: message.RequestID, ServerID: message.ServerID, ContainerID: id}
		if err != nil {
			response.Type = "request_error"
			response.Payload = map[string]string{"error": err.Error()}
		}
		_ = a.send(conn, response)
		return
	}
	result, err := a.handleRequest(message)
	response := Message{Type: "request_result", RequestID: message.RequestID, ServerID: message.ServerID, Payload: result}
	if err != nil {
		response.Type = "request_error"
		response.Payload = map[string]string{"error": err.Error()}
	}
	_ = a.send(conn, response)
}
