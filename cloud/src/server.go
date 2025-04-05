package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net"
	"sort"
	"sync"
)

// Message types for different components
type Message struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// ChargingPoint represents a charging point in the system
type ChargingPoint struct {
	ID        string  `json:"id"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Available bool    `json:"available"`
	Queue     int     `json:"queue_size"`
	Distance  float64 `json:"distance,omitempty"` // Distance from the car
}

// Car represents a car in the system
type Car struct {
	ID            string  `json:"id"`
	PaymentMethod string  `json:"payment_method"`
	Debits        []Debit `json:"debits"`
}

type Debit struct {
	Amount        float64 `json:"amount"`
	PaymentMethod string  `json:"payment_method"`
}

// BatteryNotification represents a battery status update from a car
type BatteryNotification struct {
	Tipo         string  `json:"tipo"`
	IDVeiculo    string  `json:"id_veiculo"`
	NivelBateria float64 `json:"nivel_bateria"`
	Localizacao  Coords  `json:"localizacao"`
	Timestamp    string  `json:"timestamp"`
}

// Coords represents geographical coordinates
type Coords struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type ReservePoint struct {
	Tipo           string `json:"tipo"`
	IDVeiculo      string `json:"id_veiculo"`
	IDPontoRecarga string `json:"id_ponto_recarga"`
	TempoEstimado  int    `json:"tempo_estimado"`
	Timestamp      string `json:"timestamp"`
}

type confirmation struct {
	IDVeicle        string  `json:"id_veiculo"`
	IDChargingPoint string  `json:"id_ponto_recarga"`
	LevelBattery    float64 `json:"nivel_bateria"`
	LevelCharge     float64 `json:"nivel_carregar"`
	PaymentMethod   string  `json:"metodo_pagamento"`
	Timestamp       string  `json:"timestamp"`
}

var (
	chargingPoints     = make(map[string]*ChargingPoint)
	chargingPointsConn = make(map[string]net.Conn)
	carsConn           = make(map[string]net.Conn)
	cars               = make(map[string]*Car)
	mu                 sync.RWMutex
)

// calculateDistance calculates the distance between two points using the Haversine formula
func calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	// Convert latitude and longitude to radians
	lat1Rad := lat1 * math.Pi / 180
	lon1Rad := lon1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180
	lon2Rad := lon2 * math.Pi / 180

	// Earth's radius in kilometers
	const R = 6371.0

	// Differences in coordinates
	dlat := lat2Rad - lat1Rad
	dlon := lon2Rad - lon1Rad

	// Haversine formula
	a := math.Sin(dlat/2)*math.Sin(dlat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(dlon/2)*math.Sin(dlon/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	// Calculate distance in kilometers
	distance := R * c

	return distance
}

func handleCarConnection(conn net.Conn) {
	defer conn.Close()

	ID := conn.RemoteAddr().String()
	_, ID, err := net.SplitHostPort(ID)
	if err != nil {
		fmt.Printf("Error extracting port: %v\n", err)
		return
	}

	IDVeiculo := fmt.Sprintf("CAR%s", ID)

	// Add car connection to the map
	mu.Lock()
	carsConn[IDVeiculo] = conn
	cars[IDVeiculo] = &Car{ID: IDVeiculo}
	mu.Unlock()
	fmt.Printf("Car connected: %s\n", IDVeiculo)

	buffer := make([]byte, 1024)
	for {
		n, err := conn.Read(buffer)
		if err != nil {
			// remove carID from cars map
			fmt.Printf("Car disconnected: %s\n", IDVeiculo)
			mu.Lock()
			delete(carsConn, IDVeiculo)
			delete(cars, IDVeiculo)
			mu.Unlock()
			return
		}
		// unmarshal the data

		var msg Message
		if err := json.Unmarshal(buffer[:n], &msg); err != nil {
			fmt.Println("Error unmarshaling car message:", err)
			continue
		}
		// Handle different message types from cars
		switch msg.Type {
		case "bateria":
			// Handle battery notification
			var batteryMsg BatteryNotification
			if err := json.Unmarshal(msg.Data, &batteryMsg); err != nil {
				fmt.Println("Error unmarshaling battery message:", err)
				continue
			}
			sendAvailablePoints(batteryMsg, conn)
		case "reserva":
			// Handle reservation request
			var reservaMsg ReservePoint
			if err := json.Unmarshal(msg.Data, &reservaMsg); err != nil {
				fmt.Println("Error unmarshaling reservation message:", err)
				continue
			}
			handleReservation(reservaMsg, conn)
		case "chegada":
			// Handle arrival confirmation
			var confirmMsg confirmation
			if err := json.Unmarshal(msg.Data, &confirmMsg); err != nil {
				fmt.Println("Error unmarshaling confirmation message:", err)
				continue
			}

			cars[confirmMsg.IDVeicle] = &Car{PaymentMethod: confirmMsg.PaymentMethod}

			handlearrivial(confirmMsg)
		case "consulta":
			// Handle consultation request
			var sortedPoints []ChargingPoint
			mu.RLock()
			for _, cp := range chargingPoints {
				sortedPoints = append(sortedPoints, *cp)
			}
			mu.RUnlock()

			sort.Slice(sortedPoints, func(i, j int) bool {
				return sortedPoints[i].ID < sortedPoints[j].ID
			})

			response := struct {
				Type string          `json:"type"`
				Data []ChargingPoint `json:"data"`
			}{
				Type: "lista_pontos",
				Data: sortedPoints,
			}

			data, err := json.Marshal(response)
			if err != nil {
				fmt.Println("Error marshaling sorted points response:", err)
				return
			}

			_, err = conn.Write(data)
			if err != nil {
				fmt.Println("Error sending sorted points response:", err)
				return
			}

		}
	}
}

func handlearrivial(confirmMsg confirmation) {
	mu.Lock()
	defer mu.Unlock()

	// Find the charging point
	_, exists := chargingPoints[confirmMsg.IDChargingPoint]
	if !exists {
		fmt.Printf("Charging point %s not found\n", confirmMsg.IDChargingPoint)
		return
	}
	// Create message to send to car
	msg := Message{
		Type: "chegada",
		Data: json.RawMessage(fmt.Sprintf(`{
			"id_veiculo": "%s",
			"nivel_bateria": %.2f,
			"nivel_carregar": %.2f
		}`, confirmMsg.IDVeicle, confirmMsg.LevelBattery, confirmMsg.LevelCharge)),
	}

	// Marshal the message
	data, err := json.Marshal(msg)
	if err != nil {
		fmt.Println("Error marshaling confirmation message:", err)
		return
	}
	// Send confirmation request to charging point
	conn := chargingPointsConn[confirmMsg.IDChargingPoint]
	_, err = conn.Write(data)
	if err != nil {
		fmt.Printf("Error sending confirmation request to charging point %s: %v\n",
			confirmMsg.IDChargingPoint, err)
		return
	}

}

func handleReservation(reservaMsg ReservePoint, carConn net.Conn) {
	mu.Lock()
	defer mu.Unlock()

	// Find the charging point
	_, exists := chargingPoints[reservaMsg.IDPontoRecarga]
	if !exists {
		fmt.Printf("Charging point %s not found\n", reservaMsg.IDPontoRecarga)
		return
	}

	// Create message to send to charging point
	msg := Message{
		Type: "reserva",
		Data: json.RawMessage(fmt.Sprintf(`{"vehicleid":"%s"}`, reservaMsg.IDVeiculo)),
	}

	// Marshal the message
	data, err := json.Marshal(msg)
	if err != nil {
		fmt.Println("Error marshaling reservation message:", err)
		return
	}

	// Send reservation request to charging point
	conn := chargingPointsConn[reservaMsg.IDPontoRecarga]
	_, err = conn.Write(data)
	if err != nil {
		fmt.Printf("Error sending reservation request to charging point %s: %v\n",
			reservaMsg.IDPontoRecarga, err)
		return
	}

}

func sendAvailablePoints(msg BatteryNotification, conn net.Conn) {
	// Calculate distances for each charging point
	points := make([]ChargingPoint, 0, len(chargingPoints))
	for _, cp := range chargingPoints {
		cpCopy := *cp
		cpCopy.Distance = calculateDistance(
			msg.Localizacao.Latitude,
			msg.Localizacao.Longitude,
			cp.Latitude,
			cp.Longitude,
		)
		points = append(points, cpCopy)
	}

	// Sort points by distance
	sort.Slice(points, func(i, j int) bool {
		return points[i].Distance < points[j].Distance
	})

	// Create response message in the exact format expected by the car
	response := struct {
		Type string `json:"type"`
		Data struct {
			Pontos []ChargingPoint `json:"pontos"`
		} `json:"data"`
	}{
		Type: "pontos_disponiveis",
		Data: struct {
			Pontos []ChargingPoint `json:"pontos"`
		}{
			Pontos: points,
		},
	}

	// Marshal final response
	data, err := json.Marshal(response)
	if err != nil {
		fmt.Println("Error marshaling response:", err)
		return
	}

	_, err = conn.Write(data)
	if err != nil {
		fmt.Println("Error sending available points:", err)
		return
	}
}

func handleChargingPointConnection(conn net.Conn) {
	defer conn.Close()

	// Buffer para leitura de mensagens
	buffer := make([]byte, 1024)

	// Lê os dados iniciais do ponto de recarga (espera-se que contenham informações do ponto)
	n, err := conn.Read(buffer)
	if err != nil {
		fmt.Println("Error reading initial data from charging point:", err)
		return
	}

	// Desserializa os dados iniciais para obter informações do ponto de recarga
	var cp ChargingPoint
	if err := json.Unmarshal(buffer[:n], &cp); err != nil {
		fmt.Printf("Error unmarshaling initial charging point data: %v\n", err)
		return
	}

	// Adiciona o ponto de recarga aos mapas
	mu.Lock()
	chargingPointsConn[cp.ID] = conn
	chargingPoints[cp.ID] = &cp
	mu.Unlock()

	fmt.Printf("New charging point connected: %s (Location: %.6f, %.6f)\n", cp.ID, cp.Latitude, cp.Longitude)

	// Loop para processar mensagens do ponto de recarga
	for {
		n, err := conn.Read(buffer)
		if err != nil {
			fmt.Println("Error reading from charging point:", err)
			mu.Lock()
			// Remove o ponto de recarga dos mapas em caso de desconexão
			delete(chargingPointsConn, cp.ID)
			delete(chargingPoints, cp.ID)
			mu.Unlock()
			fmt.Printf("Charging point disconnected: %s\n", cp.ID)
			return
		}

		// Desserializa a mensagem recebida
		var msg Message
		if err := json.Unmarshal(buffer[:n], &msg); err != nil {
			fmt.Printf("Error unmarshaling message from charging point: %v\n", err)
			continue
		}

		// Processa o tipo de mensagem
		switch msg.Type {
		case "reserva_response":
			var response struct {
				Status    string `json:"status"`
				Message   string `json:"message"`
				VehicleID string `json:"vehicle_id"`
			}
			if err := json.Unmarshal(msg.Data, &response); err != nil {
				fmt.Printf("Error unmarshaling reserva_response: %v\n", err)
				continue
			}

			// Encaminha a resposta para o carro correspondente
			mu.RLock()
			carConn, exists := carsConn[response.VehicleID]
			mu.RUnlock()
			if !exists {
				fmt.Printf("Car connection not found for vehicle ID: %s\n", response.VehicleID)
				continue
			}

			// Envia a resposta para o carro
			_, err := carConn.Write(buffer[:n])
			if err != nil {
				fmt.Printf("Error sending reserva_response to car %s: %v\n", response.VehicleID, err)
			} else {
				fmt.Printf("Reservation response forwarded to car %s: %s\n", response.VehicleID, string(buffer[:n]))
			}

		case "custo_final":
			var finalCost struct {
				VehicleID string  `json:"vehicle_id"`
				Cost      float64 `json:"cost"`
			}
			if err := json.Unmarshal(msg.Data, &finalCost); err != nil {
				fmt.Printf("Error unmarshaling custo_final: %v\n", err)
				continue
			}

			// Create a message to send to the car
			msg := Message{
				Type: "custo_final",
				Data: json.RawMessage(fmt.Sprintf(`{"vehicle_id":"%s", "cost":%.2f, "payment_method":"%s"}`, finalCost.VehicleID, finalCost.Cost, cars[finalCost.VehicleID].PaymentMethod)),
			}

			// Marshal the message
			data, err := json.Marshal(msg)
			if err != nil {
				fmt.Printf("Error marshaling custo_final message: %v\n", err)
				continue
			}

			// Forward the final cost to the car
			mu.RLock()
			carConn, exists := carsConn[finalCost.VehicleID]
			cars[finalCost.VehicleID].Debits = append(cars[finalCost.VehicleID].Debits, Debit{Amount: finalCost.Cost, PaymentMethod: cars[finalCost.VehicleID].PaymentMethod})
			mu.RUnlock()
			if !exists {
				fmt.Printf("Car connection not found for vehicle ID: %s\n", finalCost.VehicleID)
				continue
			}

			_, err = carConn.Write(data)
			if err != nil {
				fmt.Printf("Error sending custo_final to car %s: %v\n", finalCost.VehicleID, err)
			} else {
				fmt.Printf("Final cost forwarded to car %s: %s\n", finalCost.VehicleID, string(msg.Data))
			}

		case "encerramento":
			var encerramento struct {
				VehicleID      string `json:"id_veiculo"`
				BatteryCharged int    `json:"energia_consumida"`
			}
			if err := json.Unmarshal(msg.Data, &encerramento); err != nil {
				fmt.Printf("Error unmarshaling encerramento: %v\n", err)
				continue
			}

			data, err := json.Marshal(encerramento)
			if err != nil {
				fmt.Printf("Error marshaling encerramento message: %v\n", err)
				continue
			}

			// Forward encerramento message to the car
			mu.RLock()
			carConn, exists := carsConn[encerramento.VehicleID]
			mu.RUnlock()
			if !exists {
				fmt.Printf("Car connection not found for vehicle ID: %s\n", encerramento.VehicleID)
				continue
			}

			_, err = carConn.Write(data)
			if err != nil {
				fmt.Printf("Error sending encerramento to car %s: %v\n", encerramento.VehicleID, err)
			} else {
				fmt.Printf("Encerramento message forwarded to car %s: %s\n", encerramento.VehicleID, string(data))
			}

		default:
			fmt.Printf("Unknown message type received from charging point: %s\n", msg.Type)
		}
	}
}

func main() {
	// Listen for car connections on port 8081
	carListener, err := net.Listen("tcp", ":8081")
	if err != nil {
		fmt.Println("Error starting car listener:", err)
		return
	}
	defer carListener.Close()

	// Listen for charging point connections on port 8080
	cpListener, err := net.Listen("tcp", ":8080")
	if err != nil {
		fmt.Println("Error starting charging point listener:", err)
		return
	}
	defer cpListener.Close()

	fmt.Println("Server started - Listening for connections...")
	fmt.Println("Cars on port 8081, Charging Points on port 8080")

	// Start UDP server for status updates
	go startUDPServer()

	// Handle car connections
	go func() {
		for {
			conn, err := carListener.Accept()
			if err != nil {
				fmt.Println("Error accepting car connection:", err)
				continue
			}
			go handleCarConnection(conn)
		}
	}()

	// Handle charging point connections
	for {
		conn, err := cpListener.Accept()
		if err != nil {
			fmt.Println("Error accepting charging point connection:", err)
			continue
		}
		go handleChargingPointConnection(conn)
	}
}
