package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net"
	"sort"
	"sync"
)

type NotificacaoBateria struct {
	Type         string  `json:"tipo"`
	IDVeiculo    string  `json:"id_veiculo"`
	NivelBateria float64 `json:"nivel_bateria"`
	Localizacao  Coods   `json:"localizacao"`
	Timestamp    string  `json:"timestamp"`
}

type Coods struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type ChargingStation struct {
	StationID  string `json:"station_id"`
	Location   Coods  `json:"location"`
	Availiable bool   `json:"availiable"`
	Queue      int    `json:"queue"`
}

type reservation struct {
	Type          string `json:"tipo"`
	CarID         string `json:"id_veiculo"`
	StationID     string `json:"id_ponto_recarga"`
	EstimatedTime int    `json:"tempo_estimado"`
	TimeStamp     string `json:"timestamp"`
}

type BaseMassage struct {
	Type string `json:"tipo"`
}

var chargingPointsConnections = make(map[string]net.Conn)
var chargingPoints = make(map[string]*ChargingStation)
var mu sync.Mutex

func main() {

	go startTCPServer()

	go reciveStationStatus()

	go startChargingPointTCPServer()

	select {} // Mantém o servidor rodando
}

func startChargingPointTCPServer() {
	// Start the TCP server to recharging point
	ln, err := net.Listen("tcp", ":8083")
	if err != nil {
		fmt.Println("Error starting server", err)
		return
	}
	defer ln.Close()

	fmt.Println("Server for charging point started on port 8083")

	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Println("Error accepting connection", err)
			continue
		}
		go handleChargingPoint(conn)
	}
}

func handleChargingPoint(conn net.Conn) {
	defer conn.Close()

	buffer := make([]byte, 1024)
	n, err := conn.Read(buffer)
	if err != nil {
		fmt.Println("Error reading", err)
		return
	}

	var point ChargingStation
	err = json.Unmarshal(buffer[:n], &point)
	if err != nil {
		fmt.Println("Error unmarshalling", err)
		conn.Write([]byte("Error processing request"))
		return
	}

	mu.Lock()
	chargingPointsConnections[point.StationID] = conn
	fmt.Printf("%v", chargingPointsConnections)
	mu.Unlock()

	fmt.Printf("Updated station %v", point.StationID)

	for {
		_, err := conn.Read(buffer)
		if err != nil {
			fmt.Printf("charging point %v disconnected", point.StationID)
			mu.Lock()
			delete(chargingPoints, point.StationID)
			delete(chargingPointsConnections, point.StationID)
			mu.Unlock()
			return
		}

	}
}

func startTCPServer() {
	// Start the TCP server
	ln, err := net.Listen("tcp", ":8081")
	if err != nil {
		fmt.Println("Error starting server", err)
		return
	}
	defer ln.Close()

	fmt.Println("Server started on port 8081")

	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Println("Error accepting connection", err)
			continue
		}
		go handleVeicle(conn)
	}
}

func handleVeicle(conn net.Conn) {
	defer conn.Close()

	buffer := make([]byte, 1024)
	n, err := conn.Read(buffer)
	if err != nil {
		fmt.Println("Error reading", err)
		return
	}

	var base BaseMassage
	err = json.Unmarshal(buffer[:n], &base)
	if err != nil {
		fmt.Println("Error unmarshalling", err)
		return
	}

	fmt.Printf("Received message %v\n\n\n", base)

	switch base.Type {
	case "bateria":
		var notificacao NotificacaoBateria
		err = json.Unmarshal(buffer[:n], &notificacao)
		if err != nil {
			fmt.Println("Error unmarshalling NotificacaoBateria", err)
			conn.Write([]byte("Error processing battery notification"))
			return
		}
		fmt.Printf("Battery notification received: %+v\n", notificacao)
		sendAvailableStations(conn, notificacao.Localizacao)

	case "reserva":
		var vata reservation
		err = json.Unmarshal(buffer[:n], &vata)
		if err != nil {
			fmt.Println("Error unmarshalling NotificacaoBateria", err)
			conn.Write([]byte("Error processing battery notification"))
			return
		}
		processReservation(conn, vata)
	default:
		conn.Write([]byte("Invalid request"))
	}
}

