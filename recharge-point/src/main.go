package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net"
	"sync"
	"time"
)

var (
	reservationQueue []string
	mu               sync.Mutex
	udpConn          *net.UDPConn
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
	log.Printf("[FILA] Novo veículo na fila: %s (Tamanho: %d)", vehicleID, len(reservationQueue))
}

func removeFromQueue() string {
	mu.Lock()
	defer mu.Unlock()
	if len(reservationQueue) == 0 {
		return ""
	}
	vehicleID := reservationQueue[0]
	reservationQueue = reservationQueue[1:]
	log.Printf("[FILA] Veículo %s removido. Fila atual: %d", vehicleID, len(reservationQueue))
	return vehicleID
}

func sendStatusUpdate(cp ChargingPoint) {
	data, err := json.Marshal(cp)
	if err != nil {
		log.Printf("[ERRO] Falha ao serializar status: %v", err)
		return
	}
	_, err = udpConn.Write(data)
	if err != nil {
		log.Printf("[ERRO] Falha ao enviar status via UDP: %v", err)
		return
	}
	log.Printf("[STATUS] Enviado - Disponível: %v, Veículo: %s, Bateria: %d%%, Fila: %d",
		cp.Available, cp.VehicleID, cp.Battery, cp.Queue)
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
		log.Printf("[ERRO] Falha ao serializar custo: %v", err)
		return
	}
	_, err = conn.Write(data)
	if err != nil {
		log.Printf("[ERRO] Falha ao enviar custo: %v", err)
		return
	}
	log.Printf("[PAGAMENTO] Custo final para %s: R$%.2f", vehicleID, cost)
}

