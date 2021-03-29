FROM golang:alpine

RUN mkdir app
WORKDIR app
COPY go.* ./
RUN go mod download

COPY src/ src/

VOLUME /data

CMD go run src/*.go

