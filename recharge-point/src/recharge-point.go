package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"sync"
	"time"
)

// Global variables for managing reservation queue and connections
var (
	reservationQueue []string      // Queue of vehicle IDs waiting for charging
	mu               sync.Mutex    // Mutex to protect shared data access
	udpConn          *net.UDPConn  // UDP connection for status broadcasts
	logger           *log.Logger   // Custom logger for timestamp formatting
	clientID         string        // This charging point's unique identifier
)

// ChargingPoint represents the state of a charging station
type ChargingPoint struct {
	ID        string  `json:"id"`        
	Latitude  float64 `json:"latitude"`  
	Longitude float64 `json:"longitude"` 
	Available bool    `json:"available"` // Whether the point is free for charging
	Queue     int     `json:"queue_size"`// Number of vehicles in reservation queue
	VehicleID string  `json:"vehicle_id,omitempty"` // Currently charging vehicle 
	Battery   int     `json:"battery,omitempty"`    // Current battery level of charging vehicle
}

// ReservationRequest contains a vehicle's reservation data
type ReservationRequest struct {
	VehicleID string `json:"vehicleid"`
}

// Message is the general wrapper format for TCP communication
type Message struct {
	Type string          `json:"type"` // Message type identifier
	Data json.RawMessage `json:"data"` // Raw JSON payload
}

// Returns formatted timestamp for logging
func logTimestamp() string {
	return time.Now().Format("2006-01-02 15:04:05.000")
}

// Logs a message with component, severity level, and formatted content
func logMessage(component, level, format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	logger.Printf("[%s] [%s] [%s] %s", logTimestamp(), component, level, message)
}

// Generates random coordinates within São Paulo area
func generateRandomCoordinates(r *rand.Rand) (float64, float64) {
	minLat := -23.6500
	maxLat := -23.4500
	minLon := -46.7000
	maxLon := -46.5000
	lat := minLat + r.Float64()*(maxLat-minLat)
	lon := minLon + r.Float64()*(maxLon-minLon)
	return lat, lon
}

// Creates a unique charging point ID in format "CPXXXX"
func generateRandomID(r *rand.Rand) string {
	const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 4)
	for i := range b {
		b[i] = letters[r.Intn(len(letters))]
	}
	return "CP" + string(b)
}

// Adds a vehicle to the reservation queue
func addReserve(vehicleID string) {
	reservationQueue = append(reservationQueue, vehicleID)
	logMessage("QUEUE", "INFO", "New vehicle added to queue: %s (Current size: %d)", vehicleID, len(reservationQueue))
}

// Removes and returns the first vehicle from the reservation queue
func removeFromQueue() string {
	if len(reservationQueue) == 0 {
		return ""
	}
	vehicleID := reservationQueue[0]
	reservationQueue = reservationQueue[1:]
	logMessage("QUEUE", "INFO", "Vehicle %s removed from queue. Current queue size: %d", vehicleID, len(reservationQueue))
	return vehicleID
}

// Broadcasts the current charging point status via UDP
func sendStatusUpdate(cp ChargingPoint) {
	data, err := json.Marshal(cp)
	if err != nil {
		logMessage("UDP", "ERROR", "Failed to serialize status: %v", err)
		return
	}

	_, err = udpConn.Write(data)
	if err != nil {
		logMessage("UDP", "ERROR", "Failed to send status via UDP: %v", err)
		return
	}

	batteryInfo := ""
	if cp.Battery > 0 {
		batteryInfo = fmt.Sprintf(", Battery: %d%%", cp.Battery)
	}

	vehicleInfo := "none"
	if cp.VehicleID != "" {
		vehicleInfo = cp.VehicleID
	}

	logMessage("STATUS", "INFO", "Update sent - ID: %s, Available: %v, Vehicle: %s%s, Queue: %d",
		cp.ID, cp.Available, vehicleInfo, batteryInfo, cp.Queue)
}

// Calculates the cost of a charging session based on battery percentage charged
func calculateChargingCost(batteryCharged int) float64 {
	const pricePerPercent = 2.0
	return float64(batteryCharged) * pricePerPercent
}

