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

func sendStatusUpdate() {
    addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:9090")
    if err != nil {
        fmt.Println("Error resolving UDP address:", err)
        return
    }

    conn, err := net.DialUDP("udp", nil, addr)
    if err != nil {
        fmt.Println("Error creating UDP connection:", err)
        return
    }
    defer conn.Close()

    cp := ChargingPoint{
        ID:        "CP01",
        Latitude:  0,
        Longitude: 0,
        Available: true,
        Queue:     len(reservationQueue),
    }

    for {
        cp.Queue = len(reservationQueue)
        data, err := json.Marshal(cp)
        if err != nil {
            fmt.Println("Error marshaling JSON:", err)
            return
        }

        _, err = conn.Write(data)
        if err != nil {
            fmt.Println("Error sending data:", err)
            return
        }

        time.Sleep(5 * time.Second)
    }
}

func isVehicleInQueue(vehicleID string) bool {
	for _, v := range reservationQueue {
		if v == vehicleID {
			return true
		}
	}
	return false
}

func tcpListener (){
	listener, _ := net.Listen("tcp", ":8080")
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
		if isVehicleInQueue(reserve.VehicleID) {
			fmt.Println("Vehicle alredy in queue:", reserve.VehicleID)
			conn.Write([]byte("Error: Vehicle alredy in queue \n"))
		} else {
		addReserve(reserve.VehicleID)
		fmt.Println("New reservation made: ", reserve.VehicleID)
		conn.Write([]byte("Reserve confirmed \n"))
		}
	}
}

func main () {
	go sendStatusUpdate()
	go tcpListener()

	select {}

}


