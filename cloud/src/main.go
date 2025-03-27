package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
)

// Estruturas de dados compartilhadas com o cliente
type Coords struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type NotificacaoBateria struct {
	IDVeiculo    string  `json:"id_veiculo"`
	NivelBateria float64 `json:"nivel_bateria"`
	Localizacao  *Coords `json:"localizacao"`
	Timestamp    string  `json:"timestamp"`
}

type ConsultaPontos struct {
	IDVeiculo    string  `json:"id_veiculo"`
	Localizacao  *Coords `json:"localizacao"`
	DistanciaMax float64 `json:"distancia_max,omitempty"`
	Timestamp    string  `json:"timestamp"`
}

type ReservaPonto struct {
	IDVeiculo      string `json:"id_veiculo"`
	IDPontoRecarga string `json:"id_ponto_recarga"`
	TempoEstimado  int    `json:"tempo_estimado"`
	Timestamp      string `json:"timestamp"`
}

type ConfirmacaoChegada struct {
	IDVeiculo      string `json:"id_veiculo"`
	IDPontoRecarga string `json:"id_ponto_recarga"`
	Timestamp      string `json:"timestamp"`
}

type EncerramentoSessao struct {
	IDVeiculo        string  `json:"id_veiculo"`
	IDPontoRecarga   string  `json:"id_ponto_recarga"`
	EnergiaConsumida float64 `json:"energia_consumida"`
	Timestamp        string  `json:"timestamp"`
}

// Estruturas de resposta do servidor
type PontoRecarga struct {
	IDPontoRecarga string  `json:"id_ponto_recarga"`
	Localizacao    *Coords `json:"localizacao"`
	Status         string  `json:"status"`       // livre, ocupado, em manutenção
	TempoEspera    int     `json:"tempo_espera"` // em minutos
	CustoPorKWh    float64 `json:"custo_por_kwh"`
}

type RespostaPontos struct {
	Pontos []PontoRecarga `json:"pontos"`
}

type RespostaReserva struct {
	IDReserva       string `json:"id_reserva"`
	Status          string `json:"status"`            // confirmada/rejeitada
	TempoMaxChegada int    `json:"tempo_max_chegada"` // em minutos
}

type RespostaInicioCarregamento struct {
	IDPontoRecarga   string `json:"id_ponto_recarga"`
	TempoEstimadoFim int    `json:"tempo_estimado_fim"` // em minutos
}

type RespostaFimCarregamento struct {
	IDPontoRecarga   string  `json:"id_ponto_recarga"`
	EnergiaFornecida float64 `json:"energia_fornecida"` // em kWh
	CustoTotal       float64 `json:"custo_total"`
}

func main() {
	// Abrir o servidor na porta 8081
	listener, err := net.Listen("tcp", ":8081")
	if err != nil {
		fmt.Println("Erro ao iniciar o servidor:", err)
		os.Exit(1)
	}
	defer listener.Close()
	fmt.Println("Servidor aguardando conexões na porta 8081...")

	for {
		// Aceitar uma conexão de entrada
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Erro ao aceitar conexão:", err)
			continue
		}

		// Processar a conexão
		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()

	// Receber a mensagem JSON
	buffer := make([]byte, 1024)
	n, err := conn.Read(buffer)
	if err != nil {
		if err != io.EOF {
			fmt.Println("Erro ao ler dados:", err)
		}
		return
	}

	// Tentar decodificar a mensagem como cada tipo possível
	var response interface{}

	// Notificação de Bateria
	var notifBateria NotificacaoBateria
	if err := json.Unmarshal(buffer[:n], &notifBateria); err == nil && notifBateria.IDVeiculo != "" {
		fmt.Printf("Notificação de bateria recebida do veículo %s\n", notifBateria.IDVeiculo)
		response = "Notificação de bateria registrada com sucesso!"
	}

	// Consulta de Pontos
	var consultaPontos ConsultaPontos
	if err := json.Unmarshal(buffer[:n], &consultaPontos); err == nil && consultaPontos.IDVeiculo != "" {
		fmt.Printf("Consulta de pontos recebida do veículo %s\n", consultaPontos.IDVeiculo)
		// Simular resposta com pontos de recarga
		response = RespostaPontos{
			Pontos: []PontoRecarga{
				{
					IDPontoRecarga: "P001",
					Localizacao:    &Coords{Latitude: -23.550520, Longitude: -46.633308},
					Status:         "livre",
					TempoEspera:    0,
					CustoPorKWh:    0.85,
				},
				{
					IDPontoRecarga: "P002",
					Localizacao:    &Coords{Latitude: -23.551520, Longitude: -46.634308},
					Status:         "ocupado",
					TempoEspera:    15,
					CustoPorKWh:    0.85,
				},
			},
		}
	}

	// Reserva de Ponto
	var reservaPonto ReservaPonto
	if err := json.Unmarshal(buffer[:n], &reservaPonto); err == nil && reservaPonto.IDVeiculo != "" {
		fmt.Printf("Reserva de ponto recebida do veículo %s\n", reservaPonto.IDVeiculo)
		response = RespostaReserva{
			IDReserva:       "R001",
			Status:          "confirmada",
			TempoMaxChegada: 30,
		}
	}

	// Confirmação de Chegada
	var confirmacaoChegada ConfirmacaoChegada
	if err := json.Unmarshal(buffer[:n], &confirmacaoChegada); err == nil && confirmacaoChegada.IDVeiculo != "" {
		fmt.Printf("Confirmação de chegada recebida do veículo %s\n", confirmacaoChegada.IDVeiculo)
		response = RespostaInicioCarregamento{
			IDPontoRecarga:   confirmacaoChegada.IDPontoRecarga,
			TempoEstimadoFim: 120, // 2 horas estimadas
		}
	}

	// Encerramento de Sessão
	var encerramentoSessao EncerramentoSessao
	if err := json.Unmarshal(buffer[:n], &encerramentoSessao); err == nil && encerramentoSessao.IDVeiculo != "" {
		fmt.Printf("Encerramento de sessão recebido do veículo %s\n", encerramentoSessao.IDVeiculo)
		response = RespostaFimCarregamento{
			IDPontoRecarga:   encerramentoSessao.IDPontoRecarga,
			EnergiaFornecida: encerramentoSessao.EnergiaConsumida,
			CustoTotal:       encerramentoSessao.EnergiaConsumida * 0.85, // Custo estimado
		}
	}

	// Enviar resposta
	jsonResponse, err := json.Marshal(response)
	if err != nil {
		fmt.Println("Erro ao gerar resposta JSON:", err)
		return
	}

	_, err = conn.Write(jsonResponse)
	if err != nil {
		fmt.Println("Erro ao enviar resposta:", err)
		return
	}
}
