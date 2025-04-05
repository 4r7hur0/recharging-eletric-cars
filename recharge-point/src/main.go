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
	reservationQueue = append(reservationQueue, vehicleID)
	logMessage("QUEUE", "INFO", "New vehicle added to queue: %s (Current size: %d)", vehicleID, len(reservationQueue))
}

func removeFromQueue() string {
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

func handleConnection(conn net.Conn, r *rand.Rand) {
	defer conn.Close()

	remoteAddr := conn.RemoteAddr().String()
	logMessage("CONNECTION", "INFO", "New connection established with server at %s", remoteAddr)

	// --- Geração e envio do estado inicial (sem mutex ainda necessário) ---
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

	clientID = cp.ID // ok se clientID for apenas informativo para logs deste client
	logMessage("INIT", "INFO", "New charging point initialized - ID: %s, Location: (%.6f, %.6f)",
		cp.ID, cp.Latitude, cp.Longitude)

	// Send initial data to server
	data, err := json.Marshal(cp)
	if err != nil {
		logMessage("INIT", "ERROR", "Failed to serialize charging point data: %v", err)
		return // Sai se não puder enviar estado inicial
	}
	_, err = conn.Write(data)
	if err != nil {
		logMessage("INIT", "ERROR", "Failed to send initial data to server: %v", err)
		return // Sai se não puder enviar estado inicial
	}
	logMessage("INIT", "INFO", "Initial data sent to server successfully")
	// Envia o status inicial via UDP
	// (Considerar pegar o tamanho da fila global aqui se necessário, com lock)
	mu.Lock()
	cp.Queue = len(reservationQueue) // Atualiza com o tamanho global antes de enviar
	mu.Unlock()
	sendStatusUpdate(cp) // Envia estado inicial (com tamanho da fila atualizado)

	// --- Loop de Leitura de Mensagens ---
	buffer := make([]byte, 1024)
	for {
		// --- É ALTAMENTE RECOMENDADO ADICIONAR UM TIMEOUT DE LEITURA AQUI ---
		// conn.SetReadDeadline(time.Now().Add(60 * time.Second)) // Exemplo: 60 segundos

		n, err := conn.Read(buffer)
		if err != nil {
			// Tratamento de erro de leitura (ocorre fora da seção crítica do mutex)
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				logMessage("CONNECTION", "WARN", "Read timeout. Closing connection.")
				// A função handleConnection retornará, e o loop main tentará reconectar.
			} else if err == io.EOF { // Usar io.EOF é mais idiomático
				logMessage("CONNECTION", "INFO", "Connection closed by server (EOF)")
			} else {
				logMessage("CONNECTION", "ERROR", "Read operation failed: %v", err)
			}
			return // Sai de handleConnection em caso de erro de leitura/EOF/Timeout
		}

		rawMsg := string(buffer[:n])
		logMessage("MESSAGE", "DEBUG", "Received raw data (%d bytes): %s", n, rawMsg)

		var msg Message
		// Unmarshal da mensagem wrapper (ocorre fora da seção crítica)
		if err := json.Unmarshal(buffer[:n], &msg); err != nil {
			logMessage("MESSAGE", "ERROR", "Failed to decode message wrapper: %v - Raw data: %s", err, rawMsg)
			// Pula para a próxima leitura se o wrapper JSON estiver inválido
			continue // Este continue está OK, pois é ANTES do mu.Lock()
		}

		logMessage("MESSAGE", "INFO", "Received message type: %s", msg.Type)

		// --- SEÇÃO CRÍTICA: Adquire o Lock ANTES de acessar/modificar estado compartilhado ---
		mu.Lock()

		// Atualiza o tamanho da fila em cp ANTES de processar a mensagem,
		// pois as ações podem depender do tamanho atual.
		cp.Queue = len(reservationQueue)

		switch msg.Type {
		case "reserva":
			var reserve ReservationRequest
			if err := json.Unmarshal(msg.Data, &reserve); err != nil {
				logMessage("RESERVATION", "ERROR", "Invalid reservation request data: %v", err)
				// Enviar resposta de erro... (conn.Write é seguro para concorrência)
				response := Message{Type: "reserva_response", Data: json.RawMessage(fmt.Sprintf(`{"status":"error","message":"Invalid request","vehicle_id":"%s"}`, reserve.VehicleID))}
				responseData, _ := json.Marshal(response)
				conn.Write(responseData)
				// NENHUM continue AQUI! Deixa sair do if/switch para liberar o lock.
			} else {
				logMessage("RESERVATION", "INFO", "Processing reservation request for vehicle %s", reserve.VehicleID)
				if isVehicleInQueue(reserve.VehicleID) {
					logMessage("RESERVATION", "WARN", "Vehicle %s is already in queue - position: %d",
						reserve.VehicleID, findVehiclePosition(reserve.VehicleID))
					// Enviar resposta de erro...
					response := Message{Type: "reserva_response", Data: json.RawMessage(fmt.Sprintf(`{"status":"error","message":"Vehicle already in queue","vehicle_id":"%s"}`, reserve.VehicleID))}
					responseData, _ := json.Marshal(response)
					conn.Write(responseData)
					// NENHUM continue AQUI!
				} else if len(reservationQueue) >= 5 {
					logMessage("RESERVATION", "WARN", "Queue is full (maximum 5 vehicles) - rejecting request from %s", reserve.VehicleID)
					cp.Available = false // Atualiza estado local
					// cp.Queue já foi atualizado antes do switch
					sendStatusUpdate(cp) // Envia status UDP (dentro do lock - ok, mas veja nota)
					// Enviar resposta de erro...
					response := Message{Type: "reserva_response", Data: json.RawMessage(fmt.Sprintf(`{"status":"error","message":"Queue full","vehicle_id":"%s"}`, reserve.VehicleID))}
					responseData, _ := json.Marshal(response)
					conn.Write(responseData)
					// NENHUM continue AQUI!
				} else {
					addReserve(reserve.VehicleID)    // Modifica estado compartilhado
					cp.Queue = len(reservationQueue) // Atualiza estado local
					logMessage("RESERVATION", "INFO", "Reservation confirmed for vehicle %s - Queue position: %d",
						reserve.VehicleID, len(reservationQueue))
					sendStatusUpdate(cp) // Envia status UDP
					// Enviar resposta de sucesso...
					response := Message{Type: "reserva_response", Data: json.RawMessage(fmt.Sprintf(`{"status":"success","message":"Reserve confirmed","vehicle_id":"%s"}`, reserve.VehicleID))}
					responseData, _ := json.Marshal(response)
					conn.Write(responseData)
					logMessage("RESERVATION", "INFO", "Sent successful reservation response to server")
					// NENHUM continue AQUI!
				}
			}

		case "chegada":
			var chegada struct {
				IDVeiculo         string  `json:"id_veiculo"`
				NivelBateria      float64 `json:"nivel_bateria"`
				NivelCarregamento float64 `json:"nivel_carregar"`
			}
			if err := json.Unmarshal(msg.Data, &chegada); err != nil {
				logMessage("ARRIVAL", "ERROR", "Invalid arrival data: %v - Raw data: %s", err, string(msg.Data))
				// NENHUM continue AQUI!
			} else {
				logMessage("ARRIVAL", "INFO", "Vehicle arrival notification - ID: %s, Current Battery: %.1f%%, Target: %.1f%%",
					chegada.IDVeiculo, chegada.NivelBateria, chegada.NivelCarregamento)

				if len(reservationQueue) == 0 {
					logMessage("ARRIVAL", "WARN", "Vehicle %s arrived but queue is empty", chegada.IDVeiculo)
					// NENHUM continue AQUI!
				} else if chegada.IDVeiculo != reservationQueue[0] {
					logMessage("ARRIVAL", "WARN", "Vehicle %s arrived but is not next in queue (expected: %s)",
						chegada.IDVeiculo, reservationQueue[0])
					// NENHUM continue AQUI!
				} else {
					// Veículo correto chegou
					cp.VehicleID = chegada.IDVeiculo
					cp.Battery = int(chegada.NivelBateria)
					cp.Available = false // Ponto ocupado
					batteryToCharge := int(chegada.NivelCarregamento) - int(chegada.NivelBateria)
					if batteryToCharge < 0 {
						batteryToCharge = 0
					} // Evitar carga negativa

					logMessage("CHARGING", "INFO", "Starting charging session for vehicle %s - Current: %d%%, Target: %.0f%% (Increase: %d%%)",
						cp.VehicleID, cp.Battery, chegada.NivelCarregamento, batteryToCharge)

					// ATENÇÃO: Passar um ponteiro para 'cp' que é local para handleConnection.
					// Isso é OK porque handleConnection ficará bloqueada em conn.Read()
					// e não sairá do escopo enquanto simulateCharging estiver rodando (normalmente).
					// A goroutine acessará a 'cp' correta.
					go simulateCharging(conn, &cp, batteryToCharge) // Inicia goroutine de carga

					// Envia status de que o ponto está ocupado agora
					sendStatusUpdate(cp)
				}
			}

		case "encerramento": // Mensagem recebida DO SERVIDOR indicando fim (talvez redundante se o cliente já envia?)
			var encerramento struct {
				IDVeiculo        string  `json:"id_veiculo"`
				EnergiaConsumida float64 `json:"energia_consumida"` // Ou talvez só o ID seja necessário?
			}
			if err := json.Unmarshal(msg.Data, &encerramento); err != nil {
				logMessage("SESSION_END", "ERROR", "Invalid session end data from server: %v", err)
				// NENHUM continue AQUI!
			} else {
				logMessage("SESSION_END", "INFO", "Processing session end from server for vehicle %s", encerramento.IDVeiculo)

				// Verifica se o veículo a encerrar é o que está carregando
				// (Pode ser que simulateCharging já tenha terminado e limpado cp.VehicleID)
				if cp.VehicleID == encerramento.IDVeiculo {
					// Este caso é estranho. Se simulateCharging controla o fim,
					// por que o servidor mandaria um encerramento também?
					// Talvez seja um comando para forçar o fim?
					// Se for forçar, precisaria de uma forma de sinalizar simulateCharging para parar.
					// Por ora, apenas logamos. Se simulateCharging já limpou, este if falhará.
					logMessage("SESSION_END", "WARN", "Server sent end for vehicle %s which is currently charging. (No action taken, simulateCharging handles end)", cp.VehicleID)

				} else if findVehiclePosition(encerramento.IDVeiculo) != -1 {
					// Veículo está na fila, talvez o servidor queira cancelar a reserva?
					logMessage("SESSION_END", "INFO", "Server sent end for vehicle %s which is in queue. Removing from queue.", encerramento.IDVeiculo)
					// Remove da fila (implementar função específica se necessário ou adaptar removeFromQueue)
					tempQueue := []string{}
					for _, v := range reservationQueue {
						if v != encerramento.IDVeiculo {
							tempQueue = append(tempQueue, v)
						}
					}
					reservationQueue = tempQueue
					cp.Queue = len(reservationQueue)
					sendStatusUpdate(cp) // Atualiza status com fila menor

				} else {
					logMessage("SESSION_END", "WARN", "Session end from server received for %s but vehicle not charging or in queue.",
						encerramento.IDVeiculo)
				}
			}

		default:
			logMessage("MESSAGE", "WARN", "Unknown message type received: %s", msg.Type)
		}

		// --- SEÇÃO CRÍTICA: Libera o Lock DEPOIS do switch, ANTES da próxima iteração ---
		mu.Unlock()

	} // Fim do loop for {}
}

