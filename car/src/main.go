package main 

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"
)

// Estrutura da mensagem JSON

type Mensagem interface {
	ToJson() ([]byte, error)
}

type NotificacaoBateria struct {
	IDVeiculo string `json:"id_veiculo"`
	NivelBateria float64 `json:"nivel_bateria"`
	Localizacao *Coords `json:"localizacao"`
	Timestamp string `json:"timestamp"`
}

func (m *NotificacaoBateria) ToJSON() ([]byte, error){
	return json.Marshal(m)
}

type Coords struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

func main() {	
	
	// Receber os dados pelo terminal
	fmt.Println("Digite o ID do veículo: ")
	var IdVeiculo string
	fmt.Scanln(&IdVeiculo)

	fmt.Println("Digite a latitude: ")
	var latitude float64
	fmt.Scanln(&latitude)

	fmt.Println("Digite a longitude: ")
	var longitude float64
	fmt.Scanln(&longitude)

	fmt.Println("Digite o nível atual da bateria (%): ")
	var nivelBateria float64
	fmt.Scanln(&nivelBateria)

	// Gerar o timestamp (data e hora) com time.Now()
	timestamp := time.Now().Format("2006-01-02T15:04:05") // Formato padrão: yyyy-mm-ddTHH:MM:SS

	// Criar a mensagem de notificação de bateria
	msg := &NotificacaoBateria{
		IDVeiculo:    IdVeiculo,
		NivelBateria: nivelBateria,
		Localizacao: &Coords{
			Latitude:  latitude,
			Longitude: longitude,
		},
		Timestamp: timestamp,
	}

	// Conectar ao servidor
	conn, err := net.Dial("tcp", "localhost:8081")
	if err != nil {
		fmt.Println("Erro ao conectar:", err)
		os.Exit(1)
	}
	defer conn.Close()

	// Enviar a mensagem
	jsonData, err := msg.ToJSON()
	if err != nil {
		fmt.Println("Erro ao gerar JSON:", err)
		return
	}

	_, err = conn.Write(jsonData)
	if err != nil {
		fmt.Println("Erro ao enviar dados: ", err)
		return
	}

	// Receber resposta
	buffer := make([]byte, 1024)
	n, err := conn.Read(buffer)
	if err != nil {
		fmt.Println("Erro ao receber resposta: ", err)
		return
	}

	fmt.Println("Resposta recebida: ", string(buffer[:n]))
}
