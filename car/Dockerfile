# Imagem base do Go
FROM golang:1.21-alpine

# Define o diretório de trabalho dentro do container
WORKDIR /app

# Copia o arquivo go.mod primeiro para aproveitar o cache do Docker
COPY go.mod .

# Copia o código fonte
COPY src/ .

# Compila o aplicativo
RUN go build -o car

# Define variáveis de ambiente para o terminal
ENV TERM=xterm-256color
ENV TTY=true

# Comando para executar o cliente
CMD ["./car"] 