func simulateCharging(conn net.Conn, cp *ChargingPoint, batteryToCharge int) {
	// Guarda o ID do veículo localmente para referência segura
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
	if batteryToCharge <= 0 {
		logMessage("CHARGING", "INFO", "No charging needed for vehicle %s (already at/above target).", vehicleID)
		// Mesmo sem carregar, precisamos finalizar a sessão corretamente
		batteryCharged := 0 // Nenhuma bateria carregada

		mu.Lock()
		// Limpa o estado do ponto de carregamento
		if cp.VehicleID == vehicleID { // Verifica se ainda é o mesmo veículo
			cp.Available = true
			cp.VehicleID = ""
			cp.Battery = 0                      // Ou manter o nível final? Melhor zerar.
			removedVehicle := removeFromQueue() // Remove da fila (AGORA SEM LOCK INTERNO)
			cp.Queue = len(reservationQueue)
			logMessage("CHARGING", "INFO", "Charging session finished immediately for %s (no charge needed). Removed: %s", vehicleID, removedVehicle)
			stateToSend := *cp // Copia estado para enviar fora do lock
			mu.Unlock()

			sendStatusUpdate(stateToSend)
			sendFinalCost(conn, vehicleID, batteryCharged) // Envia custo zero

			// Envia mensagem de encerramento para o servidor
			encerramentoMsg := Message{Type: "encerramento", Data: json.RawMessage(fmt.Sprintf(`{"id_veiculo":"%v","energia_consumida":%v}`, vehicleID, batteryCharged))}
			data, _ := json.Marshal(encerramentoMsg)
			_, err := conn.Write(data)
			if err != nil {
				logMessage("SESSION_END", "ERROR", "Failed to send session end message: %v", err)
			} else {
				logMessage("SESSION_END", "INFO", "Session end notification sent to server for vehicle %s", vehicleID)
			}

		} else {
			// O veículo mudou enquanto estávamos processando a carga zero? Improvável mas possível.
			logMessage("CHARGING", "WARN", "Vehicle changed during zero-charge processing for %s. Current vehicle: %s", vehicleID, cp.VehicleID)
			mu.Unlock()
		}
		return // Termina a função simulateCharging
	}

	logMessage("CHARGING", "INFO", "Charging simulation started for vehicle %s - Initial: %d%%, Target: %d%% (Delta: %d%%)",
		vehicleID, initialBattery, targetBattery, batteryToCharge)

	// Loop de carregamento
	for {
		// Pequena pausa antes de verificar/carregar
		time.Sleep(1 * time.Second)

		var stateToSend ChargingPoint // Para enviar status fora do lock
		var shouldBreak bool = false
		var logMsg string = ""
		var logLvl string = "INFO"

		mu.Lock() // Lock para verificar/atualizar estado
		// Verifica se o veículo ainda é o esperado E se a bateria não atingiu o alvo
		if cp.VehicleID != vehicleID {
			logMsg = fmt.Sprintf("Charging interrupted - Vehicle changed from %s to %s", vehicleID, cp.VehicleID)
			logLvl = "WARN"
			shouldBreak = true
		} else if cp.Battery >= targetBattery {
			logMsg = fmt.Sprintf("Target battery level reached for vehicle %s: %d%%", vehicleID, cp.Battery)
			shouldBreak = true
			// Garante que não excedemos o alvo (caso a taxa seja > 1)
			cp.Battery = targetBattery
		} else {
			// Carrega
			cp.Battery += chargingRate
			// Garante que não excedemos o alvo na etapa de incremento
			if cp.Battery > targetBattery {
				cp.Battery = targetBattery
			}
			progress := float64(cp.Battery-initialBattery) / float64(batteryToCharge) * 100
			logMsg = fmt.Sprintf("Vehicle %s charging: %d%% (Progress: %.1f%%)", vehicleID, cp.Battery, progress)
			stateToSend = *cp // Copia estado para enviar status
		}
		mu.Unlock() // Libera o lock

		// Log e envio de status fora do lock
		if logMsg != "" {
			logMessage("CHARGING", logLvl, "%s", logMsg)
		}
		if stateToSend.ID != "" { // Só envia se copiamos um estado válido
			sendStatusUpdate(stateToSend)
		}

		if shouldBreak {
			break // Sai do loop de carregamento
		}
	}

	// Finalização da carga (ocorre após o loop)
	var finalStateToSend ChargingPoint
	var batteryCharged int = 0 // Inicializa

	mu.Lock() // Lock para limpeza final
	// Verifica se o veículo ainda é o mesmo antes de finalizar
	if cp.VehicleID == vehicleID {
		batteryCharged = cp.Battery - initialBattery // Calcula carga efetiva
		if batteryCharged < 0 {
			batteryCharged = 0
		} // Sanity check

		cp.Available = true
		cp.VehicleID = ""
		cp.Battery = 0                      // Reseta bateria do ponto
		removedVehicle := removeFromQueue() // Remove da fila (AGORA SEM LOCK INTERNO)
		cp.Queue = len(reservationQueue)

		logMessage("CHARGING", "INFO", "Charging completed for vehicle %s - Total charged: %d%%, Vehicle removed: %s",
			vehicleID, batteryCharged, removedVehicle)

		finalStateToSend = *cp // Copia estado final para enviar
		mu.Unlock()            // Libera o lock ANTES de enviar I/O final

		// Envio de status e custo final (fora do lock)
		if finalStateToSend.ID != "" {
			sendStatusUpdate(finalStateToSend)
		}
		sendFinalCost(conn, vehicleID, batteryCharged)

		// Envia mensagem de encerramento para o servidor
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
		// O veículo mudou BEM NO FIM? Loga e não faz mais nada para o veículo original.
		logMessage("CHARGING", "WARN", "Charging loop ended for %s, but current vehicle is now %s. No final actions taken for %s.",
			vehicleID, cp.VehicleID, vehicleID)
		mu.Unlock() // Libera o lock
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
	serverAddr, err := net.ResolveUDPAddr("udp", "localhost:8082") // Verifique a porta do servidor UDP
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

		conn, err := net.Dial("tcp", "localhost:8080") // Verifique IP/Porta do servidor TCP
		if err != nil {
			logMessage("CONNECTION", "ERROR", "TCP connection failed: %v (will retry in 5s)", err)
			time.Sleep(5 * time.Second)
			attempt++
			continue
		}

		logMessage("CONNECTION", "INFO", "Successfully connected to central server at %s", conn.RemoteAddr().String())
		attempt = 1 // Reset attempt counter after successful connection

		// Handle the connection (esta função agora bloqueará até a conexão cair)
		handleConnection(conn, r)

		// Se handleConnection retornar, a conexão foi perdida ou fechada.
		logMessage("CONNECTION", "INFO", "Connection lost or closed, attempting to reconnect in 2s...")
		time.Sleep(2 * time.Second)
		// O loop 'for' continuará para a próxima tentativa de conexão.
	}

	logMessage("SYSTEM", "FATAL", "Maximum reconnection attempts reached. Exiting.")
}
