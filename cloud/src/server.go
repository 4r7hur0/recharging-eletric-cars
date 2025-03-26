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

var (
	chargingPoints = make(map[string]*ChargingPoint)
	cars           = make(map[string]*Car)
	mu             sync.RWMutex
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

	buffer := make([]byte, 1024)
	for {
		n, err := conn.Read(buffer)
		if err != nil {
			fmt.Println("Error reading from car:", err)
			return
		}

		var msg Message
		if err := json.Unmarshal(buffer[:n], &msg); err != nil {
			fmt.Println("Error unmarshaling car message:", err)
			continue
		}
		// Handle different message types from cars
		switch msg.Type {
		case "bateria":
			var batteryMsg BatteryNotification
			if err := json.Unmarshal(msg.Data, &batteryMsg); err != nil {
				fmt.Println("Error unmarshaling battery message:", err)
				continue
			}

			// Update car information
			mu.Lock()
			cars[batteryMsg.IDVeiculo] = &Car{ID: batteryMsg.IDVeiculo}
			mu.Unlock()

			// Find all charging points and calculate distances
			var availablePoints []ChargingPoint
			mu.RLock()
			for _, cp := range chargingPoints {
				// Calculate distance from car to charging point
				distance := calculateDistance(
					batteryMsg.Localizacao.Latitude,
					batteryMsg.Localizacao.Longitude,
					cp.Latitude,
					cp.Longitude,
				)

				// Create a copy of the charging point with distance
				pointWithDistance := *cp
				pointWithDistance.Distance = distance
				availablePoints = append(availablePoints, pointWithDistance)
			}
			mu.RUnlock()

			// Sort points by distance
			sort.Slice(availablePoints, func(i, j int) bool {
				return availablePoints[i].Distance < availablePoints[j].Distance
			})

			// Send available points back to the car
			response := Message{
				Type: "pontos_disponiveis",
				Data: json.RawMessage(fmt.Sprintf(`{"pontos": %s}`, must(json.Marshal(availablePoints)))),
			}
			responseData, _ := json.Marshal(response)
			conn.Write(responseData)

		case "reserva":
			// Handle reservation request
			// TODO: Implement reservation handling
		case "confirmacao":
			// Handle arrival confirmation
			// TODO: Implement arrival confirmation handling
		case "encerramento":
			// Handle session end
			// TODO: Implement session end handling
		}
	}
}

func handleChargingPointConnection(conn net.Conn) {
	defer conn.Close()

	buffer := make([]byte, 1024)
	for {
		n, err := conn.Read(buffer)
		if err != nil {
			fmt.Println("Error reading from charging point:", err)
			return
		}

		var cp ChargingPoint
		if err := json.Unmarshal(buffer[:n], &cp); err != nil {
			fmt.Println("Error unmarshaling charging point data:", err)
			continue
		}

		// Update charging point status
		mu.Lock()
		chargingPoints[cp.ID] = &cp
		mu.Unlock()

		// Send acknowledgment back to charging point
		response := "Status update received\n"
		conn.Write([]byte(response))
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
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
		fmt.Println("New charging point connected")
		go handleChargingPointConnection(conn)
	}
}
