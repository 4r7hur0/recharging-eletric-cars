package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"
)

// Estrutura para mensagem genérica
type Message struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// Estrutura para notificação de bateria
type NotificacaoBateria struct {
	IDVeiculo    string  `json:"id_veiculo"`
	NivelBateria float64 `json:"nivel_bateria"`
	Localizacao  Coords  `json:"localizacao"`
	Timestamp    string  `json:"timestamp"`
}

// Estrutura para consulta de pontos de recarga
type ConsultaPontos struct {
	IDVeiculo    string  `json:"id_veiculo"`
	Localizacao  *Coords `json:"localizacao"`
	DistanciaMax float64 `json:"distancia_max,omitempty"` // campo opcional
	Timestamp    string  `json:"timestamp"`
}

// Estrutura para representar coordenadas
type Coords struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// Estrutura para solicitação de reserva de ponto
type ReservaPonto struct {
	IDVeiculo      string `json:"id_veiculo"`
	IDPontoRecarga string `json:"id_ponto_recarga"`
	TempoEstimado  int    `json:"tempo_estimado"` // em minutos
	Timestamp      string `json:"timestamp"`
}

// Estrutura para confirmação de chegada
type ConfirmacaoChegada struct {
	IDVeiculo      string `json:"id_veiculo"`
	IDPontoRecarga string `json:"id_ponto_recarga"`
	NivelBateria   float64 `json:"nivel_bateria"`
	NivelCarregamento  float64 `json:"nivel_carregar"`
	Timestamp      string `json:"timestamp"`
}

// Estrutura para encerramento de sessão
type EncerramentoSessao struct {
	IDVeiculo        string  `json:"id_veiculo"`
	IDPontoRecarga   string  `json:"id_ponto_recarga"`
	EnergiaConsumida float64 `json:"energia_consumida"`
	Timestamp        string  `json:"timestamp"`
}

func main() {
	for {
		fmt.Println("\n--- Menu ---")
		fmt.Println("1. Enviar notificação de bateria")
		fmt.Println("2. Consultar pontos de recarga disponíveis")
		fmt.Println("3. Solicitar reserva de ponto")
		fmt.Println("4. Confirmar chegada ao ponto")
		fmt.Println("5. Encerrar sessão de carregamento")
		fmt.Println("6. Sair")
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
			solicitarReserva()
		case 4:
			confirmarChegada()
		case 5:
			encerrarSessao()
		case 6:
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

	fmt.Print("Digite até que nível deseja carregar a bateria (%):")
	var nivelCarregar float64
	fmt.Scanln(&nivelCarregar)

	timestamp := time.Now().Format("2006-01-02T15:04:05")

	notif := &NotificacaoBateria{
		IDVeiculo:    idVeiculo,
		NivelBateria: nivelBateria,
		Localizacao: Coords{
			Latitude:  latitude,
			Longitude: longitude,
		},
		Timestamp: timestamp,
	}

	jsonData, err := json.Marshal(notif)
	if err != nil {
		fmt.Println("Erro ao gerar JSON:", err)
		return
	}

	msg := &Message{
		Type: "bateria",
		Data: jsonData,
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

	jsonData, err := json.Marshal(consulta)
	if err != nil {
		fmt.Println("Erro ao gerar JSON:", err)
		return
	}

	msg := &Message{
		Type: "consulta",
		Data: jsonData,
	}

	enviarMensagem(msg)
}

// Função para solicitar reserva de ponto
func solicitarReserva() {
	fmt.Println("\n--- Solicitação de Reserva de Ponto ---")

	fmt.Print("Digite o ID do veículo: ")
	var idVeiculo string
	fmt.Scanln(&idVeiculo)

	fmt.Print("Digite o ID do ponto de recarga: ")
	var idPontoRecarga string
	fmt.Scanln(&idPontoRecarga)

	fmt.Print("Digite o tempo estimado de chegada (em minutos): ")
	var tempoEstimado int
	fmt.Scanln(&tempoEstimado)

	timestamp := time.Now().Format("2006-01-02T15:04:05")

	reserva := &ReservaPonto{
		IDVeiculo:      idVeiculo,
		IDPontoRecarga: idPontoRecarga,
		TempoEstimado:  tempoEstimado,
		Timestamp:      timestamp,
	}

	jsonData, err := json.Marshal(reserva)
	if err != nil {
		fmt.Println("Erro ao gerar JSON:", err)
		return
	}

	msg := &Message{
		Type: "reserva",
		Data: jsonData,
	}

	enviarMensagem(msg)
}

// Função para confirmar chegada ao ponto
func confirmarChegada() {
	fmt.Println("\n--- Confirmação de Chegada ---")

	fmt.Print("Digite o ID do veículo: ")
	var idVeiculo string
	fmt.Scanln(&idVeiculo)

	fmt.Print("Digite o ID do ponto de recarga: ")
	var idPontoRecarga string
	fmt.Scanln(&idPontoRecarga)

	fmt.Print("Digite o nível atual da bateria (%): ")
	var nivelBateria float64
	fmt.Scanln(&nivelBateria)

	fmt.Print("Digite até que nível deseja carregar a bateria (%):")
	var nivelCarregamento float64
	fmt.Scanln(&nivelCarregamento)

	timestamp := time.Now().Format("2006-01-02T15:04:05")

	confirmacao := &ConfirmacaoChegada{
		IDVeiculo:      idVeiculo,
		IDPontoRecarga: idPontoRecarga,
		NivelBateria:   nivelBateria,
		NivelCarregamento: nivelCarregamento,
		Timestamp:      timestamp,
	}

	jsonData, err := json.Marshal(confirmacao)
	if err != nil {
		fmt.Println("Erro ao gerar JSON:", err)
		return
	}

	msg := &Message{
		Type: "chegada",
		Data: jsonData,
	}

	enviarMensagem(msg)
}

// Função para encerrar sessão de carregamento
func encerrarSessao() {
	fmt.Println("\n--- Encerramento de Sessão de Carregamento ---")

	fmt.Print("Digite o ID do veículo: ")
	var idVeiculo string
	fmt.Scanln(&idVeiculo)

	fmt.Print("Digite o ID do ponto de recarga: ")
	var idPontoRecarga string
	fmt.Scanln(&idPontoRecarga)

	fmt.Print("Digite a energia consumida (kWh): ")
	var energiaConsumida float64
	fmt.Scanln(&energiaConsumida)

	timestamp := time.Now().Format("2006-01-02T15:04:05")

	encerramento := &EncerramentoSessao{
		IDVeiculo:        idVeiculo,
		IDPontoRecarga:   idPontoRecarga,
		EnergiaConsumida: energiaConsumida,
		Timestamp:        timestamp,
	}

	jsonData, err := json.Marshal(encerramento)
	if err != nil {
		fmt.Println("Erro ao gerar JSON:", err)
		return
	}

	msg := &Message{
		Type: "encerramento",
		Data: jsonData,
	}

	enviarMensagem(msg)
}

// Função genérica para enviar uma mensagem via TCP
func enviarMensagem(msg *Message) {
	conn, err := net.Dial("tcp", "cloud-server:8081")
	if err != nil {
		fmt.Println("Erro ao conectar:", err)
		return
	}
	defer conn.Close()

	jsonData, err := json.Marshal(msg)
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
