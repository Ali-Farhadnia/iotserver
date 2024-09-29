package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	_ "github.com/mattn/go-sqlite3"
	"github.com/rs/cors"
)

// Structs for our data models
type Sensor struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Unit      string    `json:"unit"`
	Timestamp time.Time `json:"timestamp"`
	Latitude  float64   `json:"latitude"`
	Longitude float64   `json:"longitude"`
	Details   string    `json:"details"`
}

type NewSensor struct {
	Type      string  `json:"type"`
	Unit      string  `json:"unit"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Details   string  `json:"details"`
}

type UpdateSensor struct {
	Type      string  `json:"type"`
	Unit      string  `json:"unit"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Details   string  `json:"details"`
}

type Record struct {
	ID        string    `json:"id"`
	SensorID  string    `json:"sensor_id"`
	Value     float64   `json:"value"`
	Timestamp time.Time `json:"timestamp"`
}

// Global variables
var db *sql.DB
var mqttClient mqtt.Client
var mqttBroker string
var sensorTopic string

func main() {
	var err error
	// Initialize the SQLite database
	db, err = sql.Open("sqlite3", "/app/data/mydb.db") // Ensure this path is correct and matches your Docker setup
	if err != nil {
		// Handle error if the database connection cannot be opened
		panic(fmt.Sprintf("Failed to open the database: %v", err))
	}

	// Create tables if they don't exist
	createTables()

	// Set up HTTP server
	router := mux.NewRouter()
	setupRoutes(router)

	// Create a new CORS handler
	co := cors.New(cors.Options{
		AllowedOrigins:   []string{"*"}, // Allow all origins
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"*"}, // Allow all headers
		AllowCredentials: true,
		// Enable Debugging for testing, consider disabling in production
		Debug: true,
	})

	// Wrap router with CORS handler
	handler := co.Handler(router)

	// Start the server
	go func() {
		log.Println("Starting server on :9090")
		log.Fatal(http.ListenAndServe(":9090", handler))
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c

	log.Println("Shutting down...")
	mqttClient.Disconnect(250)
}

func createTables() {
	// Create sensors table
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS sensors (
		id TEXT PRIMARY KEY,
		type TEXT,
		unit TEXT,
		timestamp DATETIME,
		latitude REAL,
		longitude REAL,
		details TEXT
	)`)
	if err != nil {
		log.Fatal(err)
	}

	// Create records table
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS records (
	    id TEXT  PRIMARY KEY,
		sensor_id TEXT,
		value REAL,
		timestamp DATETIME,
		FOREIGN KEY(sensor_id) REFERENCES sensors(id)
	)`)
	if err != nil {
		log.Fatal(err)
	}
}

func initMQTT() {
	opts := mqtt.NewClientOptions().AddBroker(mqttBroker)
	opts.SetClientID("golang-mqtt-client")
	mqttClient = mqtt.NewClient(opts)
	if token := mqttClient.Connect(); token.Wait() && token.Error() != nil {
		log.Fatal(token.Error())
	}

	if token := mqttClient.Subscribe(sensorTopic, 0, messageHandler); token.Wait() && token.Error() != nil {
		log.Fatal(token.Error())
	}
}

func messageHandler(client mqtt.Client, msg mqtt.Message) {
	var record Record
	err := json.Unmarshal(msg.Payload(), &record)
	if err != nil {
		log.Println("Error decoding MQTT message:", err)
		return
	}

	// Check if sensor exists
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM sensors WHERE id = ?", record.SensorID).Scan(&count)
	if err != nil {
		log.Println("Error checking sensor existence:", err)
		return
	}

	if count == 0 {
		log.Println("Unknown sensor ID:", record.SensorID)
		return
	}

	// Insert record into database
	_, err = db.Exec("INSERT INTO records (id,sensor_id, value, timestamp) VALUES (?,?, ?, ?)",
		uuid.NewString(), record.SensorID, record.Value, record.Timestamp)
	if err != nil {
		log.Println("Error inserting record:", err)
	}
}

func setupRoutes(router *mux.Router) {
	router.HandleFunc("/mqtt-config", setMQTTConfig).Methods("POST")
	router.HandleFunc("/sensors", createSensor).Methods("POST")
	router.HandleFunc("/sensors", querySensors).Methods("GET")
	router.HandleFunc("/sensors/{id}", updateSensor).Methods("PUT")
	router.HandleFunc("/sensors/{id}", deleteSensor).Methods("DELETE")
	router.HandleFunc("/records/{sensor_id}", queryRecords).Methods("GET")
	// Add WebSocket route for real-time querying
	router.HandleFunc("/ws/records/{sensor_id}", handleWebSocket)

	// Log all registered routes
	router.Walk(func(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
		pathTemplate, _ := route.GetPathTemplate()
		methods, _ := route.GetMethods()
		log.Printf("Route: %s, Methods: %v\n", pathTemplate, methods)
		return nil
	})
}

// API handlers
func setMQTTConfig(w http.ResponseWriter, r *http.Request) {
	var config struct {
		Broker string `json:"broker"`
		Topic  string `json:"topic"`
	}

	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Update global variables
	mqttBroker = config.Broker
	sensorTopic = config.Topic

	// Reconnect MQTT client with new configuration
	if mqttClient != nil && mqttClient.IsConnected() {
		mqttClient.Disconnect(250)
	}
	initMQTT()

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "MQTT configuration updated successfully"})
}

