package main

import (
	"encoding/json"
	"fmt"
	"net"
)

func startUDPServer() {
	addr, err := net.ResolveUDPAddr("udp", ":8082")
	if err != nil {
		fmt.Println("Error resolving UDP address:", err)
		return
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		fmt.Println("Error starting UDP server:", err)
		return
	}
	defer conn.Close()

	fmt.Println("UDP Server started - Listening for status updates on port 8082")

	buffer := make([]byte, 1024)
	for {
		n, remoteAddr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			fmt.Println("Error reading UDP:", err)
			continue
		}

		var cp ChargingPoint
		if err := json.Unmarshal(buffer[:n], &cp); err != nil {
			fmt.Println("Error unmarshaling charging point data:", err)
			continue
		}

		mu.Lock()
		chargingPoints[cp.ID] = &cp
		mu.Unlock()

		fmt.Printf("Received status update from %s (remote: %s) - Available: %v, Queue: %d\n",
			cp.ID, remoteAddr, cp.Available, cp.Queue)
	}
}
