package cloud

import (
	"encoding/json"
	"fmt"
	"net"
)

type ChargingRequest struct {
	Type      string  `json:"type"`
	CarID     string  `json:"car_id"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Battery   float64 `json:"battery"`
}

type ChargingStation struct {
	StationID string  `json:"station_id"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Avaliable bool    `json:"avaliable"`
	Price     float64 `json:"price"`
}

type ChargingResponse struct {
	Type     string            `json:"type"`
	Stations []ChargingStation `json:"stations"`
}

func HandleConnection(conn net.Conn) {
	defer conn.Close()

	buffer := make([]byte, 1024)
	n, err := conn.Read(buffer)
	if err != nil {
		fmt.Println("Error reading:", err)
		return
	}

	var request ChargingRequest
	err = json.Unmarshal(buffer[:n], &request)
	if err != nil {
		fmt.Println("Error unmarshaling:", err)
		return
	}

	fmt.Println("Request acepted: ", request.CarID)

	response := ChargingResponse{
		Type: "charging_response",
		Stations: []ChargingStation{
			{StationID: "st1", Latitude: -23.55, Longitude: -46.63, Avaliable: true, Price: 5.99},
			{StationID: "st2", Latitude: -23.56, Longitude: -46.64, Avaliable: false, Price: 7.00},
		},
	}

	jsonResponse, _ := json.Marshal(response)
	conn.Write(jsonResponse)
}

func main() {

}
