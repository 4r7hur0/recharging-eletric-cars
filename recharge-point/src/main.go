package main

import (
	"encoding/json"
	"fmt"
	"net"
	"time"
)

var reservationQueue []string

type ChargingPoint struct {
	Type      string `json:"tipo"`
	StationID string `json:"station_id"`
	Location  Coods  `json:"location"`
	Available bool   `json:"availiable"` // Corrigido para coincidir com o servidor
	Queue     int    `json:"queue"`
}

type Coods struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type ReservationRequest struct {
	VehicleID string `json:"vehicleid"`
}

func addReserve(vehicleID string) {
	reservationQueue = append(reservationQueue, vehicleID)
}

func sendStatusUpdate() {
	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:9090")
	conn, _ := net.DialUDP("udp", nil, addr)
	defer conn.Close()

	cp := ChargingPoint{
		StationID: "CP01",
		Location: Coods{
			Latitude:  -23.55052,
			Longitude: -46.633308,
		},
		Available: true,
		Queue:     len(reservationQueue),
	}

	for {
		cp.Queue = len(reservationQueue)
		data, _ := json.Marshal(cp)
		conn.Write(data)
		time.Sleep(5 * time.Second)
	}
}

func conectToServer() {
	for {
		conn, err := net.Dial("tcp", "localhost:8083")
		if err != nil {
			fmt.Println("Error connecting to server:", err)
			time.Sleep(5 * time.Second)
			continue
		}

		fmt.Println("Connected to server")

		handleConnection(conn)

		conn.Close()
		time.Sleep(5 * time.Second)
	}
}

func handleConnection(conn net.Conn) {
	buffer := make([]byte, 1024)

	for {
		n, err := conn.Read(buffer)
		if err != nil {
			fmt.Println("Error reading:", err)
			return
		}

		var req ReservationRequest
		err = json.Unmarshal(buffer[:n], &req)
		if err != nil {
			fmt.Println("Error unmarshalling:", err)
			continue
		}

		addReserve(req.VehicleID)
		fmt.Printf("Added reservation for vehicle %s\n", req.VehicleID)

		response := "Reservation added"
		conn.Write([]byte(response))
	}
}

func main() {
	go sendStatusUpdate()
	go conectToServer()

	select {}

}
