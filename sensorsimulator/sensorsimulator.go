package main

import (
	"encoding/json"
	"flag"
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
	MinValue float64
	MaxValue float64
}

func main() {
	// Define input flags with default values
	sensorID := flag.String("sensor-id", "", "ID of the sensor (required)")
	minValue := flag.Float64("min-value", 2, "Minimum sensor value")
	maxValue := flag.Float64("max-value", 100, "Maximum sensor value")
	mqttBroker := flag.String("broker", "tcp://localhost:1883", "MQTT broker address")
	sensorTopic := flag.String("topic", "sensors", "MQTT topic for sensor data")
	publishDelay := flag.Duration("delay", 5*time.Second, "Delay between publishing sensor data")

	// Parse the flags
	flag.Parse()

	// Validate that sensorID is provided
	if *sensorID == "" {
		log.Fatal("The --sensor-id flag is required but was not provided.")
	}

	// Create MQTT client
	opts := mqtt.NewClientOptions().AddBroker(*mqttBroker)
	opts.SetClientID("sensor-simulator")
	client := mqtt.NewClient(opts)

	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Fatal(token.Error())
	}

	// Start publishing sensor data
	done := make(chan struct{})
	go publishSensorData(client, Sensor{
		ID:       *sensorID,
		MinValue: *minValue,
		MaxValue: *maxValue,
	}, *sensorTopic, *publishDelay, done)

	// Wait for interrupt signal to gracefully shutdown the server
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c

	close(done)
	client.Disconnect(250)
	fmt.Println("Sensor simulator stopped")
}

func publishSensorData(client mqtt.Client, sensor Sensor, topic string, delay time.Duration, done chan struct{}) {
	ticker := time.NewTicker(delay)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
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

			token := client.Publish(topic, 0, false, payload)
			token.Wait()

			if token.Error() != nil {
				log.Printf("Error publishing sensor data: %v", token.Error())
			} else {
				fmt.Printf("Published data for sensor %s: %v\n", sensor.ID, reading)
			}
		case <-done:
			return
		}
	}
}

func generateRandomValue(min, max float64) float64 {
	return min + rand.Float64()*(max-min)
}
