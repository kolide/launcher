package wsrelay

import (
	"log"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

type message struct {
	// the message data
	data []byte

	// connection that sent the message
	sender *connection

	// room the message was for
	room string
}

type subscription struct {
	// subscription's ws connection
	conn *connection

	// room subscription belongs to
	room string
}

// Broker maintains the set of active connections and broadcasts messages to the
// connections.
type Broker struct {
	// Registered connections.
	rooms map[string]map[*connection]bool

	// Inbound messages from the connections.
	broadcast chan message

	// Register requests from the connections.
	register chan subscription

	// Unregister requests from connections.
	unregister chan subscription

	// Upgrader for upgrading http requests to websockets
	upgrader *websocket.Upgrader
}

// NewBroker returns a new Broker
func NewBroker() *Broker {
	return &Broker{
		broadcast:  make(chan message),
		register:   make(chan subscription),
		unregister: make(chan subscription),
		rooms:      make(map[string]map[*connection]bool),
		upgrader: &websocket.Upgrader{
			ReadBufferSize:  2048,
			WriteBufferSize: 2048,
		},
	}
}

// Start starts the Broker
func (b *Broker) Start() {
	for {
		select {
		case sub := <-b.register:
			// get the connections for a room
			connections := b.rooms[sub.room]

			// create a set for connections if it doesn't exist
			if connections == nil {
				log.Printf("Creating room %s", sub.room)
				b.rooms[sub.room] = make(map[*connection]bool)
			}

			// put connection in the set of connections
			b.rooms[sub.room][sub.conn] = true

			log.Printf("Client joined room %s", sub.room)

		case sub := <-b.unregister:
			// get the connections for a room
			connections := b.rooms[sub.room]

			// skip if there are no connections
			if connections == nil {
				continue
			}

			// if the connection is registered
			if _, ok := connections[sub.conn]; ok {

				// delete the connection
				delete(connections, sub.conn)

				// close the connection send channel
				close(sub.conn.send)
			}

			log.Printf("Client left room %s", sub.room)

			// remove room if empty
			if len(connections) == 0 {
				delete(b.rooms, sub.room)

				log.Printf("Closed room %s", sub.room)
			}

		case msg := <-b.broadcast:
			// get the connections for a room
			connections := b.rooms[msg.room]

			// pick a connection
			for conn := range connections {

				// don't send to self
				if conn == msg.sender {
					continue
				}

				select {

				// try to send
				case conn.send <- msg.data:

				// if broker can't send
				default:
					// close the connections send channel
					close(conn.send)

					// delete connection from set
					delete(connections, conn)

					// remove the room if empty
					if len(connections) == 0 {
						delete(b.rooms, msg.room)
					}
				}
			}
		}
	}
}

// Handler handles websocket requests from the peer.
func (b *Broker) Handler(w http.ResponseWriter, r *http.Request) {
	// upgrade connection to a websocket
	ws, err := b.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("error upgrading to websocket: %v", err)
		return
	}

	// the url parameters
	vars := mux.Vars(r)

	// create a connection with the websocket
	conn := &connection{Conn: ws, send: make(chan []byte, maxMessageSize)}

	// create the subscription for the room
	sub := subscription{conn: conn, room: vars["room"]}

	// register the subscription with the broker
	b.register <- sub

	// create a goroutine to handle writes
	go sub.relayWrites()

	// handle the reads
	sub.relayReads(b)
}

// SetRoom sets the room variable on the request, so that the handler can access it
// TODO: remove or set the room in the context directly... needs more exploration
func (b *Broker) SetRoom(room string, r *http.Request) *http.Request {
	req := mux.SetURLVars(r, map[string]string{"room": room})
	return req
}
