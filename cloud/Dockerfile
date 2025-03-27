FROM golang:1.21-alpine

WORKDIR /app

COPY go.mod .
COPY src/ .

RUN go build -o cloud

EXPOSE 8081

CMD ["./cloud"] 