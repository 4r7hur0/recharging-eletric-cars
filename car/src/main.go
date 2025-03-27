package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
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

// Estrutura para solicitação de reserva de ponto
type ReservaPonto struct {
	IDVeiculo      string `json:"id_veiculo"`
	IDPontoRecarga string `json:"id_ponto_recarga"`
	TempoEstimado  int    `json:"tempo_estimado"` // em minutos
	Timestamp      string `json:"timestamp"`
}

func (r *ReservaPonto) ToJSON() ([]byte, error) {
	return json.Marshal(r)
}

// Estrutura para confirmação de chegada
type ConfirmacaoChegada struct {
	IDVeiculo      string `json:"id_veiculo"`
	IDPontoRecarga string `json:"id_ponto_recarga"`
	Timestamp      string `json:"timestamp"`
}

func (c *ConfirmacaoChegada) ToJSON() ([]byte, error) {
	return json.Marshal(c)
}

// Estrutura para encerramento de sessão
type EncerramentoSessao struct {
	IDVeiculo        string  `json:"id_veiculo"`
	IDPontoRecarga   string  `json:"id_ponto_recarga"`
	EnergiaConsumida float64 `json:"energia_consumida"`
	Timestamp        string  `json:"timestamp"`
}

func (e *EncerramentoSessao) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Println("\n--- Menu ---")
		fmt.Println("1. Enviar notificação de bateria")
		fmt.Println("2. Consultar pontos de recarga disponíveis")
		fmt.Println("3. Solicitar reserva de ponto")
		fmt.Println("4. Confirmar chegada ao ponto")
		fmt.Println("5. Encerrar sessão de carregamento")
		fmt.Println("6. Sair")
		fmt.Print("Selecione uma opção: ")

		if scanner.Scan() {
			opcao, err := strconv.Atoi(strings.TrimSpace(scanner.Text()))
			if err != nil {
				fmt.Println("Erro na entrada:", err)
				continue
			}

			switch opcao {
			case 1:
				enviarNotificacaoBateria(scanner)
			case 2:
				consultarPontos(scanner)
			case 3:
				solicitarReserva(scanner)
			case 4:
				confirmarChegada(scanner)
			case 5:
				encerrarSessao(scanner)
			case 6:
				fmt.Println("Saindo...")
				os.Exit(0)
			default:
				fmt.Println("Opção inválida, tente novamente.")
			}
		}
	}
}

// Função para ler string do scanner
func lerString(scanner *bufio.Scanner) string {
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text())
	}
	return ""
}

// Função para ler float64 do scanner
func lerFloat64(scanner *bufio.Scanner) float64 {
	if scanner.Scan() {
		valor, err := strconv.ParseFloat(strings.TrimSpace(scanner.Text()), 64)
		if err != nil {
			fmt.Println("Erro ao ler número:", err)
			return 0
		}
		return valor
	}
	return 0
}

// Função para ler int do scanner
func lerInt(scanner *bufio.Scanner) int {
	if scanner.Scan() {
		valor, err := strconv.Atoi(strings.TrimSpace(scanner.Text()))
		if err != nil {
			fmt.Println("Erro ao ler número:", err)
			return 0
		}
		return valor
	}
	return 0
}

// Função para enviar notificação de bateria
func enviarNotificacaoBateria(scanner *bufio.Scanner) {
	fmt.Println("\n--- Notificação de Bateria ---")

	fmt.Print("Digite o ID do veículo: ")
	idVeiculo := lerString(scanner)

	fmt.Print("Digite a latitude: ")
	latitude := lerFloat64(scanner)

	fmt.Print("Digite a longitude: ")
	longitude := lerFloat64(scanner)

	fmt.Print("Digite o nível atual da bateria (%): ")
	nivelBateria := lerFloat64(scanner)

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
func consultarPontos(scanner *bufio.Scanner) {
	fmt.Println("\n--- Consulta de Pontos de Recarga ---")

	fmt.Print("Digite o ID do veículo: ")
	idVeiculo := lerString(scanner)

	fmt.Print("Digite a latitude do veículo: ")
	latitude := lerFloat64(scanner)

	fmt.Print("Digite a longitude do veículo: ")
	longitude := lerFloat64(scanner)

	fmt.Print("Digite a distância máxima aceitável (em km, opcional - digite 0 para ignorar): ")
	distancia := lerFloat64(scanner)

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

// Função para solicitar reserva de ponto
func solicitarReserva(scanner *bufio.Scanner) {
	fmt.Println("\n--- Solicitação de Reserva de Ponto ---")

	fmt.Print("Digite o ID do veículo: ")
	idVeiculo := lerString(scanner)

	fmt.Print("Digite o ID do ponto de recarga: ")
	idPontoRecarga := lerString(scanner)

	fmt.Print("Digite o tempo estimado de chegada (em minutos): ")
	tempoEstimado := lerInt(scanner)

	timestamp := time.Now().Format("2006-01-02T15:04:05")

	reserva := &ReservaPonto{
		IDVeiculo:      idVeiculo,
		IDPontoRecarga: idPontoRecarga,
		TempoEstimado:  tempoEstimado,
		Timestamp:      timestamp,
	}

	enviarMensagem(reserva)
}

// Função para confirmar chegada ao ponto
func confirmarChegada(scanner *bufio.Scanner) {
	fmt.Println("\n--- Confirmação de Chegada ---")

	fmt.Print("Digite o ID do veículo: ")
	idVeiculo := lerString(scanner)

	fmt.Print("Digite o ID do ponto de recarga: ")
	idPontoRecarga := lerString(scanner)

	timestamp := time.Now().Format("2006-01-02T15:04:05")

	confirmacao := &ConfirmacaoChegada{
		IDVeiculo:      idVeiculo,
		IDPontoRecarga: idPontoRecarga,
		Timestamp:      timestamp,
	}

	enviarMensagem(confirmacao)
}

// Função para encerrar sessão de carregamento
func encerrarSessao(scanner *bufio.Scanner) {
	fmt.Println("\n--- Encerramento de Sessão de Carregamento ---")

	fmt.Print("Digite o ID do veículo: ")
	idVeiculo := lerString(scanner)

	fmt.Print("Digite o ID do ponto de recarga: ")
	idPontoRecarga := lerString(scanner)

	fmt.Print("Digite a energia consumida (kWh): ")
	energiaConsumida := lerFloat64(scanner)

	timestamp := time.Now().Format("2006-01-02T15:04:05")

	encerramento := &EncerramentoSessao{
		IDVeiculo:        idVeiculo,
		IDPontoRecarga:   idPontoRecarga,
		EnergiaConsumida: energiaConsumida,
		Timestamp:        timestamp,
	}

	enviarMensagem(encerramento)
}

// Função genérica para enviar uma mensagem (notificação ou consulta) via TCP
func enviarMensagem(msg Mensagem) {
	conn, err := net.Dial("tcp", "cloud:8081")
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