// Sends the final cost for a charging session to the server
func sendFinalCost(conn net.Conn, vehicleID string, batteryCharged int) {
	cost := calculateChargingCost(batteryCharged)
	msg := Message{
		Type: "custo_final",
		Data: json.RawMessage(fmt.Sprintf(`{"vehicle_id":"%s","cost":%v}`, vehicleID, cost)),
	}

	logMessage("PAYMENT", "DEBUG", "Final cost JSON size: %d bytes", len(fmt.Sprintf(`{"vehicle_id":"%s","cost":%v}`, vehicleID, cost)))

	logMessage("PAYMENT", "INFO", "Sending final cost for vehicle %s: R$%.2f", vehicleID, cost)

	data, err := json.Marshal(msg)
	if err != nil {
		logMessage("PAYMENT", "ERROR", "Failed to serialize cost message for vehicle %s: %v", vehicleID, err)
		return
	}

	_, err = conn.Write(data)
	if err != nil {
		logMessage("PAYMENT", "ERROR", "Failed to send final cost to vehicle %s: %v", vehicleID, err)
		return
	}

	logMessage("PAYMENT", "INFO", "Final cost for vehicle %s: R$%.2f (Charged: %d%%)",
		vehicleID, cost, batteryCharged)
}

// Main handler for TCP connection with the central server
func handleConnection(conn net.Conn, r *rand.Rand) {
	defer conn.Close()

	remoteAddr := conn.RemoteAddr().String()
	logMessage("CONNECTION", "INFO", "New connection established with server at %s", remoteAddr)

	// Initialize this charging point with random location
	lat, lon := generateRandomCoordinates(r)
	cp := ChargingPoint{ 
		ID:        generateRandomID(r),
		Latitude:  lat,
		Longitude: lon,
		Available: true,
		Queue: 0, 
	}

	clientID = cp.ID 
	logMessage("INIT", "INFO", "New charging point initialized - ID: %s, Location: (%.6f, %.6f)",
		cp.ID, cp.Latitude, cp.Longitude)

	// Send initial data to register with central server
	data, err := json.Marshal(cp)
	if err != nil {
		logMessage("INIT", "ERROR", "Failed to serialize charging point data: %v", err)
		return 
	}
	_, err = conn.Write(data)
	if err != nil {
		logMessage("INIT", "ERROR", "Failed to send initial data to server: %v", err)
		return 
	}
	logMessage("INIT", "INFO", "Initial data sent to server successfully")
	mu.Lock()
	cp.Queue = len(reservationQueue) 
	mu.Unlock()
	sendStatusUpdate(cp) 

	// Main message processing loop
	buffer := make([]byte, 1024)
	for {
		// Read incoming messages from server
		n, err := conn.Read(buffer)
		if err != nil {
			// Handle connection errors
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				logMessage("CONNECTION", "WARN", "Read timeout. Closing connection.")
			} else if err == io.EOF {
				logMessage("CONNECTION", "INFO", "Connection closed by server (EOF)")
			} else {
				logMessage("CONNECTION", "ERROR", "Read operation failed: %v", err)
			}
			return // Exit and let main() reconnect
		}

		rawMsg := string(buffer[:n])
		logMessage("MESSAGE", "DEBUG", "Received raw data (%d bytes): %s", n, rawMsg)

		var msg Message
		// Parse the message envelope
		if err := json.Unmarshal(buffer[:n], &msg); err != nil {
			logMessage("MESSAGE", "ERROR", "Failed to decode message wrapper: %v - Raw data: %s", err, rawMsg)
			continue
		}

		logMessage("MESSAGE", "INFO", "Received message type: %s", msg.Type)

		// Critical section - acquire lock before accessing shared state
		mu.Lock()

		// Update queue size before processing message
		cp.Queue = len(reservationQueue)

		switch msg.Type {
		case "reserva":
			// Handle reservation request
			var reserve ReservationRequest
			if err := json.Unmarshal(msg.Data, &reserve); err != nil {
				logMessage("RESERVATION", "ERROR", "Invalid reservation request data: %v", err)
				response := Message{Type: "reserva_response", Data: json.RawMessage(fmt.Sprintf(`{"status":"error","message":"Invalid request","vehicle_id":"%s"}`, reserve.VehicleID))}
				responseData, _ := json.Marshal(response)
				conn.Write(responseData)
			} else {
				logMessage("RESERVATION", "INFO", "Processing reservation request for vehicle %s", reserve.VehicleID)
				if isVehicleInQueue(reserve.VehicleID) {
					// Vehicle already in queue
					logMessage("RESERVATION", "WARN", "Vehicle %s is already in queue - position: %d",
						reserve.VehicleID, findVehiclePosition(reserve.VehicleID))
					response := Message{Type: "reserva_response", Data: json.RawMessage(fmt.Sprintf(`{"status":"error","message":"Vehicle already in queue","vehicle_id":"%s"}`, reserve.VehicleID))}
					responseData, _ := json.Marshal(response)
					conn.Write(responseData)
				} else if len(reservationQueue) >= 5 {
					// Queue is full (max 5 vehicles)
					logMessage("RESERVATION", "WARN", "Queue is full (maximum 5 vehicles) - rejecting request from %s", reserve.VehicleID)
					cp.Available = false
					sendStatusUpdate(cp)
					response := Message{Type: "reserva_response", Data: json.RawMessage(fmt.Sprintf(`{"status":"error","message":"Queue full","vehicle_id":"%s"}`, reserve.VehicleID))}
					responseData, _ := json.Marshal(response)
					conn.Write(responseData)
				} else {
					// Reservation accepted
					addReserve(reserve.VehicleID)
					cp.Queue = len(reservationQueue)
					logMessage("RESERVATION", "INFO", "Reservation confirmed for vehicle %s - Queue position: %d",
						reserve.VehicleID, len(reservationQueue))
					sendStatusUpdate(cp)
					response := Message{Type: "reserva_response", Data: json.RawMessage(fmt.Sprintf(`{"status":"success","message":"Reserve confirmed","vehicle_id":"%s"}`, reserve.VehicleID))}
					responseData, _ := json.Marshal(response)
					conn.Write(responseData)
					logMessage("RESERVATION", "INFO", "Sent successful reservation response to server")
				}
			}

		case "chegada":
			// Handle vehicle arrival for charging
			var chegada struct {
				IDVeiculo         string  `json:"id_veiculo"`
				NivelBateria      float64 `json:"nivel_bateria"`
				NivelCarregamento float64 `json:"nivel_carregar"`
			}
			if err := json.Unmarshal(msg.Data, &chegada); err != nil {
				logMessage("ARRIVAL", "ERROR", "Invalid arrival data: %v - Raw data: %s", err, string(msg.Data))
			} else {
				logMessage("ARRIVAL", "INFO", "Vehicle arrival notification - ID: %s, Current Battery: %.1f%%, Target: %.1f%%",
					chegada.IDVeiculo, chegada.NivelBateria, chegada.NivelCarregamento)

				if len(reservationQueue) == 0 {
					logMessage("ARRIVAL", "WARN", "Vehicle %s arrived but queue is empty", chegada.IDVeiculo)
				} else if chegada.IDVeiculo != reservationQueue[0] {
					logMessage("ARRIVAL", "WARN", "Vehicle %s arrived but is not next in queue (expected: %s)",
						chegada.IDVeiculo, reservationQueue[0])
				} else {
					// Correct vehicle arrived - start charging process
					cp.VehicleID = chegada.IDVeiculo
					cp.Battery = int(chegada.NivelBateria)
					cp.Available = false
					batteryToCharge := int(chegada.NivelCarregamento) - int(chegada.NivelBateria)
					if batteryToCharge < 0 {
						batteryToCharge = 0
					}

					logMessage("CHARGING", "INFO", "Starting charging session for vehicle %s - Current: %d%%, Target: %.0f%% (Increase: %d%%)",
						cp.VehicleID, cp.Battery, chegada.NivelCarregamento, batteryToCharge)

					// Start charging in a separate goroutine
					go simulateCharging(conn, &cp, batteryToCharge)

					// Broadcast updated status
					sendStatusUpdate(cp)
				}
			}

		default:
			logMessage("MESSAGE", "WARN", "Unknown message type received: %s", msg.Type)
		}

		// Release the lock before next iteration
		mu.Unlock()
	}
}

