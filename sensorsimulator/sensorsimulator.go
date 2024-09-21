package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type SensorReading struct {
	SensorID  string    `json:"sensor_id"`
	Value     float64   `json:"value"`
	Timestamp time.Time `json:"timestamp"`
}

type Sensor struct {
	ID       string
	Type     string
	MinValue float64
	MaxValue float64
}

var sensors = []Sensor{
	{
		ID:       "c8bf7abf-b93f-4980-b224-bbcf260ed9a2",
		Type:     "temperature",
		MinValue: 2,
		MaxValue: 50,
	},
}
var (
	mqttBroker   = "tcp://localhost:1883" // Change this to your MQTT broker address
	sensorTopic  = "sensors"
	publishDelay = 5 * time.Second
)

func main() {
	// Create MQTT client
	opts := mqtt.NewClientOptions().AddBroker(mqttBroker)
	opts.SetClientID("sensor-simulator")
	client := mqtt.NewClient(opts)

	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Fatal(token.Error())
	}

	// Start publishing sensor data
	done := make(chan struct{})
	go publishSensorData(client, sensors, done)

	// Wait for interrupt signal to gracefully shutdown the server
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c

	close(done)
	client.Disconnect(250)
	fmt.Println("Sensor simulator stopped")
}

// func createSensors(count int) []Sensor {
// 	sensors := make([]Sensor, count)
// 	sensorTypes := []string{"temperature", "humidity", "pressure", "light"}

// 	for i := 0; i < count; i++ {
// 		sensorType := sensorTypes[i%len(sensorTypes)]
// 		sensors[i] = Sensor{
// 			ID:        uuid.New().String(),
// 			Type:      sensorType,
// 		}
// 	}

// 	return sensors
// }

// func getUnitForType(sensorType string) string {
// 	switch sensorType {
// 	case "temperature":
// 		return "°C"
// 	case "humidity":
// 		return "%"
// 	case "pressure":
// 		return "hPa"
// 	case "light":
// 		return "lux"
// 	default:
// 		return ""
// 	}
// }

func publishSensorData(client mqtt.Client, sensors []Sensor, done chan struct{}) {
	ticker := time.NewTicker(publishDelay)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			for _, sensor := range sensors {
				reading := SensorReading{
					SensorID:  sensor.ID,
					Value:     generateRandomValue(sensor.MinValue, sensor.MaxValue),
					Timestamp: time.Now().UTC(),
				}

				payload, err := json.Marshal(reading)
				if err != nil {
					log.Printf("Error marshaling sensor data: %v", err)
					continue
				}

				token := client.Publish(sensorTopic, 0, false, payload)
				token.Wait()

				if token.Error() != nil {
					log.Printf("Error publishing sensor data: %v", token.Error())
				} else {
					fmt.Printf("Published data for sensor %s: %v\n", sensor.ID, reading)
				}
			}
		case <-done:
			return
		}
	}
}

func generateRandomValue(min, max float64) float64 {
	return min + rand.Float64()*(max-min)
}
