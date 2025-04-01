package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"sync"
	"time"
)

var (
	reservationQueue []string
	mu               sync.Mutex
	udpConn          *net.UDPConn
	logger           *log.Logger
	clientID         string
)

// ChargingPoint representa o estado atual do ponto de recarga
type ChargingPoint struct {
	ID        string  `json:"id"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Available bool    `json:"available"`
	Queue     int     `json:"queue_size"`
	VehicleID string  `json:"vehicle_id,omitempty"`
	Battery   int     `json:"battery,omitempty"`
}

// ReservationRequest representa uma solicitação de reserva
type ReservationRequest struct {
	VehicleID string `json:"vehicleid"`
}

type Message struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// logTimestamp returns current timestamp formatted for logging
func logTimestamp() string {
	return time.Now().Format("2006-01-02 15:04:05.000")
}

// logMessage formats and prints a log message with timestamp and component info
func logMessage(component, level, format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	logger.Printf("[%s] [%s] [%s] %s", logTimestamp(), component, level, message)
}

func generateRandomCoordinates(r *rand.Rand) (float64, float64) {
	minLat := -23.6500
	maxLat := -23.4500
	minLon := -46.7000
	maxLon := -46.5000
	lat := minLat + r.Float64()*(maxLat-minLat)
	lon := minLon + r.Float64()*(maxLon-minLon)
	return lat, lon
}

func generateRandomID(r *rand.Rand) string {
	const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 4)
	for i := range b {
		b[i] = letters[r.Intn(len(letters))]
	}
	return "CP" + string(b)
}

func addReserve(vehicleID string) {
	mu.Lock()
	defer mu.Unlock()
	reservationQueue = append(reservationQueue, vehicleID)
	logMessage("QUEUE", "INFO", "New vehicle added to queue: %s (Current size: %d)", vehicleID, len(reservationQueue))
}

func removeFromQueue() string {
	mu.Lock()
	defer mu.Unlock()
	if len(reservationQueue) == 0 {
		return ""
	}
	vehicleID := reservationQueue[0]
	reservationQueue = reservationQueue[1:]
	logMessage("QUEUE", "INFO", "Vehicle %s removed from queue. Current queue size: %d", vehicleID, len(reservationQueue))
	return vehicleID
}

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
	
	// Format battery status
	batteryInfo := ""
	if cp.Battery > 0 {
		batteryInfo = fmt.Sprintf(", Battery: %d%%", cp.Battery)
	}
	
	// Format vehicle info
	vehicleInfo := "none"
	if cp.VehicleID != "" {
		vehicleInfo = cp.VehicleID
	}
	
	logMessage("STATUS", "INFO", "Update sent - ID: %s, Available: %v, Vehicle: %s%s, Queue: %d", 
		cp.ID, cp.Available, vehicleInfo, batteryInfo, cp.Queue)
}

func calculateChargingCost(batteryCharged int) float64 {
	const pricePerPercent = 2.0
	return float64(batteryCharged) * pricePerPercent
}

func sendFinalCost(conn net.Conn, vehicleID string, batteryCharged int) {
	cost := calculateChargingCost(batteryCharged)
	msg := Message{
		Type: "custo_final",
		Data: json.RawMessage(fmt.Sprintf(`{"id_veiculo":"%s","custo":%.2f}`, vehicleID, cost)),
	}
	
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

func handleConnection(conn net.Conn, r *rand.Rand) {
	defer conn.Close()
	
	remoteAddr := conn.RemoteAddr().String()
	logMessage("CONNECTION", "INFO", "New connection established with server at %s", remoteAddr)

	lat, lon := generateRandomCoordinates(r)
	cp := ChargingPoint{
		ID:        generateRandomID(r),
		Latitude:  lat,
		Longitude: lon,
		Available: true,
		Queue:     len(reservationQueue),
	}
	
	clientID = cp.ID
	logMessage("INIT", "INFO", "New charging point initialized - ID: %s, Location: (%.6f, %.6f)", 
		cp.ID, cp.Latitude, cp.Longitude)

	// Send initial data to server
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
	sendStatusUpdate(cp)

	buffer := make([]byte, 1024)
	for {
		n, err := conn.Read(buffer)
		if err != nil {
			if err.Error() == "EOF" {
				logMessage("CONNECTION", "INFO", "Connection closed by server")
			} else {
				logMessage("CONNECTION", "ERROR", "Read operation failed: %v", err)
			}
			return
		}

		rawMsg := string(buffer[:n])
		logMessage("MESSAGE", "DEBUG", "Received raw data (%d bytes): %s", n, rawMsg)

		var msg Message
		if err := json.Unmarshal(buffer[:n], &msg); err != nil {
			logMessage("MESSAGE", "ERROR", "Failed to decode message: %v - Raw data: %s", err, rawMsg)
			continue
		}

		logMessage("MESSAGE", "INFO", "Received message type: %s", msg.Type)
		
		mu.Lock()
		switch msg.Type {
		case "reserva":
			var reserve ReservationRequest
			if err := json.Unmarshal(msg.Data, &reserve); err != nil {
				logMessage("RESERVATION", "ERROR", "Invalid reservation request data: %v", err)
				mu.Unlock()
				continue
			}
			
			logMessage("RESERVATION", "INFO", "Processing reservation request for vehicle %s", reserve.VehicleID)
			
			if isVehicleInQueue(reserve.VehicleID) {
				logMessage("RESERVATION", "WARN", "Vehicle %s is already in queue - position: %d", 
					reserve.VehicleID, findVehiclePosition(reserve.VehicleID))
				
				response := Message{
					Type: "reserva_response",
					Data: json.RawMessage(`{"status":"error","message":"Vehicle already in queue"}`),
				}
				data, _ := json.Marshal(response)
				conn.Write(data)
				
				logMessage("RESERVATION", "INFO", "Sent duplicate reservation error response to server")
			} else if len(reservationQueue) >= 5 {
				logMessage("RESERVATION", "WARN", "Queue is full (maximum 5 vehicles) - rejecting request from %s", 
					reserve.VehicleID)
				
				cp.Available = false
				cp.Queue = len(reservationQueue)
				sendStatusUpdate(cp)
				
				response := Message{
					Type: "reserva_response",
					Data: json.RawMessage(`{"status":"error","message":"Queue full"}`),
				}
				data, _ := json.Marshal(response)
				conn.Write(data)
				
				logMessage("RESERVATION", "INFO", "Sent queue full error response to server")
			} else {
				addReserve(reserve.VehicleID)
				cp.Queue = len(reservationQueue)
				
				logMessage("RESERVATION", "INFO", "Reservation confirmed for vehicle %s - Queue position: %d", 
					reserve.VehicleID, len(reservationQueue))
				
				sendStatusUpdate(cp)
				
				response := Message{
					Type: "reserva_response",
					Data: json.RawMessage(`{"status":"success","message":"Reserve confirmed"}`),
				}
				data, _ := json.Marshal(response)
				conn.Write(data)
				
				logMessage("RESERVATION", "INFO", "Sent successful reservation response to server")
			}

		case "chegada":
			var chegada struct {
				IDVeiculo         string  `json:"id_veiculo"`
				NivelBateria      float64 `json:"nivel_bateria"`
				NivelCarregamento float64 `json:"nivel_carregar"`
			}
			
			if err := json.Unmarshal(msg.Data, &chegada); err != nil {
				logMessage("ARRIVAL", "ERROR", "Invalid arrival data: %v - Raw data: %s", 
					err, string(msg.Data))
				mu.Unlock()
				continue
			}
			
			logMessage("ARRIVAL", "INFO", "Vehicle arrival notification - ID: %s, Current Battery: %.1f%%, Target: %.1f%%", 
				chegada.IDVeiculo, chegada.NivelBateria, chegada.NivelCarregamento)
			
			if len(reservationQueue) == 0 {
				logMessage("ARRIVAL", "WARN", "Vehicle %s arrived but queue is empty", chegada.IDVeiculo)
				mu.Unlock()
				continue
			}
			
			if chegada.IDVeiculo != reservationQueue[0] {
				logMessage("ARRIVAL", "WARN", "Vehicle %s arrived but is not next in queue (expected: %s)", 
					chegada.IDVeiculo, reservationQueue[0])
				mu.Unlock()
				continue
			}
			
			cp.VehicleID = chegada.IDVeiculo
			cp.Battery = int(chegada.NivelBateria)
			batteryToCharge := int(chegada.NivelCarregamento) - int(chegada.NivelBateria)
			
			logMessage("CHARGING", "INFO", "Starting charging session for vehicle %s - Current: %d%%, Target: %.0f%% (Increase: %d%%)",
				cp.VehicleID, cp.Battery, chegada.NivelCarregamento, batteryToCharge)
			
			go simulateCharging(conn, &cp, batteryToCharge)

		case "encerramento":
			var encerramento struct {
				IDVeiculo        string  `json:"id_veiculo"`
				EnergiaConsumida float64 `json:"energia_consumida"`
			}
			
			if err := json.Unmarshal(msg.Data, &encerramento); err != nil {
				logMessage("SESSION_END", "ERROR", "Invalid session end data: %v", err)
				mu.Unlock()
				continue
			}
			
			logMessage("SESSION_END", "INFO", "Processing session end for vehicle %s - Energy consumed: %.1f%%", 
				encerramento.IDVeiculo, encerramento.EnergiaConsumida)
			
			if cp.VehicleID == encerramento.IDVeiculo {
				batteryCharged := int(encerramento.EnergiaConsumida)
				cp.Available = true
				cp.VehicleID = ""
				cp.Battery = 0
				removedVehicle := removeFromQueue()
				cp.Queue = len(reservationQueue)
				
				logMessage("SESSION_END", "INFO", "Charging session completed for vehicle %s - Total energy: %d%%, Removed from queue: %s",
					encerramento.IDVeiculo, batteryCharged, removedVehicle)
				
				sendStatusUpdate(cp)
				sendFinalCost(conn, encerramento.IDVeiculo, batteryCharged)
			} else {
				logMessage("SESSION_END", "WARN", "Session end received for %s but current vehicle is %s", 
					encerramento.IDVeiculo, cp.VehicleID)
			}
		default:
			logMessage("MESSAGE", "WARN", "Unknown message type received: %s", msg.Type)
		}
		mu.Unlock()
	}
}

func simulateCharging(conn net.Conn, cp *ChargingPoint, batteryToCharge int) {
	mu.Lock()
	chargingRate := 1
	vehicleID := cp.VehicleID
	initialBattery := cp.Battery
	targetBattery := initialBattery + batteryToCharge
	mu.Unlock()

	logMessage("CHARGING", "INFO", "Charging simulation started for vehicle %s - Initial: %d%%, Target: %d%% (Delta: %d%%)",
		vehicleID, initialBattery, targetBattery, batteryToCharge)

	for {
		mu.Lock()
		if cp.Battery >= targetBattery || cp.VehicleID != vehicleID {
			if cp.VehicleID != vehicleID {
				logMessage("CHARGING", "WARN", "Charging interrupted - Vehicle changed from %s to %s", 
					vehicleID, cp.VehicleID)
			} else {
				logMessage("CHARGING", "INFO", "Target battery level reached for vehicle %s: %d%%", 
					vehicleID, cp.Battery)
			}
			mu.Unlock()
			break
		}
		
		cp.Battery += chargingRate
		progress := float64(cp.Battery-initialBattery) / float64(batteryToCharge) * 100
		
		logMessage("CHARGING", "INFO", "Vehicle %s charging: %d%% (Progress: %.1f%%)", 
			vehicleID, cp.Battery, progress)
		
		sendStatusUpdate(*cp)
		mu.Unlock()
		time.Sleep(1 * time.Second)
	}

	mu.Lock()
	batteryCharged := cp.Battery - initialBattery
	cp.Available = true
	cp.VehicleID = ""
	cp.Battery = 0
	removedVehicle := removeFromQueue()
	cp.Queue = len(reservationQueue)
	
	logMessage("CHARGING", "INFO", "Charging completed for vehicle %s - Total charged: %d%%, Vehicle removed: %s",
		vehicleID, batteryCharged, removedVehicle)
	
	sendStatusUpdate(*cp)
	sendFinalCost(conn, vehicleID, batteryCharged)
	mu.Unlock()

	encerramento := Message{
		Type: "encerramento",
		Data: json.RawMessage(fmt.Sprintf(`{"id_veiculo":"%s","energia_consumida":%d}`, vehicleID, batteryCharged)),
	}
	
	data, err := json.Marshal(encerramento)
	if err != nil {
		logMessage("SESSION_END", "ERROR", "Failed to serialize session end message: %v", err)
		return
	}
	
	_, err = conn.Write(data)
	if err != nil {
		logMessage("SESSION_END", "ERROR", "Failed to send session end message: %v", err)
		return
	}
	
	logMessage("SESSION_END", "INFO", "Session end notification sent to server for vehicle %s", vehicleID)
}

func isVehicleInQueue(vehicleID string) bool {
	for _, v := range reservationQueue {
		if v == vehicleID {
			return true
		}
	}
	return false
}

func findVehiclePosition(vehicleID string) int {
	for i, v := range reservationQueue {
		if v == vehicleID {
			return i + 1
		}
	}
	return -1
}

func main() {
	// Set up logger with custom format
	logger = log.New(os.Stdout, "", 0)
	
	// Create a random seed based on current time
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	
	// Log startup information
	logMessage("SYSTEM", "INFO", "Charging point client starting up...")

	// Create UDP connection for status updates
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

	// Main connection loop - keeps trying to connect to server
	attempt := 1
	maxAttempt := 100 // Prevent infinite attempts
	
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
		attempt = 1 // Reset attempt counter after successful connection
		
		// Handle the connection
		handleConnection(conn, r)
		
		logMessage("CONNECTION", "INFO", "Connection lost, attempting to reconnect in 2s...")
		time.Sleep(2 * time.Second)
	}
	
	logMessage("SYSTEM", "FATAL", "Maximum reconnection attempts reached. Exiting.")
}