func sendAvailableStations(conn net.Conn, carLocation Coods) {
	mu.Lock()
	defer mu.Unlock()

	type StationWithDistance struct {
		Station  ChargingStation
		Distance float64
	}

	var stationWithDistance []StationWithDistance
	for _, station := range chargingPoints {
		if station.Availiable {
			distance := calculateDistance(carLocation.Latitude, carLocation.Longitude, station.Location.Latitude, station.Location.Longitude)
			stationWithDistance = append(stationWithDistance, StationWithDistance{Station: *station, Distance: distance})
		}
	}

	// ordena as estações por distância
	sort.Slice(stationWithDistance, func(i, j int) bool {
		return stationWithDistance[i].Distance < stationWithDistance[j].Distance
	})

	var sortedStations []ChargingStation
	for _, ststationWithDistance := range stationWithDistance {
		sortedStations = append(sortedStations, ststationWithDistance.Station)
	}

	data, err := json.Marshal(sortedStations)
	if err != nil {
		fmt.Println("Error marshalling stations", err)
		conn.Write([]byte("Error processing request"))
		return
	}

	_, err = conn.Write(data)
	if err != nil {
		fmt.Println("Error sending stations", err)
	}
}

func calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371 // raio da terra em km
	lat1Rad := lat1 * (math.Pi / 180)
	lon1Rad := lon1 * (math.Pi / 180)
	lat2Rad := lat2 * (math.Pi / 180)
	lon2Rad := lon2 * (math.Pi / 180)

	dlat := lat2Rad - lat1Rad
	dlon := lon2Rad - lon1Rad

	a := math.Sin(dlat/2)*math.Sin(dlat/2) + math.Cos(lat1Rad)*math.Cos(lat2Rad)*math.Sin(dlon/2)*math.Sin(dlon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return R * c // distância em km
}

func processReservation(conn net.Conn, res reservation) {
	mu.Lock()

	fmt.Printf("%v\n\n\n\n\n", res.StationID)

	exist := chargingPoints[res.StationID]
	if exist == nil {
		conn.Write([]byte("Invalid station"))
		mu.Unlock()
		return
	}

	mu.Unlock()

	fmt.Printf("Reserving point %v for car %v\n", res.StationID, res.CarID)
	sendReservationToStation(res)
	conn.Write([]byte("Reservation done"))
}

func sendReservationToStation(res reservation) {
	mu.Lock()
	conn, ok := chargingPointsConnections[res.StationID]
	mu.Unlock()
	if !ok {
		fmt.Printf("Station %v not found\n", res.StationID)
		return
	}

	data, err := json.Marshal(res)
	if err != nil {
		fmt.Println("Error marshalling reservation", err)
		return
	}

	_, err = conn.Write(data)
	if err != nil {
		fmt.Println("Error sending reservation", err)
		return
	}

	buffer := make([]byte, 1024)
	n, err := conn.Read(buffer)
	if err != nil {
		fmt.Println("Error reading", err)
		return
	}

	fmt.Printf("Station response: %v", string(buffer[:n]))
}

func reciveStationStatus() {
	addr, _ := net.ResolveUDPAddr("udp", ":9090")
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		fmt.Println("Error resolving address", err)
		return
	}
	defer conn.Close()

	buffer := make([]byte, 1024)
	for {
		n, _, err := conn.ReadFromUDP(buffer)
		if err != nil {
			fmt.Println("Error reading", err)
			continue
		}

		var point ChargingStation
		err = json.Unmarshal(buffer[:n], &point)
		if err != nil {
			fmt.Println("Error unmarshalling", err)
			continue
		}

		mu.Lock()
		chargingPoints[point.StationID] = &point
		mu.Unlock()
		fmt.Printf("Station %v is %v\n", point.StationID, point.Availiable)
	}
}
