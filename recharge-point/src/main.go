package main

import (
	"context" 
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

var (
	reservationQueue []string
	mu               sync.Mutex 
	udpConn          *net.UDPConn
	logger           *log.Logger
)

type ChargingPoint struct {
	ID        string  `json:"id"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Available bool    `json:"available"`
	Queue     int     `json:"queue_size"`
	VehicleID string  `json:"vehicle_id,omitempty"` 
	Battery   int     `json:"battery,omitempty"`   
}

type ReservationRequest struct {
	VehicleID string `json:"vehicleid"`
}

type Message struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

func logTimestamp() string {
	return time.Now().Format("2006-01-02 15:04:05.000")
}

// Manage the logs
func logMessage(component, level, format string, args ...interface{}) {
	if logger == nil {
		fmt.Printf("[%s] [%s] [%s] %s\n", logTimestamp(), component, level, fmt.Sprintf(format, args...))
		return
	}
	message := fmt.Sprintf(format, args...)
	logger.Printf("[%s] [%s] [%s] %s", logTimestamp(), component, level, message)
}

// SP Coordinates
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

func addReserveLocked(vehicleID string) bool {
	if len(reservationQueue) >= 5 {
		logMessage("QUEUE", "WARN", "Queue is full (max 5). Cannot add %s.", vehicleID)
		return false
	}
	for _, v := range reservationQueue {
		if v == vehicleID {
			logMessage("QUEUE", "WARN", "Vehicle %s already in queue. Cannot add again.", vehicleID)
			return false
		}
	}
	reservationQueue = append(reservationQueue, vehicleID)
	logMessage("QUEUE", "INFO", "Vehicle added to queue: %s (New size: %d)", vehicleID, len(reservationQueue))
	return true
}

func removeFirstFromQueueLocked() string {
	if len(reservationQueue) == 0 {
		return ""
	}
	vehicleID := reservationQueue[0]
	reservationQueue = reservationQueue[1:]
	logMessage("QUEUE", "INFO", "Vehicle %s removed from front of queue. New queue size: %d", vehicleID, len(reservationQueue))
	return vehicleID
}

func isVehicleInQueueLocked(vehicleID string) bool {
	for _, v := range reservationQueue {
		if v == vehicleID {
			return true
		}
	}
	return false
}

func findVehiclePositionLocked(vehicleID string) int {
	for i, v := range reservationQueue {
		if v == vehicleID {
			return i + 1
		}
	}
	return -1
}

func sendStatusUpdate(cpState ChargingPoint) {
	data, err := json.Marshal(cpState)
	if err != nil {
		logMessage("UDP", "ERROR", "Failed to serialize status: %v", err)
		return
	}

	if udpConn == nil {
		logMessage("UDP", "ERROR", "Cannot send status update, UDP connection is nil")
		return
	}

	_, err = udpConn.Write(data)
	if err != nil {
		logMessage("UDP", "ERROR", "Failed to send status via UDP: %v", err)
	}

	// Log formatted status
	batteryInfo := ""
	if !cpState.Available && cpState.VehicleID != "" && cpState.Battery > 0 {
		batteryInfo = fmt.Sprintf(", Battery: %d%%", cpState.Battery)
	}
	vehicleInfo := "none"
	if cpState.VehicleID != "" {
		vehicleInfo = cpState.VehicleID
	}
	logMessage("STATUS", "INFO", "UDP Update Sent - ID: %s, Available: %v, Vehicle: %s%s, Queue: %d",
		cpState.ID, cpState.Available, vehicleInfo, batteryInfo, cpState.Queue)
}

func calculateChargingCost(batteryCharged int) float64 {
	const pricePerPercent = 2.0 
	return float64(batteryCharged) * pricePerPercent
}

func sendFinalCost(conn net.Conn, chargingPointId string, vehicleID string, batteryCharged int) {
	cost := calculateChargingCost(batteryCharged)
	msg := Message{
		Type: "custo_final", 
		Data: json.RawMessage(fmt.Sprintf(`{"id_ponto_recarga":"%s", "id_veiculo":"%s","custo":%.2f,"carga_realizada_percent":%d}`, chargingPointId, vehicleID, cost, batteryCharged)),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		logMessage("PAYMENT", "ERROR", "Failed to serialize cost message for vehicle %s: %v", vehicleID, err)
		return 
	}

	_, err = conn.Write(data)
	logMessage("PAYMENT", "INFO", "Final cost sent to server for vehicle %s: R$%.2f (Charged: %d%%)",
		vehicleID, cost, batteryCharged)
}

func sendTCPResponse(conn net.Conn, responseType string, status string, message string, vehicleID string) {
	responseData := fmt.Sprintf(`{"status":"%s","message":"%s","vehicle_id":"%s"}`, status, message, vehicleID)
	responseMsg := Message{
		Type: responseType,
		Data: json.RawMessage(responseData),
	}
	data, err := json.Marshal(responseMsg)
	if err != nil {
		logMessage("TCP_RESPONSE", "ERROR", "Failed to serialize %s for %s: %v", responseType, vehicleID, err)
		return
	}
	_, err = conn.Write(data)
	if err != nil {
		logMessage("TCP_RESPONSE", "ERROR", "Failed to send %s response to server for %s: %v", responseType, vehicleID, err)
	} else {
		logMessage("TCP_RESPONSE", "INFO", "Sent %s response to server for %s: %s", responseType, vehicleID, status)
	}
}


// handleConnection manages a single TCP connection from the server.
func handleConnection(conn net.Conn, r *rand.Rand) {
	defer conn.Close()
	remoteAddr := conn.RemoteAddr().String()
	logMessage("CONNECTION", "INFO", "Handling connection from server at %s", remoteAddr)

	// Create context for this connection to manage goroutines 
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() 

	lat, lon := generateRandomCoordinates(r)
	cp := ChargingPoint{ 
		ID:        generateRandomID(r),
		Latitude:  lat,
		Longitude: lon,
		Available: true,
		Queue: 0, 
	}

	handlerClientID := cp.ID
	logMessage("INIT", "INFO", "Charging point instance created - ID: %s, Location: (%.6f, %.6f)",
		handlerClientID, cp.Latitude, cp.Longitude)

	initialData, err := json.Marshal(cp)
	if err != nil {
		logMessage("INIT", "ERROR", "[%s] Failed to serialize initial data: %v", handlerClientID, err)
		return 
	}
	_, err = conn.Write(initialData)
	if err != nil {
		logMessage("INIT", "ERROR", "[%s] Failed to send initial data to server: %v", handlerClientID, err)
		return 
	}
	logMessage("INIT", "INFO", "[%s] Initial data sent to server successfully", handlerClientID)

	var initialCpState ChargingPoint
	mu.Lock()
	cp.Queue = len(reservationQueue) 
	initialCpState = cp              
	mu.Unlock()
	sendStatusUpdate(initialCpState) 

	buffer := make([]byte, 1024)
	
	for {

		n, err := conn.Read(buffer)
		if err != nil {
			if err == io.EOF {
				logMessage("CONNECTION", "INFO", "[%s] Connection closed by server (EOF).", handlerClientID)
			} else {
				logMessage("CONNECTION", "ERROR", "[%s] Read operation failed: %v", handlerClientID, err)
			}
			return
		}
		rawMsg := buffer[:n]
		logMessage("MESSAGE", "DEBUG", "[%s] Received raw data (%d bytes): %s", handlerClientID, n, string(rawMsg))

		var msg Message
		if err := json.Unmarshal(rawMsg, &msg); err != nil {
			logMessage("MESSAGE", "ERROR", "[%s] Failed to decode message wrapper: %v - Raw data: %s", handlerClientID, err, string(rawMsg))
			continue
		}

		logMessage("MESSAGE", "INFO", "[%s] Received message type: %s", handlerClientID, msg.Type)

		switch msg.Type {
		case "reserva":
			var reserve ReservationRequest
			var cpStateToSend ChargingPoint 
			var sendStatus bool = false     
			var responseStatus, responseMessage string

			if err := json.Unmarshal(msg.Data, &reserve); err != nil {
				logMessage("RESERVATION", "ERROR", "[%s] Invalid reservation request data: %v", handlerClientID, err)
				responseStatus = "error"
				responseMessage = "Invalid request format"
			} else {
				logMessage("RESERVATION", "INFO", "[%s] Processing reservation for vehicle %s", handlerClientID, reserve.VehicleID)

				mu.Lock() // --- LOCK ---
				if isVehicleInQueueLocked(reserve.VehicleID) {
					pos := findVehiclePositionLocked(reserve.VehicleID)
					logMessage("RESERVATION", "WARN", "[%s] Vehicle %s already in queue (Pos: %d)", handlerClientID, reserve.VehicleID, pos)
					responseStatus = "error"
					responseMessage = "Vehicle already in queue"
				} else if added := addReserveLocked(reserve.VehicleID); added {
					cp.Queue = len(reservationQueue) 
					logMessage("RESERVATION", "INFO", "[%s] Reservation confirmed for %s (Queue Pos: %d)", handlerClientID, reserve.VehicleID, cp.Queue)
					responseStatus = "success"
					responseMessage = "Reserve confirmed"
					cpStateToSend = cp 
					sendStatus = true
				} else { 
					cp.Available = false         
					cp.Queue = len(reservationQueue) // Should be 5
					logMessage("RESERVATION", "WARN", "[%s] Queue full. Rejecting %s", handlerClientID, reserve.VehicleID)
					responseStatus = "error"
					responseMessage = "Queue full"
					cpStateToSend = cp 
					sendStatus = true
				}
				mu.Unlock() // --- UNLOCK ---
			}

			// Send TCP response (outside lock)
			sendTCPResponse(conn, "reserva_response", responseStatus, responseMessage, reserve.VehicleID)

			// Send UDP status update if needed (outside lock)
			if sendStatus {
				sendStatusUpdate(cpStateToSend)
			}

		case "chegada":
			var chegada struct {
				IDVeiculo         string  `json:"id_veiculo"`
				NivelBateria      float64 `json:"nivel_bateria"`    
				NivelCarregamento float64 `json:"nivel_carregar"` 
			}
			var startCharging bool = false
			var vehicleToCharge string
			var initialBattery, batteryToCharge int
			var cpStateToSend ChargingPoint 

			if err := json.Unmarshal(msg.Data, &chegada); err != nil {
				logMessage("ARRIVAL", "ERROR", "[%s] Invalid arrival data: %v - Raw data: %s", handlerClientID, err, string(msg.Data))
				continue 
			}

			logMessage("ARRIVAL", "INFO", "[%s] Vehicle arrival notification - ID: %s, Battery: %.1f%%, Target: %.1f%%",
				handlerClientID, chegada.IDVeiculo, chegada.NivelBateria, chegada.NivelCarregamento)

			mu.Lock() 
			if len(reservationQueue) == 0 {
				logMessage("ARRIVAL", "WARN", "[%s] Vehicle %s arrived but queue is empty.", handlerClientID, chegada.IDVeiculo)
			} else if chegada.IDVeiculo != reservationQueue[0] {
				logMessage("ARRIVAL", "WARN", "[%s] Vehicle %s arrived but is not next in queue (Expected: %s).", handlerClientID, chegada.IDVeiculo, reservationQueue[0])
			} else {
				cp.VehicleID = chegada.IDVeiculo
				cp.Available = false 
				cp.Battery = int(chegada.NivelBateria) 
				cp.Queue = len(reservationQueue)  

				initialBattery = cp.Battery
				targetBattery := int(chegada.NivelCarregamento)
				batteryToCharge = targetBattery - initialBattery

				vehicleToCharge = cp.VehicleID 
				startCharging = true
				cpStateToSend = cp 

				logMessage("CHARGING", "INFO", "[%s] Preparing charge for %s: Current=%d%%, Target=%d%%, Delta=%d%%",
					handlerClientID, vehicleToCharge, initialBattery, targetBattery, batteryToCharge)
			}
			mu.Unlock() 

			if startCharging {
				sendStatusUpdate(cpStateToSend)
				go simulateCharging(ctx, conn, &cp, handlerClientID, vehicleToCharge, initialBattery, batteryToCharge)
			} else {
				fmt.Println("Oia deu erro")
			}

		default:
			logMessage("MESSAGE", "WARN", "[%s] Unknown message type received: %s", handlerClientID, msg.Type)
		}
	} 
}
// ctx: Context for cancellation signal from handleConnection.
// handlerClientID: The ID of the CP instance for logging.
func simulateCharging(ctx context.Context, conn net.Conn, cp *ChargingPoint, handlerClientID string, vehicleID string, initialBattery int, batteryToCharge int) {
	targetBattery := initialBattery + batteryToCharge
	
	if targetBattery > 100 {
		targetBattery = 100
        batteryToCharge = targetBattery - initialBattery
	}

	logMessage("CHARGING", "INFO", "[%s] Goroutine started for %s: Initial=%d%%, Target=%d%% (Charge %d%%)",
		handlerClientID, vehicleID, initialBattery, targetBattery, batteryToCharge)

	// Check if charging is actually needed
	if batteryToCharge <= 0 {
		logMessage("CHARGING", "INFO", "[%s] No charging needed for %s (already at/above target or delta is zero). Finishing session.", handlerClientID, vehicleID)
	} else {
		const chargingRate = 1       // Percent per second
		const updateInterval = 1 * time.Second

	chargeLoop:
		for currentBattery := initialBattery; currentBattery < targetBattery; {
			select {
			case <-ctx.Done(): // Check if the connection handler requested cancellation
				logMessage("CHARGING", "WARN", "[%s] Charging cancelled for %s due to context cancellation (connection likely closed).", handlerClientID, vehicleID)
				return // Exit goroutine

			case <-time.After(updateInterval):
				// Proceed with charging simulation
			}
			
			// Test this out after
			mu.Lock()
			// Double-check if the assigned vehicle is still the one we are charging
			if cp.VehicleID != vehicleID {
				logMessage("CHARGING", "WARN", "[%s] Charging aborted for %s. Vehicle assignment changed to %s.", handlerClientID, vehicleID, cp.VehicleID)
				mu.Unlock() 
				return // Exit goroutine
			}

			currentBattery += chargingRate
			if currentBattery > targetBattery {
				currentBattery = targetBattery 
			}
			cp.Battery = currentBattery 

			progress := 0.0
			if batteryToCharge > 0 {
				progress = float64(currentBattery-initialBattery) / float64(batteryToCharge) * 100
			}

			logMessage("CHARGING", "DEBUG", "[%s] Vehicle %s charging: %d%% (Progress: %.1f%%)", handlerClientID, vehicleID, cp.Battery, progress)
			cpStateSnapshot := *cp 
			mu.Unlock()
			sendStatusUpdate(cpStateSnapshot)

			if currentBattery >= targetBattery {
				break chargeLoop 
			}
		} 
	}

	logMessage("CHARGING", "INFO", "[%s] Charging finished for %s. Finalizing session.", handlerClientID, vehicleID)

	var finalCpState ChargingPoint
	var actualChargedAmount int = 0 

	mu.Lock() 
	if cp.VehicleID == vehicleID {

        actualChargedAmount = cp.Battery - initialBattery
		logMessage("CHARGING", "INFO", "[%s] Finalizing state for %s. Charged %d%%.", handlerClientID, vehicleID, actualChargedAmount)

		cp.Available = true
		cp.VehicleID = ""
		cp.Battery = 0 

		// Test remove this to
		removed := removeFirstFromQueueLocked() 
		if removed != vehicleID {
			logMessage("QUEUE", "ERROR", "[%s] !!! Mismatch: Expected to remove %s from queue but removed %s !!!", handlerClientID, vehicleID, removed)
		}
		cp.Queue = len(reservationQueue) 

		finalCpState = *cp 
		mu.Unlock() 

		// 1. Send final UDP status (now available)
		sendStatusUpdate(finalCpState)

		// 2. Send final cost message TO SERVER via TCP
		// Check context *before* writing to potentially closed connection
		if ctx.Err() == nil {
			sendFinalCost(conn, handlerClientID,vehicleID, actualChargedAmount)
		} else {
			logMessage("PAYMENT", "WARN", "[%s] Cannot send final cost for %s. Context cancelled.", handlerClientID, vehicleID)
		}

		// 3. Send session end notification TO SERVER via TCP
		if ctx.Err() == nil {
			encerramentoMsg := Message{
				Type: "encerramento", 
				Data: json.RawMessage(fmt.Sprintf(`{"id_veiculo":"%s","energia_consumida_percent":%d}`, vehicleID, actualChargedAmount)),
			}
			data, err := json.Marshal(encerramentoMsg)
			if err != nil {
				logMessage("SESSION_END", "ERROR", "[%s] Failed to serialize end notification for %s: %v", handlerClientID, vehicleID, err)
			} else {
				_, err = conn.Write(data)
				if err != nil {
					logMessage("SESSION_END", "ERROR", "[%s] Failed to send end notification to server for %s: %v", handlerClientID, vehicleID, err)
				} else {
					logMessage("SESSION_END", "INFO", "[%s] Session end notification sent to server for %s", handlerClientID, vehicleID)
				}
			}
		} else {
			logMessage("SESSION_END", "WARN", "[%s] Cannot send session end notification for %s. Context cancelled.", handlerClientID, vehicleID)
		}

	}
}

func main() {
	      
	logger = log.New(os.Stdout, "", 0) 
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	logMessage("SYSTEM", "INFO", "Charging point client starting up...")

	udpServerAddr := "localhost:8082"

	serverAddr, err := net.ResolveUDPAddr("udp", udpServerAddr)
	if err != nil {
		logMessage("SYSTEM", "FATAL", "Invalid UDP server address '%s': %v", udpServerAddr, err)
		os.Exit(1)
	}
	udpConn, err = net.DialUDP("udp", nil, serverAddr)
	if err != nil {
		logMessage("SYSTEM", "FATAL", "Failed to establish UDP connection to %s: %v", serverAddr.String(), err)
		os.Exit(1)
	}
	defer udpConn.Close()
	logMessage("SYSTEM", "INFO", "UDP connection configured for status updates (Target: %s)", serverAddr.String())


	tcpServerAddr := "localhost:8080"
	reconnectDelay := 5 * time.Second
	maxReconnectAttempts := 10 

	logMessage("SYSTEM", "INFO", "Initiating connection to central server at %s...", tcpServerAddr)

	for attempt := 1; attempt <= maxReconnectAttempts; attempt++ {
		logMessage("CONNECTION", "INFO", "Attempting TCP connection to %s (Attempt %d/%d)...", tcpServerAddr, attempt, maxReconnectAttempts)

		conn, err := net.DialTimeout("tcp", tcpServerAddr, 10*time.Second) 
		if err != nil {
			logMessage("CONNECTION", "ERROR", "TCP connection failed: %v. Retrying in %v...", err, reconnectDelay)
			time.Sleep(reconnectDelay)
			if attempt == maxReconnectAttempts {
				logMessage("SYSTEM", "FATAL", "Maximum reconnection attempts reached. Exiting.")
				os.Exit(1) // Exit if max attempts reached
			}
			continue // Try next attempt
		}

		logMessage("CONNECTION", "INFO", "Successfully connected to central server at %s", conn.RemoteAddr().String())
		attempt = 1 // Reset attempt counter on successful connection

		handleConnection(conn, r)

		logMessage("CONNECTION", "INFO", "Connection closed or lost. Preparing to reconnect...")
		if attempt < maxReconnectAttempts {
			time.Sleep(2 * time.Second) // Brief pause before next automatic attempt
		} else {
             logMessage("SYSTEM", "FATAL", "Connection lost after reaching max reconnection attempts cycle. Exiting.")
             os.Exit(1)
        }
	}
}
