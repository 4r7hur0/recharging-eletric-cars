FROM golang:1.21-alpine

WORKDIR /app

COPY go.mod .
COPY src/ .

RUN go build -o car

# Configurar para interação com o terminal
ENV TERM=xterm-256color
ENV TTY=true

# Usar shell para melhor interação
CMD ["/bin/sh", "-c", "./car"] 