func createSensor(w http.ResponseWriter, r *http.Request) {
	var newSensor NewSensor
	err := json.NewDecoder(r.Body).Decode(&newSensor)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	sensor := Sensor{
		ID:        uuid.NewString(),
		Type:      newSensor.Type,
		Unit:      newSensor.Unit,
		Timestamp: time.Now(),
		Latitude:  newSensor.Latitude,
		Longitude: newSensor.Longitude,
		Details:   newSensor.Details,
	}

	_, err = db.Exec("INSERT INTO sensors (id, type, unit, timestamp, latitude, longitude, details) VALUES (?, ?, ?, ?, ?, ?, ?)",
		sensor.ID, sensor.Type, sensor.Unit, sensor.Timestamp, sensor.Latitude, sensor.Longitude, sensor.Details)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(sensor)
}

func querySensors(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters for pagination
	limit := 10 // Default limit
	offset := 0 // Default offset

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if parsedOffset, err := strconv.Atoi(offsetStr); err == nil && parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	// Query sensors
	rows, err := db.Query(`
		SELECT id, type, unit, timestamp, latitude, longitude, details 
		FROM sensors 
		ORDER BY timestamp DESC 
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		http.Error(w, "Error querying sensors", http.StatusInternalServerError)
		log.Println("Error querying sensors:", err)
		return
	}
	defer rows.Close()

	var sensors []Sensor
	for rows.Next() {
		var s Sensor
		if err := rows.Scan(&s.ID, &s.Type, &s.Unit, &s.Timestamp, &s.Latitude, &s.Longitude, &s.Details); err != nil {
			http.Error(w, "Error scanning sensors", http.StatusInternalServerError)
			log.Println("Error scanning sensors:", err)
			return
		}
		sensors = append(sensors, s)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sensors)
}

func updateSensor(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sensorID := vars["id"]

	var updatedSensor UpdateSensor
	if err := json.NewDecoder(r.Body).Decode(&updatedSensor); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Update the sensor in the database
	_, err := db.Exec(`
		UPDATE sensors 
		SET type = ?, unit = ?, latitude = ?, longitude = ?, details = ?
		WHERE id = ?
	`, updatedSensor.Type, updatedSensor.Unit, updatedSensor.Latitude, updatedSensor.Longitude, updatedSensor.Details, sensorID)

	if err != nil {
		http.Error(w, "Error updating sensor", http.StatusInternalServerError)
		log.Println("Error updating sensor:", err)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(updatedSensor)
}

func deleteSensor(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sensorID := vars["id"]

	// Start a transaction to ensure both deletions occur or neither does
	tx, err := db.Begin()
	if err != nil {
		http.Error(w, "Error starting transaction", http.StatusInternalServerError)
		log.Println("Error starting transaction:", err)
		return
	}

	// Delete related records first (due to foreign key constraint)
	_, err = tx.Exec("DELETE FROM records WHERE sensor_id = ?", sensorID)
	if err != nil {
		tx.Rollback()
		http.Error(w, "Error deleting related records", http.StatusInternalServerError)
		log.Println("Error deleting related records:", err)
		return
	}

	// Delete the sensor
	result, err := tx.Exec("DELETE FROM sensors WHERE id = ?", sensorID)
	if err != nil {
		tx.Rollback()
		http.Error(w, "Error deleting sensor", http.StatusInternalServerError)
		log.Println("Error deleting sensor:", err)
		return
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		http.Error(w, "Error committing transaction", http.StatusInternalServerError)
		log.Println("Error committing transaction:", err)
		return
	}

	// Check if a sensor was actually deleted
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "Sensor not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func queryRecords(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sensorID := vars["sensor_id"]

	// Parse query parameters
	limit := 100 // Default limit
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil {
			limit = parsedLimit
		}
	}

	startTime := time.Time{}
	if startTimeStr := r.URL.Query().Get("start_time"); startTimeStr != "" {
		if parsedTime, err := time.Parse(time.RFC3339, startTimeStr); err == nil {
			startTime = parsedTime
		}
	}

	endTime := time.Now()
	if endTimeStr := r.URL.Query().Get("end_time"); endTimeStr != "" {
		if parsedTime, err := time.Parse(time.RFC3339, endTimeStr); err == nil {
			endTime = parsedTime
		}
	}

	// Query records
	rows, err := db.Query(`
		SELECT id, sensor_id, value, timestamp 
		FROM records 
		WHERE sensor_id = ? AND timestamp BETWEEN ? AND ?
		ORDER BY timestamp DESC 
		LIMIT ?
	`, sensorID, startTime, endTime, limit)
	if err != nil {
		http.Error(w, "Error querying records", http.StatusInternalServerError)
		log.Println("Error querying records:", err)
		return
	}
	defer rows.Close()

	var records []Record
	for rows.Next() {
		var r Record
		if err := rows.Scan(&r.ID, &r.SensorID, &r.Value, &r.Timestamp); err != nil {
			http.Error(w, "Error scanning records", http.StatusInternalServerError)
			log.Println("Error scanning records:", err)
			return
		}
		records = append(records, r)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(records)
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	defer conn.Close()

	vars := mux.Vars(r)
	sensorID := vars["sensor_id"]

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			records, err := queryLatestRecords(sensorID)
			if err != nil {
				log.Println("Error querying records:", err)
				continue
			}

			if err := conn.WriteJSON(records); err != nil {
				log.Println("Error writing to WebSocket:", err)
				return
			}
		case <-r.Context().Done():
			return
		}
	}
}

func queryLatestRecords(sensorID string) ([]Record, error) {
	rows, err := db.Query("SELECT id, sensor_id, value, timestamp FROM records WHERE sensor_id = ? ORDER BY timestamp DESC LIMIT 10", sensorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []Record
	for rows.Next() {
		var r Record
		if err := rows.Scan(&r.ID, &r.SensorID, &r.Value, &r.Timestamp); err != nil {
			return nil, err
		}
		records = append(records, r)
	}

	return records, nil
}
