package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"
)

// Interface para converter a mensagem em JSON
type Mensagem interface {
	ToJSON() ([]byte, error)
}

// Estrutura para notificação de bateria
type NotificacaoBateria struct {
	IDVeiculo    string  `json:"id_veiculo"`
	NivelBateria float64 `json:"nivel_bateria"`
	Localizacao  *Coords `json:"localizacao"`
	Timestamp    string  `json:"timestamp"`
}

func (m *NotificacaoBateria) ToJSON() ([]byte, error) {
	return json.Marshal(m)
}

// Estrutura para consulta de pontos de recarga
type ConsultaPontos struct {
	IDVeiculo    string  `json:"id_veiculo"`
	Localizacao  *Coords `json:"localizacao"`
	DistanciaMax float64 `json:"distancia_max,omitempty"` // campo opcional
	Timestamp    string  `json:"timestamp"`
}

func (c *ConsultaPontos) ToJSON() ([]byte, error) {
	return json.Marshal(c)
}

// Estrutura para representar coordenadas
type Coords struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

func main() {
	for {
		fmt.Println("\n--- Menu ---")
		fmt.Println("1. Enviar notificação de bateria")
		fmt.Println("2. Consultar pontos de recarga disponíveis")
		fmt.Println("3. Sair")
		fmt.Print("Selecione uma opção: ")

		var opcao int
		_, err := fmt.Scanln(&opcao)
		if err != nil {
			fmt.Println("Erro na entrada:", err)
			continue
		}

		switch opcao {
		case 1:
			enviarNotificacaoBateria()
		case 2:
			consultarPontos()
		case 3:
			fmt.Println("Saindo...")
			os.Exit(0)
		default:
			fmt.Println("Opção inválida, tente novamente.")
		}
	}
}

// Função para enviar notificação de bateria
func enviarNotificacaoBateria() {
	fmt.Println("\n--- Notificação de Bateria ---")

	fmt.Print("Digite o ID do veículo: ")
	var idVeiculo string
	fmt.Scanln(&idVeiculo)

	fmt.Print("Digite a latitude: ")
	var latitude float64
	fmt.Scanln(&latitude)

	fmt.Print("Digite a longitude: ")
	var longitude float64
	fmt.Scanln(&longitude)

	fmt.Print("Digite o nível atual da bateria (%): ")
	var nivelBateria float64
	fmt.Scanln(&nivelBateria)

	timestamp := time.Now().Format("2006-01-02T15:04:05")

	msg := &NotificacaoBateria{
		IDVeiculo:    idVeiculo,
		NivelBateria: nivelBateria,
		Localizacao: &Coords{
			Latitude:  latitude,
			Longitude: longitude,
		},
		Timestamp: timestamp,
	}

	enviarMensagem(msg)
}

// Função para consultar pontos de recarga
func consultarPontos() {
	fmt.Println("\n--- Consulta de Pontos de Recarga ---")

	fmt.Print("Digite o ID do veículo: ")
	var idVeiculo string
	fmt.Scanln(&idVeiculo)

	fmt.Print("Digite a latitude do veículo: ")
	var latitude float64
	fmt.Scanln(&latitude)

	fmt.Print("Digite a longitude do veículo: ")
	var longitude float64
	fmt.Scanln(&longitude)

	fmt.Print("Digite a distância máxima aceitável (em km, opcional - digite 0 para ignorar): ")
	var distancia float64
	fmt.Scanln(&distancia)

	timestamp := time.Now().Format("2006-01-02T15:04:05")

	consulta := &ConsultaPontos{
		IDVeiculo:   idVeiculo,
		Localizacao: &Coords{Latitude: latitude, Longitude: longitude},
		Timestamp:   timestamp,
	}
	if distancia > 0 {
		consulta.DistanciaMax = distancia
	}

	enviarMensagem(consulta)
}

// Função genérica para enviar uma mensagem (notificação ou consulta) via TCP
func enviarMensagem(msg Mensagem) {
	conn, err := net.Dial("tcp", "localhost:8081")
	if err != nil {
		fmt.Println("Erro ao conectar:", err)
		return
	}
	defer conn.Close()

	jsonData, err := msg.ToJSON()
	if err != nil {
		fmt.Println("Erro ao gerar JSON:", err)
		return
	}

	_, err = conn.Write(jsonData)
	if err != nil {
		fmt.Println("Erro ao enviar dados:", err)
		return
	}

	buffer := make([]byte, 1024)
	n, err := conn.Read(buffer)
	if err != nil {
		fmt.Println("Erro ao receber resposta:", err)
		return
	}

	fmt.Println("Resposta recebida:", string(buffer[:n]))
}
