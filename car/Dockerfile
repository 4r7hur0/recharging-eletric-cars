# Usa a imagem base do Go
FROM golang:1.24-alpine

# Define o diretório de trabalho dentro do container
WORKDIR /app

# Copia os arquivos de módulo para aproveitar o cache do Docker
COPY go.mod go.sum ./

# Baixa as dependências antes de copiar o código-fonte
RUN go mod download

# Copia o código-fonte para dentro do container
COPY src/ .

# Compila o aplicativo
RUN go build -o car

# Define variáveis de ambiente para garantir compatibilidade com terminais interativos
ENV TERM=xterm-256color
ENV TTY=true

# Comando para executar o cliente
CMD ["./car"]