func handleConnection(conn net.Conn, r *rand.Rand) {
	defer conn.Close()
	log.Printf("[CONEXÃO] Nova conexão com %s", conn.RemoteAddr())

	lat, lon := generateRandomCoordinates(r)
	cp := ChargingPoint{
		ID:        generateRandomID(r),
		Latitude:  lat,
		Longitude: lon,
		Available: true,
		Queue:     len(reservationQueue),
	}

	log.Printf("[PONTO] Novo ponto iniciado - ID: %s, Localização: (%.4f, %.4f)", cp.ID, cp.Latitude, cp.Longitude)

	data, err := json.Marshal(cp)
	if err != nil {
		log.Printf("[ERRO] Falha ao serializar ponto: %v", err)
		return
	}
	_, err = conn.Write(data)
	if err != nil {
		log.Printf("[ERRO] Falha ao enviar dados do ponto: %v", err)
		return
	}
	sendStatusUpdate(cp)

	buffer := make([]byte, 1024)
	for {
		n, err := conn.Read(buffer)
		if err != nil {
			if err.Error() == "EOF" {
				log.Println("[CONEXÃO] Conexão encerrada pelo servidor")
			} else {
				log.Printf("[ERRO] Falha na leitura: %v", err)
			}
			return
		}

		log.Printf("[MENSAGEM] Recebida: %s", string(buffer[:n]))

		var msg Message
		if err := json.Unmarshal(buffer[:n], &msg); err != nil {
			log.Printf("[ERRO] Falha ao decodificar mensagem: %v", err)
			continue
		}

		mu.Lock()
		switch msg.Type {
		case "reserva":
			var reserve ReservationRequest
			if err := json.Unmarshal(msg.Data, &reserve); err != nil {
				log.Printf("[ERRO] Reserva inválida: %v", err)
				mu.Unlock()
				continue
			}
			if isVehicleInQueue(reserve.VehicleID) {
				log.Printf("[RESERVA] Veículo %s já está na fila", reserve.VehicleID)
				response := Message{
					Type: "reserva_response",
					Data: json.RawMessage(`{"status":"error","message":"Vehicle already in queue"}`),
				}
				data, _ := json.Marshal(response)
				conn.Write(data)
			} else if len(reservationQueue) >= 5 {
				log.Printf("[RESERVA] Fila cheia (5 veículos)")
				cp.Available = false
				cp.Queue = len(reservationQueue)
				sendStatusUpdate(cp)
				response := Message{
					Type: "reserva_response",
					Data: json.RawMessage(`{"status":"error","message":"Queue full"}`),
				}
				data, _ := json.Marshal(response)
				conn.Write(data)
			} else {
				addReserve(reserve.VehicleID)
				cp.Queue = len(reservationQueue)
				log.Printf("[RESERVA] Reserva confirmada para %s", reserve.VehicleID)
				sendStatusUpdate(cp)
				response := Message{
					Type: "reserva_response",
					Data: json.RawMessage(`{"status":"success","message":"Reserve confirmed"}`),
				}
				data, _ := json.Marshal(response)
				conn.Write(data)
			}

		case "chegada":
			var chegada struct {
				IDVeiculo         string  `json:"id_veiculo"`
				NivelBateria      float64 `json:"nivel_bateria"`
				NivelCarregamento float64 `json:"nivel_carregar"`
			}
			if err := json.Unmarshal(msg.Data, &chegada); err != nil {
				log.Printf("[ERRO] Chegada inválida: %v", err)
				mu.Unlock()
				continue
			}
			if len(reservationQueue) > 0 && chegada.IDVeiculo == reservationQueue[0] {
				cp.VehicleID = chegada.IDVeiculo
				cp.Battery = int(chegada.NivelBateria)
				batteryToCharge := int(chegada.NivelCarregamento) - int(chegada.NivelBateria)
				log.Printf("[CARREGAMENTO] Iniciado para %s - Bateria: %d%%, Carregar: %d%%",
					cp.VehicleID, cp.Battery, batteryToCharge)
				go simulateCharging(conn, &cp, batteryToCharge)
			}

		case "encerramento":
			var encerramento struct {
				IDVeiculo        string  `json:"id_veiculo"`
				EnergiaConsumida float64 `json:"energia_consumida"`
			}
			if err := json.Unmarshal(msg.Data, &encerramento); err != nil {
				log.Printf("[ERRO] Encerramento inválido: %v", err)
				mu.Unlock()
				continue
			}
			if cp.VehicleID == encerramento.IDVeiculo {
				batteryCharged := int(encerramento.EnergiaConsumida)
				cp.Available = true
				cp.VehicleID = ""
				cp.Battery = 0
				removeFromQueue()
				cp.Queue = len(reservationQueue)
				log.Printf("[CARREGAMENTO] Concluído para %s - Energia: %d%%",
					encerramento.IDVeiculo, batteryCharged)
				sendStatusUpdate(cp)
				sendFinalCost(conn, encerramento.IDVeiculo, batteryCharged)
			}
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

	log.Printf("[CARREGAMENTO] Iniciado para %s - Alvo: %d%% (+%d%%)",
		vehicleID, targetBattery, batteryToCharge)

	for {
		mu.Lock()
		if cp.Battery >= targetBattery || cp.VehicleID != vehicleID {
			mu.Unlock()
			break
		}
		cp.Battery += chargingRate
		log.Printf("[CARREGAMENTO] %s: %d%%", vehicleID, cp.Battery)
		sendStatusUpdate(*cp)
		mu.Unlock()
		time.Sleep(1 * time.Second)
	}

	mu.Lock()
	batteryCharged := cp.Battery - initialBattery
	cp.Available = true
	cp.VehicleID = ""
	cp.Battery = 0
	removeFromQueue()
	cp.Queue = len(reservationQueue)
	log.Printf("[CARREGAMENTO] Concluído para %s - Total carregado: %d%%",
		vehicleID, batteryCharged)
	sendStatusUpdate(*cp)
	sendFinalCost(conn, vehicleID, batteryCharged)
	mu.Unlock()

	encerramento := Message{
		Type: "encerramento",
		Data: json.RawMessage(fmt.Sprintf(`{"id_veiculo":"%s","energia_consumida":%d}`, vehicleID, batteryCharged)),
	}
	data, err := json.Marshal(encerramento)
	if err != nil {
		log.Printf("[ERRO] Falha ao serializar encerramento: %v", err)
		return
	}
	conn.Write(data)
}

func isVehicleInQueue(vehicleID string) bool {
	mu.Lock()
	defer mu.Unlock()
	for _, v := range reservationQueue {
		if v == vehicleID {
			return true
		}
	}
	return false
}

func main() {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	serverAddr, err := net.ResolveUDPAddr("udp", "localhost:8082")
	if err != nil {
		log.Fatalf("[ERRO] Endereço UDP inválido: %v", err)
	}

	udpConn, err = net.DialUDP("udp", nil, serverAddr)
	if err != nil {
		log.Fatalf("[ERRO] Falha ao conectar UDP: %v", err)
	}
	defer udpConn.Close()

	log.Println("[PONTO] Iniciando ponto de recarga...")
	for {
		conn, err := net.Dial("tcp", "localhost:8080")
		if err != nil {
			log.Printf("[ERRO] Falha na conexão TCP: %v (reconectando em 5s...)", err)
			time.Sleep(5 * time.Second)
			continue
		}
		log.Println("[CONEXÃO] Conectado ao servidor central")
		handleConnection(conn, r)
		log.Println("[CONEXÃO] Reconectando...")
		time.Sleep(2 * time.Second)
	}
}