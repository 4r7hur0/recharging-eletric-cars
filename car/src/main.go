package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/manifoldco/promptui"
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
	IDVeiculo         string  `json:"id_veiculo"`
	IDPontoRecarga    string  `json:"id_ponto_recarga"`
	NivelBateria      float64 `json:"nivel_bateria"`
	NivelCarregamento float64 `json:"nivel_carregar"`
	MetodoPagamento   string  `json:"metodo_pagamento"`
	Timestamp         string  `json:"timestamp"`
}

// Estrutura para encerramento de sessão
type EncerramentoSessao struct {
	IDVeiculo        string  `json:"id_veiculo"`
	IDPontoRecarga   string  `json:"id_ponto_recarga"`
	EnergiaConsumida float64 `json:"energia_consumida"`
	Timestamp        string  `json:"timestamp"`
}

/**
* Estrutura para armazenar a conexão global
* e um mutex para garantir acesso seguro à conexão.
 */
var (
	globalConn      net.Conn
	globalConnMutex sync.Mutex
	IDVeiculo       string
)

func main() {
	if err := inicializarConexao(); err != nil {
		fmt.Println("Erro ao conectar com o servidor:", err)
		os.Exit(1)
	}
	defer globalConn.Close()

	menu := []string{
		"Enviar notificação de bateria",
		"Consultar pontos de recarga disponíveis",
		"Solicitar reserva de ponto",
		"Confirmar chegada ao ponto",
		"Encerrar sessão de carregamento",
		"Sair",
	}

	for {
		prompt := promptui.Select{
			Label: "Selecione uma opção",
			Items: menu,
		}

		_, result, err := prompt.Run()
		if err != nil {
			fmt.Println("Erro na seleção:", err)
			continue
		}

		switch result {
		case menu[0]:
			enviarNotificacaoBateria()
		case menu[1]:
			consultarPontos()
		case menu[2]:
			solicitarReserva()
		case menu[3]:
			confirmarChegada()
		case menu[4]:
			encerrarSessao()
		case menu[5]:
			fmt.Println("Saindo...")
			os.Exit(0)
		}
	}
}

// Função para inicializar a conexão TCP persistente
func inicializarConexao() error {
	var err error
	globalConn, err = net.Dial("tcp", "localhost:8081")
	if err != nil {
		return err
	}

	// Obter o endereço local da conexão para extrair a porta
	localAddr := globalConn.LocalAddr().String()
	_, portStr, err := net.SplitHostPort(localAddr)
	if err != nil {
		return fmt.Errorf("erro ao extrair porta: %v", err)
	}

	IDVeiculo = fmt.Sprintf("CAR%s", portStr)
	fmt.Printf("Identificação do veículo: %s\n", IDVeiculo)

	// Iniciar goroutine para lidar com respostas assíncronas do servidor
	go receberRespostas()

	fmt.Println("Conexão estabelecida com o servidor.")
	return nil
}

// Função para receber respostas assíncronas do servidor
func receberRespostas() {
	buffer := make([]byte, 4096)
	for {
		n, err := globalConn.Read(buffer)
		if err != nil {
			fmt.Println("Conexão com o servidor perdida:", err)
			// Tentar reconectar
			tentarReconectar()
			return
		}

		if n > 0 {
			fmt.Println("Resposta do servidor:", string(buffer[:n]))
		}
	}
}

// Função para tentar reconectar em caso de perda de conexão
func tentarReconectar() {
	globalConnMutex.Lock()
	defer globalConnMutex.Unlock()

	maxTentativas := 5
	for i := 0; i < maxTentativas; i++ {
		fmt.Printf("Tentando reconectar (%d/%d)...\n", i+1, maxTentativas)

		var err error
		globalConn, err = net.Dial("tcp", "localhost:8081")
		if err == nil {
			fmt.Println("Reconectado com sucesso!")
			go receberRespostas() // Reiniciar a goroutine de recepção
			return
		}

		// Esperar antes de tentar novamente com backoff exponencial
		tempo := time.Duration(1<<uint(i)) * time.Second
		if tempo > 30*time.Second {
			tempo = 30 * time.Second
		}
		time.Sleep(tempo)
	}

	fmt.Println("Não foi possível reconectar após várias tentativas. Reinicie o aplicativo.")
	os.Exit(1)
}

// Função para enviar notificação de bateria
func enviarNotificacaoBateria() {
	fmt.Println("\n--- Notificação de Bateria ---")

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
		IDVeiculo:    IDVeiculo,
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
		IDVeiculo:   IDVeiculo,
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

	fmt.Print("Digite o ID do ponto de recarga: ")
	var idPontoRecarga string
	fmt.Scanln(&idPontoRecarga)

	fmt.Print("Digite o tempo estimado de chegada (em minutos): ")
	var tempoEstimado int
	fmt.Scanln(&tempoEstimado)

	timestamp := time.Now().Format("2006-01-02T15:04:05")

	reserva := &ReservaPonto{
		IDVeiculo:      IDVeiculo,
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

	fmt.Print("Digite o ID do ponto de recarga: ")
	var idPontoRecarga string
	fmt.Scanln(&idPontoRecarga)

	fmt.Print("Digite o nível atual da bateria (%): ")
	var nivelBateria float64
	fmt.Scanln(&nivelBateria)

	fmt.Print("Digite até que nível deseja carregar a bateria (%):")
	var nivelCarregamento float64
	fmt.Scanln(&nivelCarregamento)

	fmt.Println("\nSelecione o método de pagamento:")
	fmt.Println("1 - Pix")
	fmt.Println("2 - Débito em conta")
	fmt.Println("3 - Cartão de crédito")
	fmt.Print("Digite o número correspondente: ")
	var opcaoPagamento int
	fmt.Scanln(&opcaoPagamento)

	metodoPagamento := ""

	switch opcaoPagamento {
	case 1:
		metodoPagamento = "Pix"
	case 2:
		metodoPagamento = "Débito em conta"
	case 3:
		metodoPagamento = "Cartão de crédito"
	default:
		metodoPagamento = "Método de pagamento inválido"
	}
	timestamp := time.Now().Format("2006-01-02T15:04:05")

	confirmacao := &ConfirmacaoChegada{
		IDVeiculo:         IDVeiculo,
		IDPontoRecarga:    idPontoRecarga,
		NivelBateria:      nivelBateria,
		NivelCarregamento: nivelCarregamento,
		MetodoPagamento:   metodoPagamento,
		Timestamp:         timestamp,
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

	fmt.Print("Digite o ID do ponto de recarga: ")
	var idPontoRecarga string
	fmt.Scanln(&idPontoRecarga)

	fmt.Print("Digite a energia consumida (kWh): ")
	var energiaConsumida float64
	fmt.Scanln(&energiaConsumida)

	timestamp := time.Now().Format("2006-01-02T15:04:05")

	encerramento := &EncerramentoSessao{
		IDVeiculo:        IDVeiculo,
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
	globalConnMutex.Lock()
	defer globalConnMutex.Unlock()

	if globalConn == nil {
		fmt.Println("Erro: não há conexão ativa com o servidor")
		return
	}

	jsonData, err := json.Marshal(msg)
	if err != nil {
		fmt.Println("Erro ao gerar JSON:", err)
		return
	}

	_, err = globalConn.Write(jsonData)
	if err != nil {
		fmt.Println("Erro ao enviar dados:", err)
		// A reconexão será tratada pela goroutine de recebimento
		return
	}

	fmt.Println("Mensagem enviada com sucesso")
	// Note que não esperamos pela resposta aqui, pois ela será recebida
	// pela goroutine receberRespostas() de forma assíncrona
}
