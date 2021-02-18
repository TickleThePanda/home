FROM golang:alpine

RUN mkdir app
WORKDIR app
COPY go.* ./
RUN go mod download

COPY src/ src/

CMD go run src/main.go