// Simulates the charging process in a separate goroutine
func simulateCharging(conn net.Conn, cp *ChargingPoint, batteryToCharge int) {
	// Store vehicle info safely before releasing mutex
	mu.Lock()
	vehicleID := cp.VehicleID
	initialBattery := cp.Battery
	targetBattery := initialBattery + batteryToCharge
	chargingRate := 1 // Percent per second
	mu.Unlock()

	if vehicleID == "" {
		logMessage("CHARGING", "ERROR", "simulateCharging called with empty vehicle ID in cp")
		return
	}
	
	// Handle case where no charging is needed
	if batteryToCharge <= 0 {
		logMessage("CHARGING", "INFO", "No charging needed for vehicle %s (already at/above target).", vehicleID)
		batteryCharged := 0

		mu.Lock()
		// Clean up charging point state
		if cp.VehicleID == vehicleID {
			cp.Available = true
			cp.VehicleID = ""
			cp.Battery = 0
			removedVehicle := removeFromQueue()
			cp.Queue = len(reservationQueue)
			logMessage("CHARGING", "INFO", "Charging session finished immediately for %s (no charge needed). Removed: %s", vehicleID, removedVehicle)
			stateToSend := *cp
			mu.Unlock()

			sendStatusUpdate(stateToSend)
			sendFinalCost(conn, vehicleID, batteryCharged)

			// Send session end notification
			encerramentoMsg := Message{Type: "encerramento", Data: json.RawMessage(fmt.Sprintf(`{"id_veiculo":"%v","energia_consumida":%v}`, vehicleID, batteryCharged))}
			data, _ := json.Marshal(encerramentoMsg)
			_, err := conn.Write(data)
			if err != nil {
				logMessage("SESSION_END", "ERROR", "Failed to send session end message: %v", err)
			} else {
				logMessage("SESSION_END", "INFO", "Session end notification sent to server for vehicle %s", vehicleID)
			}

		} else {
			logMessage("CHARGING", "WARN", "Vehicle changed during zero-charge processing for %s. Current vehicle: %s", vehicleID, cp.VehicleID)
			mu.Unlock()
		}
		return
	}

	logMessage("CHARGING", "INFO", "Charging simulation started for vehicle %s - Initial: %d%%, Target: %d%% (Delta: %d%%)",
		vehicleID, initialBattery, targetBattery, batteryToCharge)

	// Charging loop - increments battery level until target reached
	for {
		time.Sleep(1 * time.Second)

		var shouldBreak bool = false
		var logMsg string = ""
		var logLvl string = "INFO"

		mu.Lock()
		// Check if vehicle is still the same and battery target not reached
		if cp.VehicleID != vehicleID {
			logMsg = fmt.Sprintf("Charging interrupted - Vehicle changed from %s to %s", vehicleID, cp.VehicleID)
			logLvl = "WARN"
			shouldBreak = true
		} else if cp.Battery >= targetBattery {
			logMsg = fmt.Sprintf("Target battery level reached for vehicle %s: %d%%", vehicleID, cp.Battery)
			shouldBreak = true
			cp.Battery = targetBattery
		} else {
			// Increment battery level
			cp.Battery += chargingRate
			if cp.Battery > targetBattery {
				cp.Battery = targetBattery
			}
			progress := float64(cp.Battery-initialBattery) / float64(batteryToCharge) * 100
			logMsg = fmt.Sprintf("Vehicle %s charging: %d%% (Progress: %.1f%%)", vehicleID, cp.Battery, progress)
		}
		mu.Unlock()

		if logMsg != "" {
			logMessage("CHARGING", logLvl, "%s", logMsg)
		}

		if shouldBreak {
			break
		}
	}

	// Finish charging session and cleanup
	var finalStateToSend ChargingPoint
	var batteryCharged int = 0

	mu.Lock()
	// Verify vehicle is still the same before finalizing
	if cp.VehicleID == vehicleID {
		batteryCharged = cp.Battery - initialBattery
		if batteryCharged < 0 {
			batteryCharged = 0
		}

		cp.Available = true
		cp.VehicleID = ""
		cp.Battery = 0
		removedVehicle := removeFromQueue()
		cp.Queue = len(reservationQueue)

		logMessage("CHARGING", "INFO", "Charging completed for vehicle %s - Total charged: %d%%, Vehicle removed: %s",
			vehicleID, batteryCharged, removedVehicle)

		finalStateToSend = *cp
		mu.Unlock()

		// Send final status updates and billing
		if finalStateToSend.ID != "" {
			sendStatusUpdate(finalStateToSend)
		}
		sendFinalCost(conn, vehicleID, batteryCharged)

		// Send session end notification
		encerramentoMsg := Message{
			Type: "encerramento",
			Data: json.RawMessage(fmt.Sprintf(`{"id_veiculo":"%s","energia_consumida":%d}`, vehicleID, batteryCharged)),
		}
		data, err := json.Marshal(encerramentoMsg)
		if err != nil {
			logMessage("SESSION_END", "ERROR", "Failed to serialize session end message: %v", err)
		} else {
			_, err = conn.Write(data)
			if err != nil {
				logMessage("SESSION_END", "ERROR", "Failed to send session end message: %v", err)
			} else {
				logMessage("SESSION_END", "INFO", "Session end notification sent to server for vehicle %s", vehicleID)
			}
		}

	} else {
		logMessage("CHARGING", "WARN", "Charging loop ended for %s, but current vehicle is now %s. No final actions taken for %s.",
			vehicleID, cp.VehicleID, vehicleID)
		mu.Unlock()
	}
}

