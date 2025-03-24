package main

import (
	"encoding/json"
	"fmt"
	"net"
	"time"
)

type ReservationRequest struct {
	VehicleID string `json:"vehicleid"`
}

type ChargingPointStatus struct {
	ID        string  `json:"id"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Available bool    `json:"available"`
	Queue     int     `json:"queue_size"`
}

//Here we test the reservation
func testTCPConnection(vehicleID string) {
	conn, err := net.Dial("tcp", "localhost:8080")
	if err != nil {
		fmt.Println("Error connecting to TCP server:", err)
		return
	}
	defer conn.Close()

	request := ReservationRequest{VehicleID: vehicleID}
	data, _ := json.Marshal(request)

	_, err = conn.Write(data)
	if err != nil {
		fmt.Println("Error sending TCP request:", err)
	}

	fmt.Println("TCP reservation sent for vehicle:", vehicleID)
}

//Test the status update via udp
func testUDPListener() {
	addr, err := net.ResolveUDPAddr("udp", ":9090")
	if err != nil {
		fmt.Println("Error resolving UDP address:", err)
		return
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		fmt.Println("Error listening for UDP messages:", err)
		return
	}
	defer conn.Close()

	buffer := make([]byte, 1024)
	for {
		n, _, err := conn.ReadFromUDP(buffer)
		if err != nil {
			fmt.Println("Error reading UDP message:", err)
			continue
		}

		var status ChargingPointStatus
		json.Unmarshal(buffer[:n], &status)
		fmt.Printf("Received status update: %+v\n", status)
	}
}

func main() {
	go testUDPListener()

	time.Sleep(5 * time.Second) //Time for the UDP Connection
	testTCPConnection("V1238")
	
	select {}
}
