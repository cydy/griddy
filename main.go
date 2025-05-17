package main

import (
	"embed"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

//go:embed index.html 98.css
var staticFiles embed.FS

type pixel struct {
	X     int    `json:"x"`
	Y     int    `json:"y"`
	Color string `json:"color"`
}

const grid_size_x = 100
const grid_size_y = 100

var grid [grid_size_x][grid_size_y]string

func init() {
	for y := 0; y < grid_size_y; y++ {
		for x := 0; x < grid_size_x; x++ {
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
	"tan":           true,
}

type gridSize struct {
	X int `json:"x"`
	Y int `json:"y"`
}

func sendGridSize(conn *websocket.Conn) {
	size := &gridSize{X: grid_size_x, Y: grid_size_y}
	err := conn.WriteJSON(size)
	if err != nil {
		log.Println(err)
	}
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

	sendGridSize(conn)
	sendGridState(conn)
	broadcastClientCount()

	for {
		p := &pixel{}
		err := conn.ReadJSON(p)
		if err != nil {
			lock.Lock()
			delete(clients, conn)
			lock.Unlock()
			log.Println(err)
			broadcastClientCount()
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
	for y := 0; y < grid_size_y; y++ {
		for x := 0; x < grid_size_x; x++ {
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

func broadcastClientCount() {
	lock.RLock()
	count := len(clients)
	lock.RUnlock()

	lock.RLock()
	defer lock.RUnlock()
	for conn := range clients {
		err := conn.WriteJSON(map[string]int{"type": 1, "count": count})
		if err != nil {
			log.Println(err)
		}
	}
}

func main() {
	http.HandleFunc("/ws", wsHandler)

	http.Handle("/", http.FileServer(http.FS(staticFiles)))

	log.Fatal(http.ListenAndServe("0.0.0.0:9090", nil))
}
