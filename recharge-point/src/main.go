package main

import (
    "encoding/json"
    "fmt"
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

type ChargingPoint struct {
    ID        string  `json:"id"`
    Latitude  float64 `json:"latitude"`
    Longitude float64 `json:"longitude"`
    Available bool    `json:"available"`
    Queue     int     `json:"queue_size"`
    VehicleID string  `json:"vehicle_id,omitempty"` // Adicionado para rastrear veículo atual
    Battery   int     `json:"battery,omitempty"`    // Adicionado para simulação
}

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
}

func removeFromQueue() string {
    mu.Lock()
    defer mu.Unlock()
    if len(reservationQueue) == 0 {
        return ""
    }
    vehicleID := reservationQueue[0]
    reservationQueue = reservationQueue[1:]
    return vehicleID
}

func sendStatusUpdate(cp ChargingPoint) {
    data, err := json.Marshal(cp)
    if err != nil {
        fmt.Println("Error marshaling status update:", err)
        return
    }
    _, err = udpConn.Write(data)
    if err != nil {
        fmt.Println("Error sending UDP status update:", err)
        return
    }
    fmt.Printf("UDP Status update sent for %s - Available: %v, Queue: %d\n", cp.ID, cp.Available, cp.Queue)
}

func handleConnection(conn net.Conn, r *rand.Rand) {
    defer conn.Close()

    lat, lon := generateRandomCoordinates(r)
    cp := ChargingPoint{
        ID:        generateRandomID(r),
        Latitude:  lat,
        Longitude: lon,
        Available: true,
        Queue:     len(reservationQueue),
    }

    fmt.Printf("Starting charging point %s at coordinates: %.4f, %.4f\n", cp.ID, cp.Latitude, cp.Longitude)

    data, err := json.Marshal(cp)
    if err != nil {
        fmt.Println("Error marshaling initial status:", err)
        return
    }
    _, err = conn.Write(data)
    if err != nil {
        fmt.Println("Error sending initial status:", err)
        return
    }
    sendStatusUpdate(cp)

    buffer := make([]byte, 1024)
    for {
        n, err := conn.Read(buffer)
        if err != nil {
            if err.Error() == "EOF" {
                fmt.Println("Server closed connection, will reconnect...")
            } else {
                fmt.Println("Error reading from server:", err)
            }
            return
        }

        var msg Message
        if err := json.Unmarshal(buffer[:n], &msg); err != nil {
            fmt.Println("Error unmarshaling message:", err)
            continue
        }

        mu.Lock()
        switch msg.Type {
        case "reserva":
            var reserve ReservationRequest
            if err := json.Unmarshal(msg.Data, &reserve); err != nil {
                fmt.Println("Error unmarshaling reservation request:", err)
                mu.Unlock()
                continue
            }
            if isVehicleInQueue(reserve.VehicleID) {
                fmt.Printf("Vehicle %s already in queue at %s\n", reserve.VehicleID, cp.ID)
                response := Message{
                    Type: "reserva_response",
                    Data: json.RawMessage(`{"status":"error","message":"Vehicle already in queue"}`),
                }
                data, _ := json.Marshal(response)
                conn.Write(data)
            } else if len(reservationQueue) >= 5 {
                fmt.Printf("Queue full at %s, marking as unavailable\n", cp.ID)
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
                fmt.Printf("New reservation made at %s for vehicle: %s (Queue size: %d)\n", cp.ID, reserve.VehicleID, cp.Queue)
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
                fmt.Println("Error unmarshaling chegada:", err)
                mu.Unlock()
                continue
            }
            if chegada.IDVeiculo == reservationQueue[0] { // Verifica se é o próximo na fila
                cp.VehicleID = chegada.IDVeiculo
                cp.Battery = int(chegada.NivelBateria)
                fmt.Printf("Chegada confirmada para %s, bateria: %d%%\n", cp.VehicleID, cp.Battery)
                go simulateCharging(conn, &cp, int(chegada.NivelCarregamento))
            }
        case "encerramento":
            var encerramento struct {
                IDVeiculo        string `json:"id_veiculo"`
                EnergiaConsumida float64 `json:"energia_consumida"`
            }
            if err := json.Unmarshal(msg.Data, &encerramento); err != nil {
                fmt.Println("Error unmarshaling encerramento:", err)
                mu.Unlock()
                continue
            }
            if cp.VehicleID == encerramento.IDVeiculo {
                cp.Available = true
                cp.VehicleID = ""
                cp.Battery = 0
                removeFromQueue()
                cp.Queue = len(reservationQueue)
                fmt.Printf("Sessão encerrada para %s, energia consumida: %.2f kWh\n", encerramento.IDVeiculo, encerramento.EnergiaConsumida)
                sendStatusUpdate(cp)
            }
        }
        mu.Unlock()
        time.Sleep(1 * time.Second) // Reduzido para resposta mais rápida
    }
}

func simulateCharging(conn net.Conn, cp *ChargingPoint, targetBattery int) {
    mu.Lock()
    chargingRate := 1
    vehicleID := cp.VehicleID
    mu.Unlock()

    for {
        mu.Lock()
        if cp.Battery >= targetBattery || cp.VehicleID != vehicleID {
            mu.Unlock()
            break
        }
        cp.Battery += chargingRate
        fmt.Printf("Carregando %s: %d%%\n", vehicleID, cp.Battery)
        sendStatusUpdate(*cp)
        mu.Unlock()
        time.Sleep(1 * time.Second)
    }

    mu.Lock()
    energiaConsumida := float64(cp.Battery) / 100 * 50 // Exemplo: 50 kWh capacidade
    cp.Available = true
    cp.VehicleID = ""
    cp.Battery = 0
    removeFromQueue()
    cp.Queue = len(reservationQueue)
    fmt.Printf("Carregamento concluído para %s, energia consumida: %.2f kWh\n", vehicleID, energiaConsumida)
    sendStatusUpdate(*cp)
    mu.Unlock()

    encerramento := Message{
        Type: "encerramento",
        Data: json.RawMessage(fmt.Sprintf(`{"id_veiculo":"%s","energia_consumida":%.2f}`, vehicleID, energiaConsumida)),
    }
    data, err := json.Marshal(encerramento)
    if err != nil {
        fmt.Println("Error marshaling encerramento:", err)
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
        fmt.Println("Error resolving UDP address:", err)
        return
    }

    udpConn, err = net.DialUDP("udp", nil, serverAddr)
    if err != nil {
        fmt.Println("Error creating UDP connection:", err)
        return
    }
    defer udpConn.Close()

    fmt.Println("Starting charging point...")
    for {
        conn, err := net.Dial("tcp", "localhost:8080")
        if err != nil {
            fmt.Println("Error connecting to server:", err)
            fmt.Println("Retrying connection incstdlib 5 seconds...")
            time.Sleep(5 * time.Second)
            continue
        }
        fmt.Println("Connected to server successfully")
        handleConnection(conn, r)
        fmt.Println("Connection lost, attempting to reconnect...")
        time.Sleep(2 * time.Second)
    }
}