// Checks if a vehicle is already in the reservation queue
func isVehicleInQueue(vehicleID string) bool {
	for _, v := range reservationQueue {
		if v == vehicleID {
			return true
		}
	}
	return false
}

// Finds a vehicle's position in the queue (1-based index)
func findVehiclePosition(vehicleID string) int {
	for i, v := range reservationQueue {
		if v == vehicleID {
			return i + 1
		}
	}
	return -1
}

func main() {
	// Initialize logger
	logger = log.New(os.Stdout, "", 0)
	
	// Create random number generator
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	logMessage("SYSTEM", "INFO", "Charging point client starting up...")

	// Set up UDP connection for status broadcasting
	serverAddr, err := net.ResolveUDPAddr("udp", "localhost:8082")
	if err != nil {
		logMessage("SYSTEM", "FATAL", "Invalid UDP address: %v", err)
		os.Exit(1)
	}

	udpConn, err = net.DialUDP("udp", nil, serverAddr)
	if err != nil {
		logMessage("SYSTEM", "FATAL", "Failed to establish UDP connection: %v", err)
		os.Exit(1)
	}
	defer udpConn.Close()

	logMessage("SYSTEM", "INFO", "UDP connection established for status updates (target: %s)", serverAddr.String())

	// Connection retry loop
	attempt := 1
	maxAttempt := 100

	logMessage("SYSTEM", "INFO", "Initiating connection to central server...")

	for attempt <= maxAttempt {
		logMessage("CONNECTION", "INFO", "Connecting to server (attempt %d/%d)...", attempt, maxAttempt)

		conn, err := net.Dial("tcp", "localhost:8080")
		if err != nil {
			logMessage("CONNECTION", "ERROR", "TCP connection failed: %v (will retry in 5s)", err)
			time.Sleep(5 * time.Second)
			attempt++
			continue
		}

		logMessage("CONNECTION", "INFO", "Successfully connected to central server at %s", conn.RemoteAddr().String())
		attempt = 1 

		// Process connection until it's closed or drops
		handleConnection(conn, r)

		logMessage("CONNECTION", "INFO", "Connection lost or closed, attempting to reconnect in 2s...")
		time.Sleep(2 * time.Second)
	}

	logMessage("SYSTEM", "FATAL", "Maximum reconnection attempts reached. Exiting.")
}
