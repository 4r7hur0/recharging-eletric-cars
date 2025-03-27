# Sistema de Recarga de Veículos Elétricos

Este projeto implementa um sistema de comunicação entre veículos elétricos e pontos de recarga.

## Estrutura do Projeto

```
.
├── car/                    # Cliente (aplicativo do veículo)
│   ├── Dockerfile
│   └── src/
│       └── main.go
├── cloud/                  # Servidor (nuvem)
│   ├── Dockerfile
│   └── src/
│       └── main.go
├── docker-compose.yml
└── README.md
```

## Requisitos

- Docker
- Docker Compose

## Como Executar

1. Clone o repositório
2. Na pasta raiz do projeto, execute:
   ```bash
   docker-compose up --build
   ```

3. O sistema iniciará com dois containers:
   - `cloud`: Servidor na porta 8081
   - `car`: Cliente interativo

4. Use o menu interativo do cliente para:
   - Enviar notificação de bateria
   - Consultar pontos de recarga
   - Solicitar reserva de ponto
   - Confirmar chegada
   - Encerrar sessão de carregamento

## Testando o Sistema

1. Após iniciar os containers, você verá o menu do cliente no terminal
2. Digite o número da opção desejada
3. Siga as instruções na tela para inserir os dados necessários
4. O servidor processará a solicitação e retornará uma resposta

## Encerrando o Sistema

Para encerrar o sistema, pressione `Ctrl+C` no terminal onde o docker-compose está rodando.