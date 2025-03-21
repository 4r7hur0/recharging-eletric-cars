package main

import (
	"encoding/json"
	"fmt"
	"net"
	"time"
)

var reservationQueue []string

type ChargingPoint struct {
	ID        string  `json:"id"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Available bool    `json:"available"`
	Queue     int     `json:"queue_size"`
}

type ReservationRequest struct {
	VehicleID string `json:"vehicleid"`
}

func addReserve(vehicleID string) {
	reservationQueue = append(reservationQueue, vehicleID)
}	

func sendStatusUpdate (){
	addr, _ := net.ResolveUDPAddr("udp", "server_adress:9090")
	conn, _ := net.DialUDP("udp", nil, addr)
	defer conn.Close()

	cp := ChargingPoint{

		ID: "CP01",
		Latitude: 0,
		Longitude: 0,
		Available: true,
		Queue: len(reservationQueue),
	}

	for {
		cp.Queue = len(reservationQueue)
		data, _ := json.Marshal(cp)
		conn.Write(data)
		time.Sleep(5 * time.Second)
	}
}

func tcpListener (){
	listener, _ := net.Listen("tcp", ":8081")
	defer listener.Close()
	fmt.Println("tamo ouvindo")

	for {
		//read the server response
		conn, _ := listener.Accept()
		buffer := make([]byte, 1024)
		n, _ := conn.Read(buffer)

		//take the ID from the vehicle and deserialize
		var reserve ReservationRequest
		json.Unmarshal(buffer [:n], &reserve)

		//add reserve in the slice
		addReserve(reserve.VehicleID)
		fmt.Println("New reservation made: ", reserve.VehicleID)
	}
}

func main () {
	go sendStatusUpdate()
	go tcpListener()

	select {}

}


