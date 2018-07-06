package wsrelay

import (
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 8) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 2048
)

// connection is an middleman between the websocket connection and the broker.
type connection struct {
	// The websocket connection.
	*websocket.Conn

	// Buffered channel of outbound messages.
	send chan []byte
}

// relayReads relays messages from the websocket connection to the broker.
func (s *subscription) relayReads(b *Broker) {
	conn := s.conn

	// when the read loop closes
	defer func() {
		// unregister
		b.unregister <- *s

		// close the socket
		conn.Close()
	}()

	// set some settings for the websocket
	conn.SetReadLimit(maxMessageSize)
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(
		func(string) error {
			return conn.SetReadDeadline(time.Now().Add(pongWait))
		})

	// read the messages
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			// if the client closed the connection
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				level.Info(b.logger).Log(
					"msg", "client closed connection",
					"err", err,
					"room", s.room,
				)
				break
			}
			// if there was an error that isn't the websocket closing
			level.Info(b.logger).Log(
				"msg", "unexpected error reading websocket",
				"err", err,
				"room", s.room,
			)
			break
		}
		// send the message
		b.broadcast <- message{msg, conn, s.room}
	}
}

// relayWrites relays messages from the broker to the websocket connection.
func (s *subscription) relayWrites(logger log.Logger) {
	conn := s.conn
	// create a ticker to send pings
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		conn.Close()
	}()
	for {
		select {
		// on receiving a message
		case message, ok := <-conn.send:
			if !ok {
				conn.write(websocket.CloseMessage, []byte{})
				return
			}
			if err := conn.write(websocket.TextMessage, message); err != nil {
				level.Info(logger).Log(
					"msg", "error writing message to websocket",
					"err", err,
				)
				return
			}

		// on tick
		case <-ticker.C:
			if err := conn.write(websocket.PingMessage, []byte{}); err != nil {
				level.Info(logger).Log(
					"msg", "error writing ping to websocket",
					"err", err,
				)
				return
			}
		}
	}
}

// write writes a message with the given message type and payload.
func (c *connection) write(mt int, payload []byte) error {
	// set a timeout
	c.SetWriteDeadline(time.Now().Add(writeWait))
	// write the message
	return c.WriteMessage(mt, payload)
}
