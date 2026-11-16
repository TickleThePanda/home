FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN go test ./...
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /calendar-proxy .

FROM scratch
COPY --from=build /calendar-proxy /calendar-proxy
USER 65532:65532
ENTRYPOINT ["/calendar-proxy"]
