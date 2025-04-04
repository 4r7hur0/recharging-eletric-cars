package main

import (
	"context" // Import context package
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
	mu               sync.Mutex // Protects reservationQueue and CP state access across goroutines
	udpConn          *net.UDPConn
	logger           *log.Logger
	// clientID is now set within handleConnection and mainly used for logging context for that specific connection instance.
)

// ChargingPoint representa o estado atual do ponto de recarga
type ChargingPoint struct {
	ID        string  `json:"id"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Available bool    `json:"available"`
	Queue     int     `json:"queue_size"`
	VehicleID string  `json:"vehicle_id,omitempty"` // Vehicle currently charging/assigned
	Battery   int     `json:"battery,omitempty"`    // Current battery % of charging vehicle
}

// ReservationRequest representa uma solicitação de reserva
type ReservationRequest struct {
	VehicleID string `json:"vehicleid"`
}

// Message representa a estrutura genérica da mensagem TCP
type Message struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// --- Logging ---

func logTimestamp() string {
	return time.Now().Format("2006-01-02 15:04:05.000")
}

func logMessage(component, level, format string, args ...interface{}) {
	// Basic check to avoid nil logger if initialization failed (though main exits)
	if logger == nil {
		fmt.Printf("[%s] [%s] [%s] %s\n", logTimestamp(), component, level, fmt.Sprintf(format, args...))
		return
	}
	message := fmt.Sprintf(format, args...)
	logger.Printf("[%s] [%s] [%s] %s", logTimestamp(), component, level, message)
}

// --- Random Data Generation ---

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

// --- Queue Management (Protected by Mutex) ---

// Adds a vehicle IF the queue is not full and the vehicle isn't already there.
// Returns true if added, false otherwise. Caller must hold the mutex.
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

// Removes and returns the first vehicle ID from the queue.
// Returns an empty string if the queue is empty. Caller must hold the mutex.
func removeFirstFromQueueLocked() string {
	if len(reservationQueue) == 0 {
		return ""
	}
	vehicleID := reservationQueue[0]
	reservationQueue = reservationQueue[1:]
	logMessage("QUEUE", "INFO", "Vehicle %s removed from front of queue. New queue size: %d", vehicleID, len(reservationQueue))
	return vehicleID
}

// Checks if a vehicle is in the queue. Caller must hold the mutex.
func isVehicleInQueueLocked(vehicleID string) bool {
	for _, v := range reservationQueue {
		if v == vehicleID {
			return true
		}
	}
	return false
}

// Finds the 1-based position of a vehicle in the queue. Returns -1 if not found.
// Caller must hold the mutex.
func findVehiclePositionLocked(vehicleID string) int {
	for i, v := range reservationQueue {
		if v == vehicleID {
			return i + 1
		}
	}
	return -1
}

// --- Communication ---

// sendStatusUpdate sends the current state via UDP.
// It accepts a copy of the ChargingPoint state to avoid holding locks during I/O.
func sendStatusUpdate(cpState ChargingPoint) {
	// Add current queue size from the passed state
	// cpState.Queue = len(reservationQueue) // This was wrong, queue size should be set by caller *before* calling

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
		// Don't return fatal error for UDP
	}

	// Logging formatted status
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
	const pricePerPercent = 2.0 // Example cost
	return float64(batteryCharged) * pricePerPercent
}

// sendFinalCost sends the final cost message TO THE SERVER via TCP.
func sendFinalCost(conn net.Conn, vehicleID string, batteryCharged int) {
	cost := calculateChargingCost(batteryCharged)
	msg := Message{
		Type: "custo_final", // Server needs to handle this message type
		Data: json.RawMessage(fmt.Sprintf(`{"id_veiculo":"%s","custo":%.2f,"carga_realizada_percent":%d}`, vehicleID, cost, batteryCharged)),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		logMessage("PAYMENT", "ERROR", "Failed to serialize cost message for vehicle %s: %v", vehicleID, err)
		return // Don't proceed if serialization fails
	}

	_, err = conn.Write(data)
	if err != nil {
		// Log error, but charging is done, maybe connection dropped right at the end
		logMessage("PAYMENT", "ERROR", "Failed to send final cost to server for vehicle %s: %v", vehicleID, err)
		return
	}

	logMessage("PAYMENT", "INFO", "Final cost sent to server for vehicle %s: R$%.2f (Charged: %d%%)",
		vehicleID, cost, batteryCharged)
}

// sendTCPResponse sends a generic response message back to the server via TCP.
// Now its been only used for sending reservation confirmation
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

// --- Core Logic ---

// handleConnection manages a single TCP connection from the server.
func handleConnection(conn net.Conn, r *rand.Rand) {
	defer conn.Close()
	remoteAddr := conn.RemoteAddr().String()
	logMessage("CONNECTION", "INFO", "Handling connection from server at %s", remoteAddr)

	// Create context for this connection to manage goroutines spawned by it
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Ensure context is cancelled when handleConnection returns

	lat, lon := generateRandomCoordinates(r)
	cp := ChargingPoint{ // Use uma variável local 'cp', não a global
		ID:        generateRandomID(r),
		Latitude:  lat,
		Longitude: lon,
		Available: true,
		// A fila inicial é 0 para este ponto específico, ou você quer sincronizar com a global?
		// Se for global, precisa de lock aqui, mas geralmente cada ponto começa vazio.
		// Vamos assumir que começa vazio localmente. Se precisar do estado global, precisa de lock.
		Queue: 0, // Inicialmente 0 para este ponto
	}

	// Use the generated ID for logging context within this handler
	handlerClientID := cp.ID
	logMessage("INIT", "INFO", "Charging point instance created - ID: %s, Location: (%.6f, %.6f)",
		handlerClientID, cp.Latitude, cp.Longitude)

	// Send initial data to server
	initialData, err := json.Marshal(cp)
	if err != nil {
		logMessage("INIT", "ERROR", "[%s] Failed to serialize initial data: %v", handlerClientID, err)
		return // Cannot proceed without sending initial state
	}
	_, err = conn.Write(initialData)
	if err != nil {
		logMessage("INIT", "ERROR", "[%s] Failed to send initial data to server: %v", handlerClientID, err)
		return // Cannot proceed if initial send fails
	}
	logMessage("INIT", "INFO", "[%s] Initial data sent to server successfully", handlerClientID)

	// Send initial UDP status update
	var initialCpState ChargingPoint
	mu.Lock()
	cp.Queue = len(reservationQueue) // Update with global queue size
	initialCpState = cp              // Create a copy of the state
	mu.Unlock()
	sendStatusUpdate(initialCpState) // Send the copied state

	// --- Message Reading Loop ---
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
			// Consider sending an error response back to server? For now, just log and continue.
			continue
		}

		logMessage("MESSAGE", "INFO", "[%s] Received message type: %s", handlerClientID, msg.Type)

		// Process message - Lock only when accessing/modifying shared state
		switch msg.Type {
		case "reserva":
			var reserve ReservationRequest
			var cpStateToSend ChargingPoint // To send UDP status outside lock
			var sendStatus bool = false     // Flag to indicate if status update is needed
			var responseStatus, responseMessage string

			if err := json.Unmarshal(msg.Data, &reserve); err != nil {
				logMessage("RESERVATION", "ERROR", "[%s] Invalid reservation request data: %v", handlerClientID, err)
				responseStatus = "error"
				responseMessage = "Invalid request format"
				// Don't lock, just send error response
			} else {
				logMessage("RESERVATION", "INFO", "[%s] Processing reservation for vehicle %s", handlerClientID, reserve.VehicleID)

				mu.Lock() // --- LOCK ---
				if isVehicleInQueueLocked(reserve.VehicleID) {
					pos := findVehiclePositionLocked(reserve.VehicleID)
					logMessage("RESERVATION", "WARN", "[%s] Vehicle %s already in queue (Pos: %d)", handlerClientID, reserve.VehicleID, pos)
					responseStatus = "error"
					responseMessage = "Vehicle already in queue"
				} else if added := addReserveLocked(reserve.VehicleID); added {
					cp.Queue = len(reservationQueue) // Update local CP queue count
					logMessage("RESERVATION", "INFO", "[%s] Reservation confirmed for %s (Queue Pos: %d)", handlerClientID, reserve.VehicleID, cp.Queue)
					responseStatus = "success"
					responseMessage = "Reserve confirmed"
					cpStateToSend = cp // Copy state for UDP update
					sendStatus = true
				} else { // Not added, likely queue full
					cp.Available = false             // Mark as unavailable *if* queue is full
					cp.Queue = len(reservationQueue) // Should be 5
					logMessage("RESERVATION", "WARN", "[%s] Queue full. Rejecting %s", handlerClientID, reserve.VehicleID)
					responseStatus = "error"
					responseMessage = "Queue full"
					cpStateToSend = cp // Copy state for UDP update
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
				NivelBateria      float64 `json:"nivel_bateria"`      // e.g., 20.0
				NivelCarregamento float64 `json:"nivel_carregar"` // e.g., 80.0
			}
			var startCharging bool = false
			var vehicleToCharge string
			var initialBattery, batteryToCharge int
			var cpStateToSend ChargingPoint // For UDP update

			if err := json.Unmarshal(msg.Data, &chegada); err != nil {
				logMessage("ARRIVAL", "ERROR", "[%s] Invalid arrival data: %v - Raw data: %s", handlerClientID, err, string(msg.Data))
				// Maybe send error response?
				continue // Skip processing this message
			}

			logMessage("ARRIVAL", "INFO", "[%s] Vehicle arrival notification - ID: %s, Battery: %.1f%%, Target: %.1f%%",
				handlerClientID, chegada.IDVeiculo, chegada.NivelBateria, chegada.NivelCarregamento)

			mu.Lock() // --- LOCK ---
			if len(reservationQueue) == 0 {
				logMessage("ARRIVAL", "WARN", "[%s] Vehicle %s arrived but queue is empty.", handlerClientID, chegada.IDVeiculo)
			} else if chegada.IDVeiculo != reservationQueue[0] {
				logMessage("ARRIVAL", "WARN", "[%s] Vehicle %s arrived but is not next in queue (Expected: %s).", handlerClientID, chegada.IDVeiculo, reservationQueue[0])
			} else if !cp.Available {
				// This case should ideally not happen if logic is correct, but good to check
				logMessage("ARRIVAL", "WARN", "[%s] Vehicle %s arrived correctly, but CP state is not Available? (Current vehicle: %s)", handlerClientID, chegada.IDVeiculo, cp.VehicleID)
			} else {
				// Correct vehicle arrived, and CP is available. Start charging.
				cp.VehicleID = chegada.IDVeiculo
				cp.Available = false // Mark as busy
				cp.Battery = int(chegada.NivelBateria) // Store initial battery
				cp.Queue = len(reservationQueue)      // Update queue size (though it won't change here)

				initialBattery = cp.Battery
				targetBattery := int(chegada.NivelCarregamento)
				batteryToCharge = targetBattery - initialBattery

				vehicleToCharge = cp.VehicleID // Store locally for goroutine
				startCharging = true
				cpStateToSend = cp // Copy state for UDP update

				logMessage("CHARGING", "INFO", "[%s] Preparing charge for %s: Current=%d%%, Target=%d%%, Delta=%d%%",
					handlerClientID, vehicleToCharge, initialBattery, targetBattery, batteryToCharge)
			}
			mu.Unlock() // --- UNLOCK ---

			// Outside lock:
			if startCharging {
				sendStatusUpdate(cpStateToSend) // Send UDP status (occupied)
				// Launch charging simulation in a new goroutine, passing necessary info and context
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
        // Adjust batteryToCharge based on clamping
        batteryToCharge = targetBattery - initialBattery
        if batteryToCharge < 0 { batteryToCharge = 0 }
	}

	logMessage("CHARGING", "INFO", "[%s] Goroutine started for %s: Initial=%d%%, Target=%d%% (Charge %d%%)",
		handlerClientID, vehicleID, initialBattery, targetBattery, batteryToCharge)

	// Check if charging is actually needed
	if batteryToCharge <= 0 {
		logMessage("CHARGING", "INFO", "[%s] No charging needed for %s (already at/above target or delta is zero). Finishing session.", handlerClientID, vehicleID)
	} else {
		const chargingRate = 1       // Percent per second (adjust as needed)
		const updateInterval = 1 * time.Second

	chargeLoop:
		for currentBattery := initialBattery; currentBattery < targetBattery; {
			select {
			case <-ctx.Done(): // Check if the connection handler requested cancellation
				logMessage("CHARGING", "WARN", "[%s] Charging cancelled for %s due to context cancellation (connection likely closed).", handlerClientID, vehicleID)
				// Cannot reliably send final messages if context is cancelled. Exit goroutine.
				// State remains locked by the vehicle until potentially next connection clears it? Or should we try to unlock?
				// Safest is probably to just exit. The next connection *should* reset state.
				return // Exit goroutine

			case <-time.After(updateInterval):
				// Proceed with charging simulation
			}

			// --- LOCK FOR STATE UPDATE ---
			mu.Lock()
			// Double-check if the assigned vehicle is still the one we are charging
			if cp.VehicleID != vehicleID {
				logMessage("CHARGING", "WARN", "[%s] Charging aborted for %s. Vehicle assignment changed to %s.", handlerClientID, vehicleID, cp.VehicleID)
				mu.Unlock() // --- UNLOCK ---
				// Don't proceed with finalization for the wrong vehicle
				return // Exit goroutine
			}

			currentBattery += chargingRate
			if currentBattery > targetBattery {
				currentBattery = targetBattery // Don't overshoot
			}
			cp.Battery = currentBattery // Update shared state

			progress := 0.0
			if batteryToCharge > 0 {
				progress = float64(currentBattery-initialBattery) / float64(batteryToCharge) * 100
			}

			logMessage("CHARGING", "DEBUG", "[%s] Vehicle %s charging: %d%% (Progress: %.1f%%)", handlerClientID, vehicleID, cp.Battery, progress)
			cpStateSnapshot := *cp // Create snapshot for UDP update
			mu.Unlock()
			// --- UNLOCK ---

			// Send UDP status update (outside lock)
			sendStatusUpdate(cpStateSnapshot)

			if currentBattery >= targetBattery {
				break chargeLoop // Exit loop once target is reached
			}
		} // --- End Charging Loop ---
	}

	// --- Finalization (runs after loop or if batteryToCharge was 0) ---
	logMessage("CHARGING", "INFO", "[%s] Charging finished for %s. Finalizing session.", handlerClientID, vehicleID)

	var finalCpState ChargingPoint
	var actualChargedAmount int = 0 // Calculate actual charge provided

	mu.Lock() // --- LOCK ---
	// Verify again: Are we still the assigned vehicle?
	if cp.VehicleID == vehicleID {
        // Calculate actual amount charged based on final state (in case rate wasn't exactly 1 or loop exited early)
        actualChargedAmount = cp.Battery - initialBattery
        if actualChargedAmount < 0 { actualChargedAmount = 0 } // Sanity check

		logMessage("CHARGING", "INFO", "[%s] Finalizing state for %s. Charged %d%%.", handlerClientID, vehicleID, actualChargedAmount)

		cp.Available = true
		cp.VehicleID = ""
		cp.Battery = 0 // Reset CP battery display

		removed := removeFirstFromQueueLocked() // Remove vehicle from shared queue
		if removed != vehicleID {
			// This indicates a potential logic error elsewhere if the wrong vehicle was removed
			logMessage("QUEUE", "ERROR", "[%s] !!! Mismatch: Expected to remove %s from queue but removed %s !!!", handlerClientID, vehicleID, removed)
		}
		cp.Queue = len(reservationQueue) // Update local CP queue count

		finalCpState = *cp // Copy final state for UDP
		mu.Unlock() // --- UNLOCK ---

		// --- Post-Finalization I/O (outside lock) ---

		// 1. Send final UDP status (now available)
		sendStatusUpdate(finalCpState)

		// 2. Send final cost message TO SERVER via TCP
		// Check context *before* writing to potentially closed connection
		if ctx.Err() == nil {
			sendFinalCost(conn, vehicleID, actualChargedAmount)
		} else {
			logMessage("PAYMENT", "WARN", "[%s] Cannot send final cost for %s. Context cancelled.", handlerClientID, vehicleID)
		}

		// 3. Send session end notification TO SERVER via TCP
		if ctx.Err() == nil {
			encerramentoMsg := Message{
				Type: "encerramento", // CP notifies SERVER that session ended
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

// --- Main Function ---

func main() {
	      
	// Set up logger (now only writing to standard output)
	logger = log.New(os.Stdout, "", 0) // Configure logger to write directly to os.Stdout

    
	// Seed random number generator
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	logMessage("SYSTEM", "INFO", "Charging point client starting up...")

	// --- Setup UDP Connection ---
	// Configuration (replace hardcoded values if needed)
	udpServerAddr := "localhost:8082"

	serverAddr, err := net.ResolveUDPAddr("udp", udpServerAddr)
	if err != nil {
		logMessage("SYSTEM", "FATAL", "Invalid UDP server address '%s': %v", udpServerAddr, err)
		os.Exit(1)
	}
	// Use DialUDP for a "connected" UDP socket (simplifies Write calls)
	udpConn, err = net.DialUDP("udp", nil, serverAddr)
	if err != nil {
		logMessage("SYSTEM", "FATAL", "Failed to establish UDP connection to %s: %v", serverAddr.String(), err)
		os.Exit(1)
	}
	defer udpConn.Close()
	logMessage("SYSTEM", "INFO", "UDP connection configured for status updates (Target: %s)", serverAddr.String())

	// --- Main TCP Connection Loop ---
	// Configuration (replace hardcoded values if needed)
	tcpServerAddr := "localhost:8080"
	reconnectDelay := 5 * time.Second
	maxReconnectAttempts := 10 // Limit reconnection attempts

	logMessage("SYSTEM", "INFO", "Initiating connection to central server at %s...", tcpServerAddr)

	for attempt := 1; attempt <= maxReconnectAttempts; attempt++ {
		logMessage("CONNECTION", "INFO", "Attempting TCP connection to %s (Attempt %d/%d)...", tcpServerAddr, attempt, maxReconnectAttempts)

		conn, err := net.DialTimeout("tcp", tcpServerAddr, 10*time.Second) // Add timeout to dial
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

		// Handle the connection (this function blocks until the connection is closed/errors out)
		handleConnection(conn, r)

		// If handleConnection returns, the connection was lost or closed.
		logMessage("CONNECTION", "INFO", "Connection closed or lost. Preparing to reconnect...")
		// The loop will continue after a brief pause if max attempts not reached.
		if attempt < maxReconnectAttempts {
			time.Sleep(2 * time.Second) // Brief pause before next automatic attempt
		} else {
             logMessage("SYSTEM", "FATAL", "Connection lost after reaching max reconnection attempts cycle. Exiting.")
             os.Exit(1)
        }
	}
}