package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

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
	for y := range grid_size_y {
		for x := range grid_size_x {
			grid[x][y] = "black"
		}
	}
	loadLatestState()
}

func loadLatestState() {
	files, err := os.ReadDir("states")
	if err != nil {
		log.Printf("No states directory found or error reading it: %v", err)
		return
	}

	// Filter and sort state files
	var stateFiles []os.DirEntry
	for _, file := range files {
		if !file.IsDir() && strings.HasPrefix(file.Name(), "grid_state_") && strings.HasSuffix(file.Name(), ".json") {
			stateFiles = append(stateFiles, file)
		}
	}

	if len(stateFiles) == 0 {
		log.Println("No state files found")
		return
	}

	// Sort by name (which includes timestamp) in descending order
	sort.Slice(stateFiles, func(i, j int) bool {
		return stateFiles[i].Name() > stateFiles[j].Name()
	})

	// Load the most recent state file
	latestFile := stateFiles[0]
	data, err := os.ReadFile(filepath.Join("states", latestFile.Name()))
	if err != nil {
		log.Printf("Error reading latest state file: %v", err)
		return
	}

	var state gridState
	if err := json.Unmarshal(data, &state); err != nil {
		log.Printf("Error parsing state file: %v", err)
		return
	}

	// Clear current grid
	for y := range grid_size_y {
		for x := range grid_size_x {
			grid[x][y] = "black"
		}
	}

	// Import state
	for key, color := range state.Grid {
		var x, y int
		fmt.Sscanf(key, "%d,%d", &x, &y)
		if x >= 0 && x < grid_size_x && y >= 0 && y < grid_size_y {
			if validColors[color] {
				grid[x][y] = color
			}
		}
	}

	log.Printf("Successfully loaded state from %s", latestFile.Name())
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

var clients = make(map[*websocket.Conn]bool)
var lock = sync.RWMutex{}
var lastUpdateTimes = make(map[*websocket.Conn]int64)
var rateLimitLock = sync.RWMutex{}

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

// Secret colors that will trigger a flag
var secretColors = map[string]bool{
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

type gridState struct {
	Grid      map[string]string `json:"grid"`
	Timestamp string            `json:"timestamp"`
}

const adminPassword = "griddy_admin_yeehaw_cowboy000" // You should change this to a secure password

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
			rateLimitLock.Lock()
			delete(lastUpdateTimes, conn)
			rateLimitLock.Unlock()
			log.Println(err)
			broadcastClientCount()
			return
		}

		// Check rate limit
		rateLimitLock.Lock()
		lastUpdate := lastUpdateTimes[conn]
		now := time.Now().UnixNano()
		if now-lastUpdate < 100_000_000 { // 0.1 seconds in nanoseconds (10 updates per second)
			rateLimitLock.Unlock()
			continue // Skip this update if too soon
		}
		lastUpdateTimes[conn] = now
		rateLimitLock.Unlock()

		if _, ok := validColors[p.Color]; !ok {
			log.Printf("Invalid color received: %s", p.Color)
			continue
		}

		// Check if this is a secret color
		if secretColors[p.Color] {
			// Send the flag to the user who discovered it
			err := conn.WriteJSON(map[string]string{"type": "flag", "message": "flag{h1dden_c0lors_are_1337}"})
			if err != nil {
				log.Println(err)
			}
		}

		grid[p.X][p.Y] = p.Color
		log.Printf("User %s placed color %s at (%d, %d)\n", conn.RemoteAddr(), p.Color, p.X, p.Y)
		broadcastPixel(p)
	}
}

func sendGridState(conn *websocket.Conn) {
	lock.RLock()
	defer lock.RUnlock()
	for y := range grid_size_y {
		for x := range grid_size_x {
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

func importGridState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check password
	password := r.Header.Get("X-Admin-Password")
	if password != adminPassword {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var state gridState
	if err := json.NewDecoder(r.Body).Decode(&state); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Clear current grid
	for y := range grid_size_y {
		for x := range grid_size_x {
			grid[x][y] = "black"
		}
	}

	// Import new state
	for key, color := range state.Grid {
		var x, y int
		fmt.Sscanf(key, "%d,%d", &x, &y)
		if x >= 0 && x < grid_size_x && y >= 0 && y < grid_size_y {
			if validColors[color] {
				grid[x][y] = color
			}
		}
	}

	// Broadcast new state to all clients
	lock.RLock()
	for conn := range clients {
		sendGridState(conn)
	}
	lock.RUnlock()

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Grid state imported successfully"))
}

func saveGridState() {
	state := gridState{
		Grid:      make(map[string]string),
		Timestamp: time.Now().Format(time.RFC3339),
	}

	lock.RLock()
	for y := range grid_size_y {
		for x := range grid_size_x {
			if grid[x][y] != "black" { // Only save non-black pixels
				state.Grid[fmt.Sprintf("%d,%d", x, y)] = grid[x][y]
			}
		}
	}
	lock.RUnlock()

	jsonData, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		log.Printf("Error marshaling grid state: %v", err)
		return
	}

	// Ensure states directory exists
	if err := os.MkdirAll("states", 0755); err != nil {
		log.Printf("Error creating states directory: %v", err)
		return
	}

	// Create filename with timestamp
	filename := fmt.Sprintf("states/grid_state_%s.json", time.Now().Format("2006-01-02_15-04-05"))
	err = os.WriteFile(filename, jsonData, 0644)
	if err != nil {
		log.Printf("Error saving grid state: %v", err)
		return
	}
	log.Printf("Grid state saved successfully to %s", filename)

	// Clean up old state files
	files, err := os.ReadDir("states")
	if err != nil {
		log.Printf("Error reading directory: %v", err)
		return
	}

	// Filter and sort state files
	var stateFiles []os.DirEntry
	for _, file := range files {
		if !file.IsDir() && strings.HasPrefix(file.Name(), "grid_state_") && strings.HasSuffix(file.Name(), ".json") {
			stateFiles = append(stateFiles, file)
		}
	}

	// Sort by name (which includes timestamp) in descending order
	sort.Slice(stateFiles, func(i, j int) bool {
		return stateFiles[i].Name() > stateFiles[j].Name()
	})

	// Remove files beyond the last 20
	for i := 20; i < len(stateFiles); i++ {
		err := os.Remove(filepath.Join("states", stateFiles[i].Name()))
		if err != nil {
			log.Printf("Error removing old state file %s: %v", stateFiles[i].Name(), err)
		} else {
			log.Printf("Removed old state file: %s", stateFiles[i].Name())
		}
	}
}

func startAutoSave() {
	ticker := time.NewTicker(10 * time.Second)
	go func() {
		for range ticker.C {
			saveGridState()
		}
	}()
}

func main() {
	http.HandleFunc("/ws", wsHandler)
	http.HandleFunc("/import_v5dr6bft7ngy", importGridState)

	http.Handle("/", http.FileServer(http.FS(staticFiles)))

	// Start auto-save
	startAutoSave()

	log.Fatal(http.ListenAndServe("0.0.0.0:9090", nil))
}
