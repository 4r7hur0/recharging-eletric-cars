package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net"
	"sort"
	"sync"
	"time"
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
	ID string `json:"id"`
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

var (
	chargingPoints     = make(map[string]*ChargingPoint)
	chargingPointsConn = make(map[string]net.Conn)
	cars               = make(map[string]net.Conn)
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

	carID := conn.RemoteAddr().String()
	mu.Lock()
	cars[carID] = conn
	mu.Unlock()

	fmt.Printf("New car connected: %s\n", carID)

	buffer := make([]byte, 1024)
	for {
		n, err := conn.Read(buffer)
		if err != nil {
			// remove carID from cars map
			fmt.Printf("Car disconnected: %s\n", carID)
			mu.Lock()
			delete(cars, carID)
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
		case "confirmacao":
			// Handle arrival confirmation
			// TODO: Implement arrival confirmation handling
		case "encerramento":
			// Handle session end
			// TODO: Implement session end handling
		}
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

	time.Sleep(time.Second * 2)

	response := Message{
		Type: "reserva_response",
		Data: json.RawMessage(`{"status": "success", "message": "Reserve confirmed"}`),
	}

	data, err = json.Marshal(response)
	if err != nil {
		fmt.Println("Error marshaling reservation response:", err)
		return
	}

	_, err = carConn.Write(data)
	if err != nil {
		fmt.Println("Error sending reservation response to car:", err)
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

	buffer := make([]byte, 1024)
	for {
		// Read initial status or updates
		n, err := conn.Read(buffer)

		// unmarshal the data
		var cp ChargingPoint
		if err := json.Unmarshal(buffer[:n], &cp); err != nil {
			fmt.Println("Error unmarshaling charging point data:", err)
			continue
		}

		if err != nil {
			fmt.Println("Error reading from charging point:", err)
			mu.Lock()
			delete(chargingPointsConn, cp.ID)
			delete(chargingPoints, cp.ID)
			mu.Unlock()
			fmt.Printf("Charging point disconnected: %s\n", cp.ID)
			return
		}

		mu.Lock()
		chargingPointsConn[cp.ID] = conn
		mu.Unlock()

		// Update charging point data
		mu.Lock()
		chargingPoints[cp.ID] = &cp
		mu.Unlock()
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
			fmt.Println("New car connected")
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
