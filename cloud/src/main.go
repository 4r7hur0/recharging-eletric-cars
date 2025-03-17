package main

import (
	"fmt"
	"net"
	"os"
	"io"
	"encoding/json"
)

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

	// Processar a mensagem recebida
	var msg NotificacaoBateria
	err = json.Unmarshal(buffer[:n], &msg)
	if err != nil {
		fmt.Println("Erro ao decodificar JSON:", err)
		return
	}

	// Exibir os dados recebidos
	fmt.Printf("Mensagem recebida: \n")
	fmt.Printf("ID do veículo: %s\n", msg.IDVeiculo)
	fmt.Printf("Nível da bateria (em porcentagem): %.2f\n", msg.NivelBateria)
	fmt.Printf("Localização: %.6f, %.6f\n", msg.Localizacao.Latitude, msg.Localizacao.Longitude)
	fmt.Printf("Timestamp: %s\n", msg.Timestamp)

	// Responder ao cliente (opcional)
	response := "Dados recebidos com sucesso!"
	conn.Write([]byte(response))
}
