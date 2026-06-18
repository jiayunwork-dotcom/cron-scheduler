package ws

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func ServeWS(hub *Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		upgrader.CheckOrigin = func(r *http.Request) bool {
			return true
		}

		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}

		client := &Client{
			hub:  hub,
			conn: conn,
			send: make(chan []byte, 256),
			tasks: make(map[string]bool),
		}

		hub.register <- client

		go client.writePump()
		client.readPump()
	}
}
