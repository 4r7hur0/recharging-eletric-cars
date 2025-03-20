package main

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

func main() {

}
