package main

import (
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

type pixel struct {
	X     int    `json:"x"`
	Y     int    `json:"y"`
	Color string `json:"color"`
}

var grid [16][16]string

func init() {
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			grid[x][y] = "black"
		}
	}
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

var clients = make(map[*websocket.Conn]bool)
var lock = sync.RWMutex{}

var validColors = map[string]bool{
	"black":         true,
	"white":         true,
	"red":           true,
	"green":         true,
	"blue":          true,
	"orange":        true,
	"yellow":        true,
	"purple":        true,
	"mediumpurple":  true,
	"fuchsia":       true,
	"rebeccapurple": true,
	"teal":          true,
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	lock.Lock()
	clients[conn] = true
	lock.Unlock()

	sendGridState(conn)

	for {
		p := &pixel{}
		err := conn.ReadJSON(p)
		if err != nil {
			lock.Lock()
			delete(clients, conn)
			lock.Unlock()
			log.Println(err)
			return
		}

		if _, ok := validColors[p.Color]; !ok {
			log.Printf("Invalid color received: %s", p.Color)
			continue
		}

		grid[p.X][p.Y] = p.Color
		log.Printf("User %s placed color %s at (%d, %d)\n", conn.RemoteAddr(), p.Color, p.X, p.Y)
		broadcastPixel(p)
	}
}

func sendGridState(conn *websocket.Conn) {
	lock.RLock()
	defer lock.RUnlock()
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			p := &pixel{X: x, Y: y, Color: grid[x][y]}
			err := conn.WriteJSON(p)
			if err != nil {
				log.Println(err)
			}
		}
	}
}

func broadcastPixel(p *pixel) {
	lock.RLock()
	defer lock.RUnlock()
	for conn := range clients {
		err := conn.WriteJSON(p)
		if err != nil {
			log.Println(err)
		}
	}
}

func main() {
	http.HandleFunc("/ws", wsHandler)
	http.Handle("/", http.FileServer(http.Dir("./public")))
	log.Fatal(http.ListenAndServe(":9090", nil))
}
