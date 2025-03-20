package main

import (
	"encoding/json"
	"fmt"
	"net"
	"time"
)

var reservationQueue []string

type Coords struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type ChargingPointUpdate struct {
	Type       string  `json:"type"`
	IDPoint    string  `json:"id_point"`
	Available  bool    `json:"available"`
	Location   *Coords `json:"location"`
	Timestamp  string  `json:"timestamp"` 
}


type ReservationRequest struct {
	VehicleID string `json:"vehicleid"`
}

func addReserve(vehicleID string) {
	reservationQueue = append(reservationQueue, vehicleID)
}	

func main() {
	conn, err := net.Dial("tcp", "localhost:8080")
	if err != nil {
		fmt.Println("Error connecting to server:", err)
		return
	}
	defer conn.Close()

	// Request information from the user
	var idPoint string
	var latitude, longitude float64

	fmt.Print("Enter the charging point ID: ")
	fmt.Scanln(&idPoint)

	fmt.Print("Enter latitude: ")
	fmt.Scanln(&latitude)

	fmt.Print("Enter longitude: ")
	fmt.Scanln(&longitude)

	// Converting numeric input to boolean
	availability := false

	// Create and send the message
	msg := ChargingPointUpdate{
		Type:      "charging_point",
		IDPoint:   idPoint,
		Available: availability,
		Location:  &Coords{Latitude: latitude, Longitude: longitude},
		Timestamp: time.Now().Format(time.RFC3339),
	}

	data, _ := json.Marshal(msg)
	conn.Write(data)

	// Read the server response
	buffer := make([]byte, 1024)
	n, _ := conn.Read(buffer)
	fmt.Println("Server response:", string(buffer[:n]))
}
